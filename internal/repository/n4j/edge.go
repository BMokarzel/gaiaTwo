package n4j

import (
	"context"
	"errors"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// EdgeRepo implementa repository.EdgeRepository sobre Neo4j.
type EdgeRepo struct{ *Client }

// NewEdgeRepo embrulha um Client.
func NewEdgeRepo(c *Client) *EdgeRepo { return &EdgeRepo{c} }

// Compile-time interface check.
var _ repository.EdgeRepository = (*EdgeRepo)(nil)

// Upsert versiona uma aresta. Valida a matriz de adjacência antes de
// persistir.
func (r *EdgeRepo) Upsert(ctx context.Context, e edge.Edge, fromKind, toKind node.Kind) error {
	if e == nil || e.ID() == "" {
		return fmt.Errorf("%w: nil edge or empty ID", repository.ErrInvalidArgument)
	}
	if err := edge.Validate(e, fromKind, toKind); err != nil {
		return fmt.Errorf("%w: %v", repository.ErrInvalidArgument, err)
	}
	props := edgeProps(e)
	now := nowFn().UTC()

	_, err := r.withWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Fecha versão corrente da aresta com o mesmo id, se houver.
		_, err := tx.Run(ctx,
			"MATCH ()-[r:CE_EDGE {id: $id}]-() WHERE r.valid_to IS NULL SET r.valid_to = $now",
			map[string]any{"id": e.ID(), "now": now},
		)
		if err != nil {
			return nil, err
		}
		// Cria nova versão entre os dois nós correntes.
		_, err = tx.Run(ctx,
			"MATCH (a:CeNode {urn: $from}), (b:CeNode {urn: $to}) "+
				"WHERE a.valid_to IS NULL AND b.valid_to IS NULL "+
				"CREATE (a)-[r:CE_EDGE]->(b) SET r += $props",
			map[string]any{
				"from":  string(e.From()),
				"to":    string(e.To()),
				"props": props,
			},
		)
		return nil, err
	})
	return err
}

// Between retorna arestas entre dois nós (current ou as-of).
func (r *EdgeRepo) Between(ctx context.Context, from, to node.URN, f repository.EdgeFilter) ([]edge.Edge, error) {
	q, params := buildBetweenQuery(from, to, f)
	return r.runEdgeQuery(ctx, q, params)
}

// Neighbors retorna arestas incidentes a urn.
func (r *EdgeRepo) Neighbors(ctx context.Context, urn node.URN, dir repository.Direction, f repository.EdgeFilter) ([]edge.Edge, error) {
	q, params := buildNeighborsQuery(urn, dir, f)
	return r.runEdgeQuery(ctx, q, params)
}

// Traverse executa BFS via Cypher pattern com profundidade.
func (r *EdgeRepo) Traverse(ctx context.Context, start node.URN, dir repository.Direction, depth int, f repository.EdgeFilter) ([]node.URN, error) {
	if depth <= 0 {
		return nil, fmt.Errorf("%w: depth must be > 0", repository.ErrInvalidArgument)
	}
	q, params := buildTraverseQuery(start, dir, depth, f)
	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, q, params)
		if err != nil {
			return nil, err
		}
		out := []node.URN{}
		for rec.Next(ctx) {
			urn, _ := rec.Record().Get("urn")
			s, _ := urn.(string)
			if s != "" {
				out = append(out, node.URN(s))
			}
		}
		return out, rec.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]node.URN), nil
}

