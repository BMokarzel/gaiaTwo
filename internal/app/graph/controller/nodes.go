package controller

import (
	"net/http"
	"net/url"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
)

// dispatchNode atende `/v1/architecture/nodes/{rest...}` despachando
// pelo sufixo (`/neighbors`, `/history`) ou caindo no GET puro do nó.
// PathValue já desfaz percent-encoding; ainda assim normalizamos `%2F`
// para clientes que escolham encodar.
func (c *Controller) dispatchNode(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	if dec, err := url.PathUnescape(rest); err == nil {
		rest = dec
	}
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "missing URN"))
		return
	}
	switch {
	case strings.HasSuffix(rest, "/neighbors"):
		c.handleNeighbors(w, r, node.URN(strings.TrimSuffix(rest, "/neighbors")))
	case strings.HasSuffix(rest, "/history"):
		c.handleHistory(w, r, node.URN(strings.TrimSuffix(rest, "/history")))
	case strings.HasSuffix(rest, "/flow"):
		c.handleFlow(w, r, node.URN(strings.TrimSuffix(rest, "/flow")))
	default:
		c.handleGetNode(w, r, node.URN(rest))
	}
}

// handleGetNode atende GET /v1/architecture/nodes/{urn}.
//
// Query:
//   - as_of=RFC3339 → versão vigente naquela data (default: corrente).
func (c *Controller) handleGetNode(w http.ResponseWriter, r *http.Request, urn node.URN) {
	as, err := parseAsOf(r.URL.Query().Get("as_of"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	n, err := c.nodes.GetByURN(r.Context(), urn, as)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, nodeView(n))
}
