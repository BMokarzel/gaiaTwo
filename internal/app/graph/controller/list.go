package controller

import (
	"net/http"

	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500
)

// handleListNodes atende GET /v1/architecture/nodes.
//
// Caso de uso primário: front-end pedir "todos os Services" ou "todos
// os Endpoints" para popular listas. Search (`/v1/architecture/search`)
// exige `q` não-vazio, então listagem por kind precisa deste path.
//
// Query:
//   - kind=KindName (opcional; se vazio retorna todos os kinds, com
//     cuidado: pode ser caro em grafos grandes — cap rígido de 500)
//   - limit=N (default 100, cap 500)
//   - offset=N (default 0; paginação simples offset-based)
//   - as_of=RFC3339
//
// Resposta:
//
//	{ "results": [ ...nodeView ], "limit": N, "offset": N, "count": M }
//
// `count` = quantos retornados (≤ limit). Sem `next_cursor` —
// paginação opaca não justifica complexidade aqui.
func (c *Controller) handleListNodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, err := parseLimit(q.Get("limit"), defaultListLimit, maxListLimit)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	offset := 0
	if s := q.Get("offset"); s != "" {
		n, perr := strconvAtoiPositive(s)
		// offset == 0 também é válido — strconvAtoiPositive recusa 0,
		// então tratamos manualmente.
		if perr != nil && s != "0" {
			httpserver.WriteError(w, r,
				httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid offset"))
			return
		}
		offset = n
	}
	as, err := parseAsOf(q.Get("as_of"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	filter := repository.NodeFilter{
		Kind:   node.Kind(q.Get("kind")),
		AsOf:   as,
		Limit:  limit,
		Offset: offset,
	}
	nodes, err := c.nodes.List(r.Context(), filter)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeView(n))
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"results": out,
		"limit":   limit,
		"offset":  offset,
		"count":   len(out),
	})
}
