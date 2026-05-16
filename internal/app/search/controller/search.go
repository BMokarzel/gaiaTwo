package controller

import (
	"net/http"
	"strconv"
	"strings"

	"costEngine/internal/app/search"
	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
)

// Caps F-014.
const (
	defaultLimit   = 50
	maxLimit       = 200
	codeBadRequest = "bad_request"
)

// handleSearch atende GET /v1/architecture/search.
//
// Query:
//   - q=string (obrigatório)
//   - kind=KindName (opcional; filtro estrito)
//   - limit=N (default 50, cap 200)
//   - cursor=opaque (continuação de paginação)
//
// Paginação cursor: primeira request manda só q/kind/limit. Resposta
// carrega `next_cursor` se há mais resultados; consumer reenvia
// idêntico com `cursor=<valor>`. Mudar q/kind invalida cursor
// (fingerprint) — devolve 400.
//
// Resposta:
//
//	{
//	  "results":     [ ...nodeView ],
//	  "limit":       N,
//	  "offset":      N,
//	  "next_cursor": "..." | ""
//	}
func (c *Controller) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	needle := strings.TrimSpace(q.Get("q"))
	if needle == "" {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "missing q"))
		return
	}
	kind := strings.TrimSpace(q.Get("kind"))
	limit, err := parseLimit(q.Get("limit"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	fp := httpserver.FingerprintSearch(needle, kind)
	cur, err := httpserver.DecodeCursor(q.Get("cursor"), fp)
	if err != nil {
		httpserver.WriteError(w, r,
			httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "%s", err.Error()))
		return
	}

	// Pedimos limit+1 ao port para detectar se há "próxima página"
	// sem requerer um count separado.
	sq := search.SearchQuery{
		Q:      needle,
		Kind:   node.Kind(kind),
		Limit:  limit + 1,
		Offset: cur.Offset,
	}
	results, err := c.searcher.Search(r.Context(), sq)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	var nextCursor string
	if len(results) > limit {
		results = results[:limit]
		nextCursor = httpserver.EncodeCursor(httpserver.Cursor{
			Offset:      cur.Offset + limit,
			Fingerprint: fp,
		})
	}
	out := make([]any, 0, len(results))
	for _, n := range results {
		out = append(out, nodeView(n))
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"results":     out,
		"limit":       limit,
		"offset":      cur.Offset,
		"next_cursor": nextCursor,
	})
}

// parseLimit valida `limit` (default 50, cap 200). 400 em valor
// inválido; valores acima do cap são silenciosamente truncados.
func parseLimit(s string) (int, error) {
	if s == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid limit: must be positive integer")
	}
	if n > maxLimit {
		n = maxLimit
	}
	return n, nil
}

// nodeView serializa um nó para o JSON de /search. Inline aqui (não
// importado do graph) para que search não dependa de outro app
// package; é uma view pequena e estável.
func nodeView(n node.Node) map[string]any {
	m := n.Meta()
	v := map[string]any{
		"urn":         n.URN(),
		"kind":        n.Kind(),
		"version":     m.Version,
		"valid_from":  m.ValidFrom,
		"observed_at": m.ObservedAt,
		"confidence":  m.Confidence,
		"data":        n,
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
	}
	if len(m.Labels) > 0 {
		v["labels"] = m.Labels
	}
	return v
}
