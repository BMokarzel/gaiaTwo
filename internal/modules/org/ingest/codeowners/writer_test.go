package codeowners

import (
	"context"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// fixture monta um grafo mínimo: Service raiz, Team payments, Person alice.
func fixture(t *testing.T) (*memory.Repo, repository.EdgeRepository, node.URN, node.URN, node.URN) {
	t.Helper()
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	svc := mkService("payments", ".", now)
	if err := r.Upsert(ctx, svc); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	team := node.Team{
		Base: node.Base{
			NodeURN:  node.NewTeamURN("acme", "payments"),
			NodeKind: node.KindTeam,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "payments", Name: "Payments",
	}
	if err := r.Upsert(ctx, team); err != nil {
		t.Fatalf("upsert team: %v", err)
	}
	alice := node.Person{
		Base: node.Base{
			NodeURN:  node.NewPersonURN("acme", node.HashEmail("alice@a.com")),
			NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("alice@a.com"),
		Name: "Alice", GithubHandle: "alice",
	}
	if err := r.Upsert(ctx, alice); err != nil {
		t.Fatalf("upsert alice: %v", err)
	}
	return r, r.AsEdgeRepo(), svc.URN(), team.URN(), alice.URN()
}

func TestWriter_OpenAndUnchanged(t *testing.T) {
	// ADR-010/F-029: Person→Service é rejeitado pelo lint; só o Team
	// vira OWNS. O Person fica em Rejected (auditoria), não no grafo.
	r, er, svc, team, alice := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 1, 0, 0, 0, time.UTC)

	w := &Writer{Nodes: r, Edges: er}

	res := Build(svc, []ResolvedOwner{
		{Handle: "@alice", URN: alice, Kind: node.KindPerson},
		{Handle: "@org/payments", URN: team, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", RunID: "r1", ObservedAt: now})

	st, err := w.Apply(ctx, res)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Opened != 1 || st.Closed != 0 || st.Unchanged != 0 || st.Rejected != 1 {
		t.Fatalf("stats=%+v want opened=1 rejected=1", st)
	}

	// Re-aplicar com mesmo conteúdo: 1 unchanged (team), 1 rejected (alice).
	st2, err := w.Apply(ctx, res)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if st2.Opened != 0 || st2.Closed != 0 || st2.Unchanged != 1 || st2.Rejected != 1 {
		t.Fatalf("stats2=%+v", st2)
	}
}

func TestWriter_CloseRemovedOwner(t *testing.T) {
	// Usa dois Teams (ADR-010 não admite Person→Service): pass1 abre
	// ambos, pass2 fecha team-b → 1 closed + 1 unchanged.
	r, er, svc, teamA := fixtureTwoTeams(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 1, 0, 0, 0, time.UTC)

	w := &Writer{Nodes: r, Edges: er}
	teamB := node.NewTeamURN("acme", "platform")

	// Passada 1: teamA + teamB.
	res1 := Build(svc, []ResolvedOwner{
		{Handle: "@org/payments", URN: teamA, Kind: node.KindTeam},
		{Handle: "@org/platform", URN: teamB, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", RunID: "r1", ObservedAt: now})
	if _, err := w.Apply(ctx, res1); err != nil {
		t.Fatalf("apply1: %v", err)
	}

	// Passada 2: só teamA (teamB removida do CODEOWNERS).
	later := now.Add(time.Hour)
	res2 := Build(svc, []ResolvedOwner{
		{Handle: "@org/payments", URN: teamA, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", RunID: "r2", ObservedAt: later})
	st, err := w.Apply(ctx, res2)
	if err != nil {
		t.Fatalf("apply2: %v", err)
	}
	if st.Closed != 1 || st.Unchanged != 1 || st.Opened != 0 {
		t.Fatalf("stats=%+v", st)
	}

	// Confere: vizinhos current devem ter só 1 edge (teamA).
	cur, _ := er.Neighbors(ctx, svc, repository.DirIn,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeOwns}})
	if len(cur) != 1 || cur[0].From() != teamA {
		t.Fatalf("current=%v", cur)
	}
}

func TestWriter_UnresolvedCounted(t *testing.T) {
	r, er, svc, _, _ := fixture(t)
	ctx := context.Background()
	w := &Writer{Nodes: r, Edges: er}
	res := Build(svc, nil, []string{"@ghost", "@org/missing"},
		EmitOptions{Tenant: "acme", ObservedAt: time.Now()})
	st, err := w.Apply(ctx, res)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Unresolved != 2 || st.Opened != 0 {
		t.Fatalf("stats=%+v", st)
	}
}

func TestWriter_DuplicateOwnerCreatesTwoEdges(t *testing.T) {
	// Critério F-011 + F-029 (ADR-010): dois Teams distintos criam
	// dois edges; um Person é rejeitado.
	r, er, svc, teamA := fixtureTwoTeams(t)
	ctx := context.Background()
	w := &Writer{Nodes: r, Edges: er}
	teamB := node.NewTeamURN("acme", "platform")
	res := Build(svc, []ResolvedOwner{
		{Handle: "@org/payments", URN: teamA, Kind: node.KindTeam},
		{Handle: "@org/platform", URN: teamB, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", ObservedAt: time.Now()})
	st, err := w.Apply(ctx, res)
	if err != nil || st.Opened != 2 {
		t.Fatalf("stats=%+v err=%v", st, err)
	}
}

// TestWriter_RejectsPersonOnService confere F-029: Person→Service não
// vira OWNS, vai para Rejected (lint ADR-010).
func TestWriter_RejectsPersonOnService(t *testing.T) {
	r, er, svc, _, alice := fixture(t)
	ctx := context.Background()
	w := &Writer{Nodes: r, Edges: er}
	res := Build(svc, []ResolvedOwner{
		{Handle: "@alice", URN: alice, Kind: node.KindPerson},
	}, nil, EmitOptions{Tenant: "acme", ObservedAt: time.Now()})
	if len(res.Owns) != 0 {
		t.Fatalf("Owns deveria estar vazio; got %+v", res.Owns)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].Kind != node.KindPerson {
		t.Fatalf("Rejected=%+v", res.Rejected)
	}
	st, err := w.Apply(ctx, res)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Opened != 0 || st.Rejected != 1 {
		t.Fatalf("stats=%+v", st)
	}
}

// fixtureTwoTeams é como fixture mas com dois Teams (payments+platform)
// para cobrir casos de open/close com múltiplos Team-owners.
func fixtureTwoTeams(t *testing.T) (*memory.Repo, repository.EdgeRepository, node.URN, node.URN) {
	t.Helper()
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	svc := mkService("payments", ".", now)
	if err := r.Upsert(ctx, svc); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	for _, slug := range []string{"payments", "platform"} {
		tm := node.Team{
			Base: node.Base{
				NodeURN:  node.NewTeamURN("acme", slug),
				NodeKind: node.KindTeam,
				NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
			},
			Tenant: "acme", Slug: slug, Name: slug,
		}
		if err := r.Upsert(ctx, tm); err != nil {
			t.Fatalf("upsert team %s: %v", slug, err)
		}
	}
	return r, r.AsEdgeRepo(), svc.URN(), node.NewTeamURN("acme", "payments")
}
