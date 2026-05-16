// Package repository define os contratos de persistência do grafo de
// inteligência arquitetural.
//
// Há duas interfaces principais — NodeRepository e EdgeRepository — e um
// pequeno conjunto de erros sentinela. Implementações vivem em
// subpacotes (memory, n4j).
//
// Todas as operações são bitemporais: upsert nunca destrói versões
// anteriores; sempre fecha (ValidTo = now) a versão corrente e cria uma
// nova. Consultas têm uma variante "as-of" para inspecionar o passado.
package repository

import (
	"context"
	"errors"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Sentinel errors. Implementações devem usá-los (ou envolvê-los com
// fmt.Errorf("...: %w", ErrNotFound)) para que callers possam usar
// errors.Is.
var (
	ErrNotFound        = errors.New("repository: not found")
	ErrInvalidArgument = errors.New("repository: invalid argument")
	ErrConflict        = errors.New("repository: conflict")

	// ErrAmbiguous indica que um lookup matcheou múltiplos candidatos
	// quando esperava-se um único (ex.: GetByExternalID com account=""
	// retornando 2 URNs em contas diferentes — F-003 / S-005).
	ErrAmbiguous = errors.New("repository: ambiguous match")
)

// AsOf indica que tempo lógico ("validTime") usar em consultas.
// Zero value (time.Time{}) significa "estado corrente" (ValidTo IS NULL).
type AsOf time.Time

// IsZero reporta se este AsOf representa o estado corrente.
func (a AsOf) IsZero() bool { return time.Time(a).IsZero() }

// Time retorna o time.Time subjacente.
func (a AsOf) Time() time.Time { return time.Time(a) }

// NodeFilter parametriza ListNodes.
type NodeFilter struct {
	Kind       node.Kind       // se vazio, todos os kinds
	Provider   node.ProviderID // se vazio, todos os provedores
	AccountURN node.URN        // se vazio, todas as contas
	RegionURN  node.URN        // se vazio, todas as regiões
	Labels     []string        // AND lógico sobre Meta.Labels
	AsOf       AsOf            // se zero, corrente
	// IncludeInactive, quando true e AsOf zero, inclui também versões
	// já fechadas (ValidTo != nil) — a última versão de cada URN é
	// devolvida. Útil para listar ex-funcionários/recursos
	// decomissionados ainda referenciáveis (F-015 D9).
	IncludeInactive bool
	Limit           int // 0 = sem limite (cuidado)
	Offset          int
}

// SearchQuery parametriza Search. Match é substring case-insensitive
// sobre URN e campos textuais visíveis do nó (Repo/ModulePath para
// Service, ExternalID/Name tag para Resource).
type SearchQuery struct {
	Q      string    // string de busca; vazia retorna ErrInvalidArgument
	Kind   node.Kind // se vazio, todos os kinds
	Limit  int       // 0 → default impl (memory cap em 100)
	Offset int
}

// NodeRepository persiste e consulta nós do grafo.
type NodeRepository interface {
	// Upsert insere ou versiona um nó. Se já existir um corrente com a
	// mesma URN, fecha-o (ValidTo = now) e cria uma nova versão.
	// Idempotente quando a versão a inserir é idêntica à corrente.
	Upsert(ctx context.Context, n node.Node) error

	// GetByURN retorna a versão corrente (ou a as-of) de um nó.
	// Retorna ErrNotFound se não houver versão visível.
	GetByURN(ctx context.Context, urn node.URN, as AsOf) (node.Node, error)

	// GetByExternalID resolve um identificador nativo do provedor para a
	// URN canônica. É o hot path do CUR ingestor (F-003).
	//
	// Semântica:
	//   - account != "" → match estrito (provider, account, externalID).
	//   - account == "" → wildcard cross-account; retorna ErrAmbiguous
	//     se mais de uma URN matchea (provider, externalID).
	//   - externalID é normalizado: se o nó foi indexado por ARN, lookup
	//     por short ID (ex.: "i-abc" derivado do ARN) também resolve;
	//     o oposto também vale se a impl tiver alias bidirecional.
	//   - AsOf zero → versão corrente; caso contrário, exige que exista
	//     versão visível em AsOf (valid_from ≤ t < valid_to).
	GetByExternalID(ctx context.Context, p node.ProviderID, account, externalID string, as AsOf) (node.URN, error)

	// List retorna nós que satisfazem o filtro.
	List(ctx context.Context, f NodeFilter) ([]node.Node, error)

	// History retorna todas as versões de uma URN, ordenadas por ValidFrom
	// crescente.
	History(ctx context.Context, urn node.URN) ([]node.Node, error)

	// Delete marca a versão corrente como fechada (ValidTo = now) sem
	// criar nova. Útil quando um recurso é decomissionado e não substituído.
	Delete(ctx context.Context, urn node.URN) error

	// Search retorna nós correntes cujo URN ou campos textuais visíveis
	// contenham Q (case-insensitive). Filtra por Kind se preenchido.
	// Determinismo: ordem por URN crescente. Resultado é stateless —
	// não há cursor opaco (paginação externa de offset basta no MVP).
	Search(ctx context.Context, q SearchQuery) ([]node.Node, error)

	// Touch atualiza apenas o ObservedAt da versão corrente para now,
	// sem versionar (não fecha valid_to nem cria nova versão). É a
	// operação que coletores chamam quando reconhecem que um recurso
	// observado já existe no grafo *sem mudanças significativas* — só
	// reafirmam que continuam vendo o mesmo estado.
	//
	// Retorna ErrNotFound se não houver versão corrente.
	Touch(ctx context.Context, urn node.URN) error
}

// EdgeFilter parametriza ListEdges / Neighbors.
type EdgeFilter struct {
	Types []edge.Type
	AsOf  AsOf
	// IncludeInactive, quando true e AsOf zero, inclui versões
	// fechadas (ValidTo != nil); usa-se para reconstituir adjacências
	// históricas (ex.: pessoas que estavam num squad mas saíram).
	IncludeInactive bool
	Limit           int
	Offset          int
}

// Direction descreve qual lado da aresta a query considera.
type Direction int

const (
	DirOut Direction = iota // From → To (saindo do nó)
	DirIn                   // To → From (entrando no nó)
	DirAny                  // ambos (ignora direção)
)

// EdgeRepository persiste e consulta arestas do grafo.
type EdgeRepository interface {
	// Upsert insere ou versiona uma aresta. Mesma semântica bitemporal de
	// NodeRepository.Upsert. Deve validar com edge.Validate.
	Upsert(ctx context.Context, e edge.Edge, fromKind, toKind node.Kind) error

	// Between retorna arestas (correntes ou as-of) entre dois nós.
	Between(ctx context.Context, from, to node.URN, f EdgeFilter) ([]edge.Edge, error)

	// Neighbors retorna arestas incidentes a 'urn' na direção indicada.
	Neighbors(ctx context.Context, urn node.URN, dir Direction, f EdgeFilter) ([]edge.Edge, error)

	// Traverse executa BFS limitada por profundidade a partir de 'urn',
	// retornando todos os nós alcançados (sem ciclos). 'depth' = 1 retorna
	// vizinhos diretos.
	Traverse(ctx context.Context, urn node.URN, dir Direction, depth int, f EdgeFilter) ([]node.URN, error)

	// Paths enumera caminhos do nó from para o nó to com no máximo
	// maxHops saltos. Retorna lista de listas de URNs (cada lista é um
	// caminho começando em from e terminando em to, incluindo
	// extremos). Determinismo: caminhos ordenados por (tamanho asc,
	// URNs lexicograficamente). Implementação MVP enumera até 100
	// caminhos; estoura erro se maxHops > 5 (proteção contra explosão
	// combinatória).
	Paths(ctx context.Context, from, to node.URN, maxHops int, f EdgeFilter) ([][]node.URN, error)

	// Delete fecha a versão corrente da aresta.
	Delete(ctx context.Context, id string) error
}
