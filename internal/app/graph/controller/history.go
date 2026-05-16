package controller

import (
	"net/http"
	"sort"

	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
)

// handleHistory atende GET /v1/architecture/nodes/{urn}/history.
//
// Query:
//   - limit=N (default 50, máx 500). Ordem: cronológica decrescente
//     (mais recente primeiro), conforme F-014.
func (c *Controller) handleHistory(w http.ResponseWriter, r *http.Request, urn node.URN) {
	limit, err := parseLimit(r.URL.Query().Get("limit"), defaultHistoryLimit, maxHistoryLimit)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	hist, err := c.nodes.History(r.Context(), urn)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	// Repo devolve ordem ValidFrom ASC; F-014 pede DESC.
	sort.SliceStable(hist, func(i, j int) bool {
		return hist[i].Meta().ValidFrom.After(hist[j].Meta().ValidFrom)
	})
	if limit < len(hist) {
		hist = hist[:limit]
	}
	out := make([]any, 0, len(hist))
	for _, n := range hist {
		out = append(out, nodeView(n))
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"versions": out})
}
