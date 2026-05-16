package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org/ingest/hris"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

func runApply(t *testing.T, body, tenant string, ts time.Time) (*memory.Repo, hris.Result, Stats) {
	t.Helper()
	rows, errs, err := hris.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("row errs: %v", errs)
	}
	res, err := hris.Build(rows, hris.EmitOptions{Tenant: tenant, ObservedAt: ts})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	repo := memory.New()
	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	st, err := w.Apply(context.Background(), res)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return repo, res, st
}

func TestApply_FullGraph(t *testing.T) {
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,bob@x.com,2024-01-15,
bob@x.com,Bob,Manager,Platform,Checkout,,2020-03-01,
carol@x.com,Carol,Eng,Product,Growth,bob@x.com,2023-06-01,
`
	repo, res, st := runApply(t, body, "acme", time.Unix(1700000000, 0).UTC())
	if st.Teams != 2 || st.Squads != 2 || st.Persons != 3 {
		t.Errorf("stats=%+v", st)
	}
	if st.Edges != 3+2+2 { // 3 MemberOf + 2 PartOf + 2 ReportsTo
		t.Errorf("edges=%d want 7", st.Edges)
	}

	// Spot check: bob's squad MEMBER_OF → squad
	bobURN := node.NewPersonURN("acme", node.HashEmail("bob@x.com"))
	out, err := repo.AsEdgeRepo().Neighbors(context.Background(), bobURN, repository.DirOut,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeMemberOf}})
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("bob MEMBER_OF=%d", len(out))
	}
	_ = res
}

func TestApply_ClosesTerminated(t *testing.T) {
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,,2024-01-15,2026-04-30
`
	repo, _, st := runApply(t, body, "acme", time.Unix(1700000000, 0).UTC())
	if st.Terminated != 1 {
		t.Errorf("Terminated=%d want 1", st.Terminated)
	}
	urn := node.NewPersonURN("acme", node.HashEmail("alice@x.com"))
	if _, err := repo.GetByURN(context.Background(), urn, repository.AsOf{}); err == nil {
		t.Error("expected ErrNotFound on terminated Person current version")
	}

	// Edge MEMBER_OF saindo dela também não deve aparecer como corrente.
	edges, err := repo.AsEdgeRepo().Neighbors(context.Background(), urn, repository.DirOut,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeMemberOf}})
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("terminated person still has MEMBER_OF edge: %+v", edges)
	}
}
