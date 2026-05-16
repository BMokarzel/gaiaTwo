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
	if st.Opened != 2 || st.Closed != 0 || st.Unchanged != 0 {
		t.Fatalf("stats=%+v", st)
	}

	// Re-aplicar com mesmo conteúdo: tudo unchanged.
	st2, err := w.Apply(ctx, res)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if st2.Opened != 0 || st2.Closed != 0 || st2.Unchanged != 2 {
		t.Fatalf("stats2=%+v", st2)
	}
}

func TestWriter_CloseRemovedOwner(t *testing.T) {
	r, er, svc, team, alice := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 1, 0, 0, 0, time.UTC)

	w := &Writer{Nodes: r, Edges: er}

	// Passada 1: alice + team.
	res1 := Build(svc, []ResolvedOwner{
		{Handle: "@alice", URN: alice, Kind: node.KindPerson},
		{Handle: "@org/payments", URN: team, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", RunID: "r1", ObservedAt: now})
	if _, err := w.Apply(ctx, res1); err != nil {
		t.Fatalf("apply1: %v", err)
	}

	// Passada 2: só team (alice removida do CODEOWNERS).
	later := now.Add(time.Hour)
	res2 := Build(svc, []ResolvedOwner{
		{Handle: "@org/payments", URN: team, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", RunID: "r2", ObservedAt: later})
	st, err := w.Apply(ctx, res2)
	if err != nil {
		t.Fatalf("apply2: %v", err)
	}
	if st.Closed != 1 || st.Unchanged != 1 || st.Opened != 0 {
		t.Fatalf("stats=%+v", st)
	}

	// Confere: vizinhos current devem ter só 1 edge (team).
	cur, _ := er.Neighbors(ctx, svc, repository.DirIn,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeOwns}})
	if len(cur) != 1 || cur[0].From() != team {
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
	// Critério F-011: Owner duplicado (Team + Person) cria os dois edges.
	r, er, svc, team, alice := fixture(t)
	ctx := context.Background()
	w := &Writer{Nodes: r, Edges: er}
	res := Build(svc, []ResolvedOwner{
		{Handle: "@alice", URN: alice, Kind: node.KindPerson},
		{Handle: "@org/payments", URN: team, Kind: node.KindTeam},
	}, nil, EmitOptions{Tenant: "acme", ObservedAt: time.Now()})
	st, err := w.Apply(ctx, res)
	if err != nil || st.Opened != 2 {
		t.Fatalf("stats=%+v err=%v", st, err)
	}
}
