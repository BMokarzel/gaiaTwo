package hris

import (
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func parseFixture(t *testing.T, body string) []Row {
	t.Helper()
	rows, errs, err := Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	return rows
}

func TestBuild_Happy(t *testing.T) {
	rows := parseFixture(t, `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,bob@x.com,2024-01-15,
bob@x.com,Bob,Manager,Platform,Checkout,,2020-03-01,
carol@x.com,Carol,Eng,Product,Growth,bob@x.com,2023-06-01,
`)
	res, err := Build(rows, EmitOptions{Tenant: "acme", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(res.Teams) != 2 {
		t.Errorf("Teams=%d want 2", len(res.Teams))
	}
	if len(res.Squads) != 2 {
		t.Errorf("Squads=%d want 2", len(res.Squads))
	}
	if len(res.Persons) != 3 {
		t.Errorf("Persons=%d want 3", len(res.Persons))
	}
	if len(res.MemberOf) != 3 {
		t.Errorf("MemberOf=%d want 3", len(res.MemberOf))
	}
	if len(res.PartOf) != 2 {
		t.Errorf("PartOf=%d want 2", len(res.PartOf))
	}
	// alice → bob, carol → bob = 2 ReportsTo
	if len(res.ReportsTo) != 2 {
		t.Errorf("ReportsTo=%d want 2", len(res.ReportsTo))
	}
	// nenhum terminado
	if len(res.Terminated) != 0 {
		t.Errorf("Terminated=%v", res.Terminated)
	}

	// Cada Person carrega EmailHint, nunca o email plain.
	for _, p := range res.Persons {
		if strings.Contains(p.EmailHint, "@x.com") && !strings.Contains(p.EmailHint, "***") {
			t.Errorf("EmailHint não mascarado: %q", p.EmailHint)
		}
	}
}

func TestBuild_TerminatedClosesPerson(t *testing.T) {
	rows := parseFixture(t, `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,,2024-01-15,2026-04-30
`)
	res, err := Build(rows, EmitOptions{Tenant: "acme"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(res.Terminated) != 1 {
		t.Errorf("Terminated=%v", res.Terminated)
	}
	expected := node.NewPersonURN("acme", node.HashEmail("alice@x.com"))
	if res.Terminated[0] != expected {
		t.Errorf("Terminated[0]=%q want %q", res.Terminated[0], expected)
	}
}

func TestBuild_DetectsCycle(t *testing.T) {
	rows := parseFixture(t, `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
a@x.com,A,Eng,T,S,b@x.com,2024-01-15,
b@x.com,B,Eng,T,S,c@x.com,2024-01-15,
c@x.com,C,Eng,T,S,a@x.com,2024-01-15,
`)
	_, err := Build(rows, EmitOptions{Tenant: "acme"})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("want cycle error, got %v", err)
	}
}

func TestBuild_DedupTeamsSquads(t *testing.T) {
	rows := parseFixture(t, `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
a@x.com,A,Eng,Platform,Checkout,,2024-01-15,
b@x.com,B,Eng,Platform,Checkout,,2024-01-15,
c@x.com,C,Eng,Platform,Checkout,,2024-01-15,
`)
	res, _ := Build(rows, EmitOptions{Tenant: "acme"})
	if len(res.Teams) != 1 || len(res.Squads) != 1 {
		t.Errorf("dedup falhou: teams=%d squads=%d", len(res.Teams), len(res.Squads))
	}
	if len(res.Persons) != 3 || len(res.MemberOf) != 3 {
		t.Errorf("persons=%d memberof=%d", len(res.Persons), len(res.MemberOf))
	}
}

func TestBuild_Idempotent(t *testing.T) {
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,bob@x.com,2024-01-15,
bob@x.com,Bob,Manager,Platform,Checkout,,2020-03-01,
`
	r1 := parseFixture(t, body)
	r2 := parseFixture(t, body)
	opts := EmitOptions{Tenant: "acme", ObservedAt: time.Unix(1700000000, 0).UTC()}
	a, _ := Build(r1, opts)
	b, _ := Build(r2, opts)
	au, bu := nodeURNSet(a), nodeURNSet(b)
	if au != bu {
		t.Errorf("URN set diverged")
	}
	ae, be := edgeIDSet(a), edgeIDSet(b)
	if ae != be {
		t.Errorf("Edge ID set diverged")
	}
}

func TestBuild_RequiresTenant(t *testing.T) {
	_, err := Build(nil, EmitOptions{})
	if err == nil {
		t.Error("want error when tenant missing")
	}
}

func nodeURNSet(r Result) string {
	urns := []string{}
	for _, t := range r.Teams {
		urns = append(urns, string(t.URN()))
	}
	for _, s := range r.Squads {
		urns = append(urns, string(s.URN()))
	}
	for _, p := range r.Persons {
		urns = append(urns, string(p.URN()))
	}
	return joinSorted(urns)
}

func edgeIDSet(r Result) string {
	ids := []string{}
	for _, e := range r.MemberOf {
		ids = append(ids, e.ID())
	}
	for _, e := range r.PartOf {
		ids = append(ids, e.ID())
	}
	for _, e := range r.ReportsTo {
		ids = append(ids, e.ID())
	}
	return joinSorted(ids)
}

// Compile-time check para garantir que MemberOf/PartOf/ReportsTo
// satisfazem `edge.Edge` (cobertura adicional ao registry).
var (
	_ edge.Edge = edge.MemberOf{}
	_ edge.Edge = edge.PartOf{}
	_ edge.Edge = edge.ReportsTo{}
)
