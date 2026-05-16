package n4j

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// NodeRepo implementa repository.NodeRepository sobre Neo4j.
type NodeRepo struct{ *Client }

// NewNodeRepo embrulha um Client.
func NewNodeRepo(c *Client) *NodeRepo { return &NodeRepo{c} }

// Compile-time interface check.
var _ repository.NodeRepository = (*NodeRepo)(nil)

// Upsert versiona o nó. Estratégia:
//  1. Se houver versão corrente (valid_to IS NULL) com mesma URN,
//     fecha-a (valid_to = now).
//  2. Cria um novo vértice com a versão atual.
//
// Ambas as operações ocorrem na mesma transação para atomicidade.
func (r *NodeRepo) Upsert(ctx context.Context, n node.Node) error {
	if n == nil || n.URN() == "" {
		return fmt.Errorf("%w: nil node or empty URN", repository.ErrInvalidArgument)
	}
	label, props, err := nodeProps(n)
	if err != nil {
		return err
	}
	now := nowFn().UTC()

	_, err = r.withWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// 1. Fecha versão corrente, se houver.
		_, err := tx.Run(ctx,
			"MATCH (cur:CeNode {urn: $urn}) WHERE cur.valid_to IS NULL "+
				"SET cur.valid_to = $now",
			map[string]any{"urn": string(n.URN()), "now": now},
		)
		if err != nil {
			return nil, err
		}
		// 2. Cria nova versão. Label dinâmico via APOC? Para evitar dep,
		//    usamos label fixo CeNode + property kind. (Labels específicos
		//    por Kind ficam para uma migration posterior usando APOC.)
		_, err = tx.Run(ctx,
			"CREATE (n:CeNode) SET n += $props",
			map[string]any{"props": props},
		)
		_ = label // reservado para uso futuro com APOC label setting
		return nil, err
	})
	return err
}

// GetByURN retorna a versão corrente (ou as-of) de um nó.
func (r *NodeRepo) GetByURN(ctx context.Context, urn node.URN, as repository.AsOf) (node.Node, error) {
	var (
		query  string
		params = map[string]any{"urn": string(urn)}
	)
	if as.IsZero() {
		query = "MATCH (n:CeNode {urn: $urn}) WHERE n.valid_to IS NULL RETURN n LIMIT 1"
	} else {
		query = "MATCH (n:CeNode {urn: $urn}) " +
			"WHERE n.valid_from <= $at AND (n.valid_to IS NULL OR n.valid_to > $at) " +
			"RETURN n LIMIT 1"
		params["at"] = as.Time().UTC()
	}

	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if !rec.Next(ctx) {
			return nil, fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
		}
		nodeRaw, _ := rec.Record().Get("n")
		nNode, ok := nodeRaw.(neo4j.Node)
		if !ok {
			return nil, errors.New("n4j: unexpected return shape")
		}
		return decodeNode(nNode.Props)
	})
	if err != nil {
		return nil, err
	}
	return res.(node.Node), nil
}