// Paths enumera caminhos do nó from para o nó to com no máximo maxHops
// saltos. Cypher usa a expansão variável `-[:CE_EDGE*1..maxHops]->` e
// filtra arestas correntes/visíveis em AsOf. Cap em 100 caminhos.
// maxHops > 5 retorna ErrInvalidArgument.
func (r *EdgeRepo) Paths(ctx context.Context, from, to node.URN, maxHops int, f repository.EdgeFilter) ([][]node.URN, error) {
	if maxHops <= 0 {
		return nil, fmt.Errorf("%w: maxHops must be > 0", repository.ErrInvalidArgument)
	}
	if maxHops > 5 {
		return nil, fmt.Errorf("%w: maxHops capped at 5 (combinatorial blast)", repository.ErrInvalidArgument)
	}
	if from == "" || to == "" {
		return nil, fmt.Errorf("%w: from/to required", repository.ErrInvalidArgument)
	}
	if from == to {
		return [][]node.URN{{from}}, nil
	}

	params := map[string]any{"from": string(from), "to": string(to)}
	timeFilter := "ALL(x IN relationships(p) WHERE x.valid_to IS NULL)"
	if !f.AsOf.IsZero() {
		timeFilter = "ALL(x IN relationships(p) WHERE x.valid_from <= $at AND (x.valid_to IS NULL OR x.valid_to > $at))"
		params["at"] = f.AsOf.Time().UTC()
	}
	where := []string{timeFilter}
	if len(f.Types) > 0 {
		where = append(where, "ALL(x IN relationships(p) WHERE x.type IN $types)")
		params["types"] = typeStrings(f.Types)
	}

	cypher := fmt.Sprintf(
		"MATCH p=(a:CeNode {urn: $from})-[:CE_EDGE*1..%d]->(b:CeNode {urn: $to}) "+
			"WHERE %s "+
			"RETURN [n IN nodes(p) | n.urn] AS urns, length(p) AS len "+
			"ORDER BY len ASC, urns ASC "+
			"LIMIT 100",
		maxHops, joinAnd(where),
	)

	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		out := [][]node.URN{}
		for rec.Next(ctx) {
			raw, _ := rec.Record().Get("urns")
			arr, ok := raw.([]any)
			if !ok {
				continue
			}
			path := make([]node.URN, 0, len(arr))
			for _, v := range arr {
				if s, ok := v.(string); ok {
					path = append(path, node.URN(s))
				}
			}
			if len(path) > 0 {
				out = append(out, path)
			}
		}
		return out, rec.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([][]node.URN), nil
}

// Delete fecha a versão corrente de uma aresta.
func (r *EdgeRepo) Delete(ctx context.Context, id string) error {
	now := nowFn().UTC()
	_, err := r.withWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx,
			"MATCH ()-[r:CE_EDGE {id: $id}]-() WHERE r.valid_to IS NULL "+
				"SET r.valid_to = $now RETURN count(r) AS affected",
			map[string]any{"id": id, "now": now},
		)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("%w: edge %q", repository.ErrNotFound, id)
		}
		affected, _ := res.Record().Get("affected")
		if n, _ := affected.(int64); n == 0 {
			return nil, fmt.Errorf("%w: edge %q has no current version", repository.ErrNotFound, id)
		}
		return nil, nil
	})
	return err
}

// ----------------------------------------------------------------------------
// query builders
// ----------------------------------------------------------------------------

func runEdgeQueryProps(ctx context.Context, rec neo4j.ResultWithContext) ([]edge.Edge, error) {
	out := []edge.Edge{}
	for rec.Next(ctx) {
		raw, _ := rec.Record().Get("r")
		rel, ok := raw.(neo4j.Relationship)
		if !ok {
			return nil, errors.New("n4j: unexpected return shape")
		}
		e, err := decodeEdge(rel.Props)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rec.Err()
}

func (r *EdgeRepo) runEdgeQuery(ctx context.Context, q string, params map[string]any) ([]edge.Edge, error) {
	res, err := r.withRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rec, err := tx.Run(ctx, q, params)
		if err != nil {
			return nil, err
		}
		return runEdgeQueryProps(ctx, rec)
	})
	if err != nil {
		return nil, err
	}
	return res.([]edge.Edge), nil
}

