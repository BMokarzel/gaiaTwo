package codeowners

import (
	"context"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository/memory"
)

func TestResolver_TeamAndPerson(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

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

	res := &Resolver{Nodes: r, Tenant: "acme"}
	resolved, unresolved, err := res.Resolve(ctx, []string{"@alice", "@org/payments"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved: %v", unresolved)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved=%v", resolved)
	}
	// Ordem preservada da entrada.
	if resolved[0].Kind != node.KindPerson || resolved[0].URN != alice.URN() {
		t.Fatalf("first=%+v want person/alice", resolved[0])
	}
	if resolved[1].Kind != node.KindTeam || resolved[1].URN != team.URN() {
		t.Fatalf("second=%+v want team/payments", resolved[1])
	}
}

func TestResolver_HandleCaseInsensitive(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	bob := node.Person{
		Base: node.Base{
			NodeURN:  node.NewPersonURN("acme", node.HashEmail("bob@a.com")),
			NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("bob@a.com"),
		Name: "Bob", GithubHandle: "BoB",
	}
	_ = r.Upsert(ctx, bob)

	res := &Resolver{Nodes: r, Tenant: "acme"}
	resolved, _, err := res.Resolve(ctx, []string{"@bob"})
	if err != nil || len(resolved) != 1 || resolved[0].URN != bob.URN() {
		t.Fatalf("resolved=%v err=%v", resolved, err)
	}
}

func TestResolver_UnresolvedHandle(t *testing.T) {
	r := memory.New()
	ctx := context.Background()
	res := &Resolver{Nodes: r, Tenant: "acme"}
	resolved, unresolved, err := res.Resolve(ctx, []string{"@ghost", "@org/missing"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resolved) != 0 {
		t.Fatalf("resolved=%v want []", resolved)
	}
	if len(unresolved) != 2 {
		t.Fatalf("unresolved=%v want 2", unresolved)
	}
}

func TestResolver_DedupSameURN(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	carol := node.Person{
		Base: node.Base{
			NodeURN:  node.NewPersonURN("acme", node.HashEmail("c@a.com")),
			NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("c@a.com"),
		Name: "Carol", GithubHandle: "carol",
	}
	_ = r.Upsert(ctx, carol)

	res := &Resolver{Nodes: r, Tenant: "acme"}
	// Mesmo handle duas vezes: só deve emitir um.
	resolved, _, _ := res.Resolve(ctx, []string{"@carol", "@carol"})
	if len(resolved) != 1 {
		t.Fatalf("dedup falhou: %v", resolved)
	}
}

func TestResolver_TeamPlusPersonNotDeduplicated(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	team := node.Team{
		Base: node.Base{
			NodeURN:  node.NewTeamURN("acme", "platform"),
			NodeKind: node.KindTeam,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "platform", Name: "Platform",
	}
	_ = r.Upsert(ctx, team)
	dave := node.Person{
		Base: node.Base{
			NodeURN:  node.NewPersonURN("acme", node.HashEmail("d@a.com")),
			NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("d@a.com"),
		Name: "Dave", GithubHandle: "dave",
	}
	_ = r.Upsert(ctx, dave)

	res := &Resolver{Nodes: r, Tenant: "acme"}
	resolved, _, _ := res.Resolve(ctx, []string{"@dave", "@org/platform"})
	if len(resolved) != 2 {
		t.Fatalf("expected 2 distinct URNs, got %v", resolved)
	}
}
