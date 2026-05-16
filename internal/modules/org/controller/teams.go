package controller

import (
	"net/http"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
	"costEngine/internal/platform/httpserver"
)

// teams.go agrupa os handlers que respondem em /v1/teams.
//
//	GET /v1/teams                       — list paginada
//	GET /v1/teams/{urn}                 — detalhe
//	GET /v1/teams/{urn}/squads          — squads do team
//
// Padrão: parse → svc → view. Erros vão por httpserver.WriteError, que
// mapeia *org.ErrTeamNotFound, *httpserver.genericHTTPError, etc.

// dispatchTeam despacha `/v1/teams/{urn}[/squads]`. URNs contêm `/`
// e `:`, então o roteador Go 1.22 captura como wildcard único e o
// controller reparte por sufixo conhecido.
func (c *Controller) dispatchTeam(w http.ResponseWriter, r *http.Request) {
	urn, suffix, ok := splitOrgPath(r, []string{"/squads"})
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch suffix {
	case "/squads":
		c.handleTeamSquads(w, r, urn)
	default:
		c.handleGetTeam(w, r, urn)
	}
}

// handleListTeams atende GET /v1/teams.
//
// Query: limit (default 50, máx 200), cursor, as_of, include_inactive.
// Counts são derivados pela service via traversal — controller só
// projeta. Resposta: { results, limit, next_cursor }.
func (c *Controller) handleListTeams(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"), defaultOrgListLimit, maxOrgListLimit)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	opts, err := parseAsOfOptions(q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	fp := fingerprintOrg("teams", "", opts.AsOf)
	cur, err := httpserver.DecodeCursor(q.Get("cursor"), fp)
	if err != nil {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "%s", err.Error()))
		return
	}

	page, err := c.svc.ListTeams(r.Context(), org.ListTeamsQuery{
		AsOfOptions: opts,
		Limit:       limit,
		Offset:      cur.Offset,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, ts := range page.Items {
		out = append(out, teamSummaryView(ts, nil))
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"results":     out,
		"limit":       page.Limit,
		"next_cursor": httpserver.EncodeCursor(httpserver.Cursor{Offset: page.NextOffset, Fingerprint: fp}),
	})
}

// handleGetTeam atende GET /v1/teams/{urn}.
func (c *Controller) handleGetTeam(w http.ResponseWriter, r *http.Request, urn node.URN) {
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	d, err := c.svc.GetTeam(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, teamDetailView(d))
}

// handleTeamSquads atende GET /v1/teams/{urn}/squads.
//
// 404 (ErrTeamNotFound) é emitido pela service quando o team não
// existe — sem confusão com "team existe mas tem 0 squads".
func (c *Controller) handleTeamSquads(w http.ResponseWriter, r *http.Request, team node.URN) {
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"), defaultOrgListLimit, maxOrgListLimit)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	opts, err := parseAsOfOptions(q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	fp := fingerprintOrg("team-squads", string(team), opts.AsOf)
	cur, err := httpserver.DecodeCursor(q.Get("cursor"), fp)
	if err != nil {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "%s", err.Error()))
		return
	}
	page, err := c.svc.ListSquadsOfTeam(r.Context(), team, org.ListSquadsQuery{
		AsOfOptions: opts,
		Limit:       limit,
		Offset:      cur.Offset,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, sd := range page.Items {
		out = append(out, squadDetailView(sd))
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"results":     out,
		"limit":       page.Limit,
		"next_cursor": httpserver.EncodeCursor(httpserver.Cursor{Offset: page.NextOffset, Fingerprint: fp}),
	})
}