// GetByExternalID resolve um identificador nativo para a URN canônica.
//
// Semântica (F-003):
//   - account != "" → match estrito sobre n.account CONTAINS $acct.
//     ("CONTAINS" permite tanto URNs completas quanto IDs nus.)
//   - account == "" → wildcard cross-account; ErrAmbiguous se mais de uma
//     URN matchea.
//   - externalID é tentado como external_id OU short_id (ARN/short alias).
//   - AsOf zero → versão corrente; caso contrário, filtra por valid_from/to.
func (r *NodeRepo) GetByExternalID(ctx context.Context, p node.ProviderID, account, externalID string, as repository.AsOf) (node.URN, error) {
	if externalID == "" {
		return "", fmt.Errorf("%w: empty externalID", repository.ErrInvalidArgument)
	}

	var (
		query  string
		params = map[string]any{"p": string(p), "eid": externalID}
	)
	// Match externalID em external_id ou short_id (alias bidirecional).
	matchClause := "MATCH (n:CeNode {provider: $p}) WHERE (n.external_id = $eid OR n.short_id = $eid)"

	// Tempo lógico: corrente ou as-of.
	if as.IsZero() {
		matchClause += " AND n.valid_to IS NULL"
	} else {
		matchClause += " AND n.valid_from <= $at AND (n.valid_to IS NULL OR n.valid_to > $at)"
		params["at"] = as.Time().UTC()
	}

	// Filtro por account (estrito) — limita a 2 resultados (suficiente
	// para detectar ambiguidade sem custo desnecessário).
	if account != "" {
		matchClause += " AND n.account CONTAINS $acct"
		params["acct"] = account
	}
	query = matchClause + " RETURN n.urn AS urn LIMIT 2"

	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		var urns []node.URN
		for rec.Next(ctx) {
			u, _ := rec.Record().Get("urn")
			urns = append(urns, node.URN(u.(string)))
		}
		if err := rec.Err(); err != nil {
			return nil, err
		}
		switch len(urns) {
		case 0:
			return nil, fmt.Errorf("%w: external %s/%s/%s", repository.ErrNotFound, p, account, externalID)
		case 1:
			return urns[0], nil
		default:
			if account != "" {
				return nil, fmt.Errorf("%w: external %s/%s/%s matched %d URNs",
					repository.ErrAmbiguous, p, account, externalID, len(urns))
			}
			return nil, fmt.Errorf("%w: external %s/*/%s matched %d URNs (cross-account)",
				repository.ErrAmbiguous, p, externalID, len(urns))
		}
	})
	if err != nil {
		return "", err
	}
	return res.(node.URN), nil
}

// List devolve nós que satisfazem o filtro.
func (r *NodeRepo) List(ctx context.Context, f repository.NodeFilter) ([]node.Node, error) {
	q, params := buildListQuery(f)
	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, q, params)
		if err != nil {
			return nil, err
		}
		out := []node.Node{}
		for rec.Next(ctx) {
			raw, _ := rec.Record().Get("n")
			nn, ok := raw.(neo4j.Node)
			if !ok {
				continue
			}
			n, err := decodeNode(nn.Props)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, rec.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]node.Node), nil
}

// History retorna todas as versões.
func (r *NodeRepo) History(ctx context.Context, urn node.URN) ([]node.Node, error) {
	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx,
			"MATCH (n:CeNode {urn: $urn}) RETURN n ORDER BY n.valid_from ASC",
			map[string]any{"urn": string(urn)},
		)
		if err != nil {
			return nil, err
		}
		out := []node.Node{}
		for rec.Next(ctx) {
			raw, _ := rec.Record().Get("n")
			nn, ok := raw.(neo4j.Node)
			if !ok {
				continue
			}
			n, err := decodeNode(nn.Props)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
		}
		return out, rec.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]node.Node), nil
}

// Delete fecha a versão corrente sem criar nova.
func (r *NodeRepo) Delete(ctx context.Context, urn node.URN) error {
	now := nowFn().UTC()
	_, err := r.withWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx,
			"MATCH (n:CeNode {urn: $urn}) WHERE n.valid_to IS NULL "+
				"SET n.valid_to = $now RETURN count(n) AS affected",
			map[string]any{"urn": string(urn), "now": now},
		)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
		}
		rec := res.Record()
		affected, _ := rec.Get("affected")
		if n, _ := affected.(int64); n == 0 {
			return nil, fmt.Errorf("%w: URN %q has no current version", repository.ErrNotFound, urn)
		}
		return nil, nil
	})
	return err
}

// Touch atualiza apenas observed_at na versão corrente sem versionar.
func (r *NodeRepo) Touch(ctx context.Context, urn node.URN) error {
	now := nowFn().UTC()
	_, err := r.withWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx,
			"MATCH (n:CeNode {urn: $urn}) WHERE n.valid_to IS NULL "+
				"SET n.observed_at = $now RETURN count(n) AS affected",
			map[string]any{"urn": string(urn), "now": now},
		)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
		}
		rec := res.Record()
		affected, _ := rec.Get("affected")
		if n, _ := affected.(int64); n == 0 {
			return nil, fmt.Errorf("%w: URN %q has no current version", repository.ErrNotFound, urn)
		}
		return nil, nil
	})
	return err
}

