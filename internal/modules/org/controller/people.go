package controller

import (
	"net/http"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
	"costEngine/internal/platform/httpserver"
)

// people.go agrupa os handlers de /v1/people.
//
//	GET /v1/people/{urn}                — perfil (PII ofuscada)
//	GET /v1/people/{urn}/reports        — árvore de reports

// dispatchPerson despacha `/v1/people/{urn}[/reports]`.
func (c *Controller) dispatchPerson(w http.ResponseWriter, r *http.Request) {
	urn, suffix, ok := splitOrgPath(r, []string{"/reports"})
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch suffix {
	case "/reports":
		c.handlePersonReports(w, r, urn)
	default:
		c.handleGetPerson(w, r, urn)
	}
}

// handleGetPerson atende GET /v1/people/{urn}.
//
// Devolve perfil com team_urn transitivo (derivado pelo service via
// squad.TeamURN — F-010 modela como propriedade, evita traversal extra).
func (c *Controller) handleGetPerson(w http.ResponseWriter, r *http.Request, urn node.URN) {
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	prof, err := c.svc.GetPerson(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, personProfileView(prof))
}

// handlePersonReports atende GET /v1/people/{urn}/reports.
//
// Query: depth (1..3; default 1), as_of, include_inactive.
//
// Cycle defense fica na service (visited set global). Truncated=true
// na resposta indica que algum ramo foi cortado por ciclo detectado.
func (c *Controller) handlePersonReports(w http.ResponseWriter, r *http.Request, root node.URN) {
	q := r.URL.Query()
	depth, err := parseReportsDepth(q.Get("depth"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	opts, err := parseAsOfOptions(q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	tree, err := c.svc.ListReports(r.Context(), root, org.ListReportsQuery{
		AsOfOptions: opts,
		Depth:       depth,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, reportsTreeView(tree))
}
