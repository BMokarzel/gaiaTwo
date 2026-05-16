package controller

import (
	"net/http"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
	"costEngine/internal/platform/httpserver"
)

// squads.go agrupa os handlers de /v1/squads.
//
//	GET /v1/squads/{urn}                — detalhe
//	GET /v1/squads/{urn}/members        — pessoas do squad

// dispatchSquad despacha `/v1/squads/{urn}[/members]`.
func (c *Controller) dispatchSquad(w http.ResponseWriter, r *http.Request) {
	urn, suffix, ok := splitOrgPath(r, []string{"/members"})
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch suffix {
	case "/members":
		c.handleSquadMembers(w, r, urn)
	default:
		c.handleGetSquad(w, r, urn)
	}
}

// handleGetSquad atende GET /v1/squads/{urn}.
func (c *Controller) handleGetSquad(w http.ResponseWriter, r *http.Request, urn node.URN) {
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	d, err := c.svc.GetSquad(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, squadDetailView(d))
}

// handleSquadMembers atende GET /v1/squads/{urn}/members.
//
// PII: emails saem como EmailHint (mascarado em ingest F-010); o
// repositório nunca persiste email pleno.
func (c *Controller) handleSquadMembers(w http.ResponseWriter, r *http.Request, squad node.URN) {
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
	fp := fingerprintOrg("squad-members", string(squad), opts.AsOf)
	cur, err := httpserver.DecodeCursor(q.Get("cursor"), fp)
	if err != nil {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "%s", err.Error()))
		return
	}
	page, err := c.svc.ListSquadMembers(r.Context(), squad, org.ListMembersQuery{
		AsOfOptions: opts,
		Limit:       limit,
		Offset:      cur.Offset,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, pp := range page.Items {
		// Em members lista, TeamURN é elidido (contexto squad-local).
		out = append(out, personProfileView(org.PersonProfile{Person: pp.Person}))
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"results":     out,
		"limit":       page.Limit,
		"next_cursor": httpserver.EncodeCursor(httpserver.Cursor{Offset: page.NextOffset, Fingerprint: fp}),
	})
}