func buildBetweenQuery(from, to node.URN, f repository.EdgeFilter) (string, map[string]any) {
	params := map[string]any{"from": string(from), "to": string(to)}
	where := []string{}
	if f.AsOf.IsZero() {
		if !f.IncludeInactive {
			where = append(where, "r.valid_to IS NULL")
		}
	} else {
		where = append(where, "r.valid_from <= $at AND (r.valid_to IS NULL OR r.valid_to > $at)")
		params["at"] = f.AsOf.Time().UTC()
	}
	if len(f.Types) > 0 {
		where = append(where, "r.type IN $types")
		params["types"] = typeStrings(f.Types)
	}
	q := "MATCH (a:CeNode {urn: $from})-[r:CE_EDGE]->(b:CeNode {urn: $to}) " +
		whereClause(where) + "RETURN r ORDER BY r.id ASC"
	q += paginate(f.Offset, f.Limit)
	return q, params
}

func buildNeighborsQuery(urn node.URN, dir repository.Direction, f repository.EdgeFilter) (string, map[string]any) {
	params := map[string]any{"urn": string(urn)}
	where := []string{}
	if f.AsOf.IsZero() {
		if !f.IncludeInactive {
			where = append(where, "r.valid_to IS NULL")
		}
	} else {
		where = append(where, "r.valid_from <= $at AND (r.valid_to IS NULL OR r.valid_to > $at)")
		params["at"] = f.AsOf.Time().UTC()
	}
	if len(f.Types) > 0 {
		where = append(where, "r.type IN $types")
		params["types"] = typeStrings(f.Types)
	}

	var pattern string
	switch dir {
	case repository.DirOut:
		pattern = "MATCH (n:CeNode {urn: $urn})-[r:CE_EDGE]->()"
	case repository.DirIn:
		pattern = "MATCH (n:CeNode {urn: $urn})<-[r:CE_EDGE]-()"
	default:
		pattern = "MATCH (n:CeNode {urn: $urn})-[r:CE_EDGE]-()"
	}
	q := pattern + " " + whereClause(where) + "RETURN r ORDER BY r.id ASC"
	q += paginate(f.Offset, f.Limit)
	return q, params
}

func buildTraverseQuery(start node.URN, dir repository.Direction, depth int, f repository.EdgeFilter) (string, map[string]any) {
	params := map[string]any{"start": string(start)}
	rel := fmt.Sprintf("[r:CE_EDGE*1..%d]", depth)
	var pattern string
	switch dir {
	case repository.DirOut:
		pattern = "MATCH (s:CeNode {urn: $start})-" + rel + "->(t:CeNode)"
	case repository.DirIn:
		pattern = "MATCH (s:CeNode {urn: $start})<-" + rel + "-(t:CeNode)"
	default:
		pattern = "MATCH (s:CeNode {urn: $start})-" + rel + "-(t:CeNode)"
	}
	where := []string{"ALL(x IN r WHERE x.valid_to IS NULL)"}
	if !f.AsOf.IsZero() {
		where = []string{"ALL(x IN r WHERE x.valid_from <= $at AND (x.valid_to IS NULL OR x.valid_to > $at))"}
		params["at"] = f.AsOf.Time().UTC()
	}
	if len(f.Types) > 0 {
		where = append(where, "ALL(x IN r WHERE x.type IN $types)")
		params["types"] = typeStrings(f.Types)
	}
	q := pattern + " WHERE " + joinAnd(where) + " RETURN DISTINCT t.urn AS urn"
	return q, params
}

// whereClause monta " WHERE …" se houver cláusulas; senão devolve "".
// Necessário porque IncludeInactive sem filtros pode esvaziar a lista.
func whereClause(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return "WHERE " + joinAnd(parts) + " "
}

func typeStrings(ts []edge.Type) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}

func paginate(offset, limit int) string {
	s := ""
	if offset > 0 {
		s += fmt.Sprintf(" SKIP %d", offset)
	}
	if limit > 0 {
		s += fmt.Sprintf(" LIMIT %d", limit)
	}
	return s
}
