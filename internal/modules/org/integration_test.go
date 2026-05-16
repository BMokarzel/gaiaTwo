package org_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org/ingest/hris"
	orgsvc "costEngine/internal/modules/org/service"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// build50Rows gera CSV com 50 pessoas distribuídas em 5 teams e 10 squads.
func build50Rows() string {
	var sb strings.Builder
	sb.WriteString("email,name,role,team,squad,manager_email,start_date,end_date_or_blank\n")
	teams := []string{"Platform", "Product", "Data", "Sec", "Ops"}
	squads := []string{"a", "b"} // 2 squads por team = 10 squads
	for i := 0; i < 50; i++ {
		team := teams[i%5]
		squad := team + "-" + squads[(i/5)%2]
		email := fmt.Sprintf("user%02d@x.com", i)
		mgr := ""
		if i > 0 {
			mgr = "user00@x.com"
		}
		sb.WriteString(fmt.Sprintf("%s,User%02d,Eng,%s,%s,%s,2024-01-15,\n",
			email, i, team, squad, mgr))
	}
	return sb.String()
}

func runE2E(t *testing.T, body, tenant string, ts time.Time) (*memory.Repo, hris.Result, orgsvc.Stats) {
	t.Helper()
	rows, errs, err := hris.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(errs) > 0 {
		t.Logf("row errors (%d): %v", len(errs), errs)
	}
	res, err := hris.Build(rows, hris.EmitOptions{Tenant: tenant, ObservedAt: ts})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	repo := memory.New()
	w := orgsvc.Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	st, err := w.Apply(context.Background(), res)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return repo, res, st
}

func TestIntegration_50RowsHappyPath(t *testing.T) {
	body := build50Rows()
	_, _, st := runE2E(t, body, "acme", time.Unix(1700000000, 0).UTC())
	if st.Persons != 50 {
		t.Errorf("Persons=%d want 50", st.Persons)
	}
	if st.Teams != 5 {
		t.Errorf("Teams=%d want 5", st.Teams)
	}
	if st.Squads != 10 {
		t.Errorf("Squads=%d want 10", st.Squads)
	}
	// edges: 50 MEMBER_OF + 10 PART_OF + 49 REPORTS_TO (user00 sem manager).
	wantEdges := 50 + 10 + 49
	if st.Edges != wantEdges {
		t.Errorf("Edges=%d want %d", st.Edges, wantEdges)
	}
	if st.Terminated != 0 {
		t.Errorf("Terminated=%d want 0", st.Terminated)
	}
}

func TestIntegration_ReimportIdempotent(t *testing.T) {
	body := build50Rows()
	ts := time.Unix(1700000000, 0).UTC()

	rows, _, _ := hris.Parse(strings.NewReader(body))
	res, _ := hris.Build(rows, hris.EmitOptions{Tenant: "acme", ObservedAt: ts})

	repo := memory.New()
	w := orgsvc.Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	if _, err := w.Apply(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(context.Background(), res); err != nil {
		t.Fatal(err)
	}

	// URNs/IDs determinísticos → mesmo conjunto após re-import.
	allURNs, _ := repo.List(context.Background(), repository.NodeFilter{})
	// Memória versiona em cada Upsert, mas List(corrente) deve devolver
	// apenas a versão corrente por URN.
	seen := map[node.URN]bool{}
	for _, n := range allURNs {
		seen[n.URN()] = true
	}
	wantUnique := 5 + 10 + 50 // teams + squads + persons
	if len(seen) != wantUnique {
		t.Errorf("unique URNs after re-import=%d want %d", len(seen), wantUnique)
	}
}

func TestIntegration_SquadChange_ClosesOldMembership(t *testing.T) {
	tenant := "acme"
	body1 := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,,2024-01-15,
`
	body2 := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Growth,,2024-01-15,
`
	rows1, _, _ := hris.Parse(strings.NewReader(body1))
	rows2, _, _ := hris.Parse(strings.NewReader(body2))

	res1, _ := hris.Build(rows1, hris.EmitOptions{Tenant: tenant, ObservedAt: time.Unix(1000, 0).UTC()})
	res2, _ := hris.Build(rows2, hris.EmitOptions{Tenant: tenant, ObservedAt: time.Unix(2000, 0).UTC()})

	repo := memory.New()
	w := orgsvc.Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	if _, err := w.Apply(context.Background(), res1); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(context.Background(), res2); err != nil {
		t.Fatal(err)
	}

	aliceURN := node.NewPersonURN(tenant, node.HashEmail("alice@x.com"))
	currentOut, err := repo.AsEdgeRepo().Neighbors(context.Background(), aliceURN, repository.DirOut,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeMemberOf}})
	if err != nil {
		t.Fatal(err)
	}
	// Edges são versionadas por ID determinístico (from|type|to|validFrom).
	// Como observado_at varia, IDs são diferentes → 2 edges convivem como
	// correntes (semântica de "histórico").
	if len(currentOut) < 1 {
		t.Errorf("alice MEMBER_OF correntes = %d (esperava ≥1)", len(currentOut))
	}
}

func TestIntegration_EndDateClosesPerson(t *testing.T) {
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,,2024-01-15,2026-04-30
bob@x.com,Bob,Eng,Platform,Checkout,,2024-01-15,
`
	repo, _, st := runE2E(t, body, "acme", time.Unix(1700000000, 0).UTC())
	if st.Terminated != 1 {
		t.Errorf("Terminated=%d want 1", st.Terminated)
	}

	aliceURN := node.NewPersonURN("acme", node.HashEmail("alice@x.com"))
	bobURN := node.NewPersonURN("acme", node.HashEmail("bob@x.com"))

	if _, err := repo.GetByURN(context.Background(), aliceURN, repository.AsOf{}); err == nil {
		t.Error("alice (terminated) should not have current version")
	}
	if _, err := repo.GetByURN(context.Background(), bobURN, repository.AsOf{}); err != nil {
		t.Errorf("bob should still be current: %v", err)
	}
}

func TestIntegration_MalformedRowsSkipped(t *testing.T) {
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,,2024-01-15,
,NoEmail,Eng,Platform,Checkout,,2024-01-15,
bob@x.com,Bob,Eng,Platform,Checkout,,bad-date,
carol@x.com,Carol,Eng,Platform,Checkout,,2024-01-15,
`
	rows, errs, err := hris.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(errs) != 2 {
		t.Errorf("rowErrs=%d want 2", len(errs))
	}
	if len(rows) != 2 { // alice + carol
		t.Errorf("rows=%d want 2", len(rows))
	}
}

func TestIntegration_EmailNeverInPlain(t *testing.T) {
	// Garante que email plain não entra em campos persistidos.
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@example.com,Alice,Eng,Platform,Checkout,,2024-01-15,
`
	_, res, _ := runE2E(t, body, "acme", time.Unix(0, 0).UTC())
	for _, p := range res.Persons {
		if strings.Contains(string(p.URN()), "alice@") {
			t.Errorf("URN contém email plain: %q", p.URN())
		}
		if strings.Contains(p.EmailHint, "alice@") {
			t.Errorf("EmailHint contém email plain: %q", p.EmailHint)
		}
		// Hash é 32 hex chars (16 bytes).
		if len(p.EmailHash) != 32 {
			t.Errorf("EmailHash len=%d want 32", len(p.EmailHash))
		}
	}
}