// Search retorna nós correntes cujo URN ou campos textuais visíveis
// contenham Q (case-insensitive). Filtra por Kind se preenchido.
// Determinismo: ORDER BY urn ASC. Limit zero → 100.
func (r *NodeRepo) Search(ctx context.Context, q repository.SearchQuery) ([]node.Node, error) {
	needle := strings.TrimSpace(q.Q)
	if needle == "" {
		return nil, fmt.Errorf("%w: empty Q", repository.ErrInvalidArgument)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	params := map[string]any{"needle": strings.ToLower(needle)}
	where := []string{"n.valid_to IS NULL"}
	if q.Kind != "" {
		where = append(where, "n.kind = $kind")
		params["kind"] = string(q.Kind)
	}
	// Match em URN, repo, module_path, external_id, tags_json.
	// Tags são persistidas como JSON blob (ver mapper.go: tags_json);
	// um substring match contra a serialização cobre `Name` e demais
	// chaves/valores no MVP. Caller filtra pós-fato se precisar de
	// match estrito por chave de tag.
	textMatch := "(" +
		"toLower(n.urn) CONTAINS $needle " +
		"OR toLower(coalesce(n.repo, '')) CONTAINS $needle " +
		"OR toLower(coalesce(n.module_path, '')) CONTAINS $needle " +
		"OR toLower(coalesce(n.external_id, '')) CONTAINS $needle " +
		"OR toLower(coalesce(n.tags_json, '')) CONTAINS $needle" +
		")"
	where = append(where, textMatch)

	cypher := "MATCH (n:CeNode) WHERE " + joinAnd(where) + " RETURN n ORDER BY n.urn ASC"
	if q.Offset > 0 {
		cypher += fmt.Sprintf(" SKIP %d", q.Offset)
	}
	cypher += fmt.Sprintf(" LIMIT %d", limit)

	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		out := []node.Node{}
		for rec.Next(ctx) {
			raw, _ := rec.Record().Get("n")
			nn, ok := raw.(neo4j.Node)
			if !ok {
				continue
			}
			n, err := decodeNode(nn.Props)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, rec.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]node.Node), nil
}

func buildListQuery(f repository.NodeFilter) (string, map[string]any) {
	params := map[string]any{}
	where := []string{}

	if f.AsOf.IsZero() {
		if !f.IncludeInactive {
			where = append(where, "n.valid_to IS NULL")
		}
		// IncludeInactive: sem cláusula → inclui versões fechadas.
	} else {
		where = append(where, "n.valid_from <= $at AND (n.valid_to IS NULL OR n.valid_to > $at)")
		params["at"] = f.AsOf.Time().UTC()
	}
	if f.Kind != "" {
		where = append(where, "n.kind = $kind")
		params["kind"] = string(f.Kind)
	}
	if f.Provider != "" {
		where = append(where, "n.provider = $provider")
		params["provider"] = string(f.Provider)
	}
	if f.AccountURN != "" {
		where = append(where, "n.account = $account")
		params["account"] = string(f.AccountURN)
	}
	if f.RegionURN != "" {
		where = append(where, "n.region = $region")
		params["region"] = string(f.RegionURN)
	}

	wc := ""
	if len(where) > 0 {
		wc = "WHERE " + joinAnd(where) + " "
	}
	q := "MATCH (n:CeNode) " + wc + "RETURN n ORDER BY n.urn ASC"
	if f.Offset > 0 {
		q += fmt.Sprintf(" SKIP %d", f.Offset)
	}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	return q, params
}

func joinAnd(parts []string) string {
	return strings.Join(parts, " AND ")
}
