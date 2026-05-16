package controller

import (
	"net/http"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

// handlePaths atende GET /v1/architecture/paths.
//
// Query:
//   - from=URN (obrigatório)
//   - to=URN   (obrigatório)
//   - max_hops=N (default 3, cap 5)
//   - edge_types=CSV (opcional)
//   - as_of=RFC3339 (opcional)
//
// Resposta:
//
//	{
//	  "paths": [
//	    {"hops": N, "urns": ["urn:a", "urn:b", ...]},
//	    ...
//	  ]
//	}
//
// Caminhos vêm do repo em ordem (tamanho asc, URNs lexicograficamente).
func (c *Controller) handlePaths(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	if from == "" || to == "" {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "from and to required"))
		return
	}
	hops, err := parseHops(q.Get("max_hops"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	as, err := parseAsOf(q.Get("as_of"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	filter := repository.EdgeFilter{
		Types: parseEdgeTypes(q.Get("edge_types")),
		AsOf:  as,
	}
	paths, err := c.edges.Paths(r.Context(), node.URN(from), node.URN(to), hops, filter)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		out = append(out, map[string]any{
			"hops": len(p) - 1, // arestas = nós - 1
			"urns": p,
		})
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"paths": out})
}
