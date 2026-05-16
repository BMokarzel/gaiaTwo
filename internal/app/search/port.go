// Package search é o "search plane" da API REST (F-014/F-016 S-009).
//
// Hospeda apenas o endpoint `/v1/architecture/search`, que devolve
// nós correntes cujo URN ou campos textuais visíveis casam com a
// query (case-insensitive). Não opera sobre arestas nem materializa
// nada além do que o port `NodeSearcher` devolve.
//
// Por que um package próprio (e não `app/graph/controller`):
//   - cross-kind, mas a forma de uso é diferente (filtragem textual
//     + paginação cursor; sem hops, sem polimorfismo de view).
//   - tem políticas próprias (`defaultLimit`, `maxLimit`, fingerprint
//     de cursor) que não pertencem ao graph plane.
//   - usa o "port-set" pattern: consumer-owned outbound port narrow,
//     em vez de depender do `repository.NodeRepository` inteiro.
//
// Imports permitidos: stdlib, `internal/entity/node`,
// `internal/platform/httpserver`. NÃO importa `internal/repository`.
package search

import (
	"context"

	"costEngine/internal/entity/node"
)

// NodeSearcher é o port narrow consumido por este package — apenas
// o que `/search` precisa do mundo de leitura de nós. Implementado
// trivialmente por `repository.NodeRepository` (Go é estrutural;
// nenhum adaptador necessário).
type NodeSearcher interface {
	Search(ctx context.Context, q SearchQuery) ([]node.Node, error)
}

// SearchQuery é uma cópia tipada da `repository.SearchQuery` para
// que este package não importe `repository`. O assembler em
// `cmd/api/main.go` passa o `NodeRepository` direto: como Go usa
// structural typing e os campos batem, a chamada compila — mas o
// search nunca depende do package repository em build-time.
type SearchQuery struct {
	Q      string
	Kind   node.Kind
	Limit  int
	Offset int
}
