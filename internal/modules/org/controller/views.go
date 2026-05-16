package controller

import (
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
)

// Helpers de projeção JSON. Convertem types do módulo org em
// map[string]any com o shape estável já consumido pelos clients
// (F-015). Mantemos o shape para não breakar consumers — a mudança
// breaking de F-016 é apenas no body de erro (RFC 7807).
//
// Determinismo: a service garante ordem por URN onde aplicável; aqui
// só serializamos.

func teamSummaryView(t org.TeamSummary, squads []node.URN) map[string]any {
	m := t.Team.Meta()
	v := map[string]any{
		"urn":          t.Team.URN(),
		"kind":         t.Team.Kind(),
		"tenant":       t.Team.Tenant,
		"slug":         t.Team.Slug,
		"name":         t.Team.Name,
		"squad_count":  t.SquadCount,
		"person_count": t.PersonCount,
		"valid_from":   m.ValidFrom,
		"observed_at":  m.ObservedAt,
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
	}
	if squads != nil {
		v["squads"] = squads
	}
	return v
}

func teamDetailView(d org.TeamDetail) map[string]any {
	return teamSummaryView(d.TeamSummary, d.Squads)
}

func squadDetailView(d org.SquadDetail) map[string]any {
	sq := d.Squad
	m := sq.Meta()
	v := map[string]any{
		"urn":          sq.URN(),
		"kind":         sq.Kind(),
		"tenant":       sq.Tenant,
		"slug":         sq.Slug,
		"name":         sq.Name,
		"team_urn":     sq.TeamURN,
		"member_count": d.MemberCount,
		"valid_from":   m.ValidFrom,
		"observed_at":  m.ObservedAt,
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
	}
	return v
}

// personView projeta um PersonProfile. teamURN vem do profile (derivado
// pela service) — passar string vazia em endpoints onde a transitividade
// não agrega (members lista) é responsabilidade do *caller*; aqui só
// omitimos se vazio.
func personProfileView(pp org.PersonProfile) map[string]any {
	p := pp.Person
	m := p.Meta()
	v := map[string]any{
		"urn":         p.URN(),
		"kind":        p.Kind(),
		"tenant":      p.Tenant,
		"name":        p.Name,
		"email":       p.EmailHint,
		"role":        p.Role,
		"squad_urn":   p.SquadURN,
		"valid_from":  m.ValidFrom,
		"observed_at": m.ObservedAt,
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
		v["active"] = false
	} else {
		v["active"] = true
	}
	if pp.TeamURN != "" {
		v["team_urn"] = pp.TeamURN
	}
	if p.GithubHandle != "" {
		v["github_handle"] = p.GithubHandle
	}
	if p.ManagerHash != "" {
		v["manager_hash"] = p.ManagerHash
	}
	return v
}

func reportsTreeView(tr org.ReportsTree) map[string]any {
	// Para o root, o profile vai sem team_urn (controller não derivou
	// — endpoint /reports é "subordinados de X", contexto é claro).
	rootProfile := personProfileView(org.PersonProfile{Person: tr.Root})
	return map[string]any{
		"root":      rootProfile,
		"reports":   reportNodesView(tr.Reports),
		"depth":     tr.Depth,
		"truncated": tr.Truncated,
	}
}

func reportNodesView(rs []org.ReportNode) []map[string]any {
	out := make([]map[string]any, 0, len(rs))
	for _, r := range rs {
		out = append(out, map[string]any{
			"person":  personProfileView(org.PersonProfile{Person: r.Person}),
			"reports": reportNodesView(r.Reports),
		})
	}
	return out
}
