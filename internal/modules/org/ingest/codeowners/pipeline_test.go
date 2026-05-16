package codeowners

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// runPipeline encadeia Parse → Resolver.Resolve → ServiceLookup →
// Build → Writer.Apply, simulando exatamente o que o CLI faz. Devolve
// stats + emit para asserts.
func runPipeline(
	t *testing.T,
	r *memory.Repo,
	tenant, repoPath, body string,
	observedAt time.Time,
) (Stats, Result) {
	t.Helper()
	ctx := context.Background()

	rules, rowErrs, err := Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rowErrs) > 0 {
		t.Fatalf("unexpected row errors: %v", rowErrs)
	}
	global, ok := GlobalRule(rules)
	if !ok {
		t.Fatalf("no global rule")
	}

	res := &Resolver{Nodes: r, Tenant: tenant}
	resolved, unresolved, err := res.Resolve(ctx, global.Owners)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	lookup := &ServiceLookup{Nodes: r}
	resolution, err := lookup.ResolveForRepo(ctx, repoPath)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	emit := Build(resolution.Found, resolved, unresolved,
		EmitOptions{Tenant: tenant, RunID: "r-" + observedAt.Format("150405"), ObservedAt: observedAt})

	w := &Writer{Nodes: r, Edges: r.AsEdgeRepo()}
	st, err := w.Apply(ctx, emit)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return st, emit
}

// seedFullGraph monta Service `payments` raiz + Team `payments` +
// Person `alice` com github_handle. Devolve as URNs.
func seedFullGraph(t *testing.T, r *memory.Repo, now time.Time) (svc, team, alice node.URN) {
	t.Helper()
	ctx := context.Background()

	s := mkService("payments", ".", now)
	if err := r.Upsert(ctx, s); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	tm := node.Team{
		Base: node.Base{
			NodeURN:  node.NewTeamURN("acme", "payments"),
			NodeKind: node.KindTeam,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "payments", Name: "Payments",
	}
	if err := r.Upsert(ctx, tm); err != nil {
		t.Fatalf("upsert team: %v", err)
	}
	a := node.Person{
		Base: node.Base{
			NodeURN:  node.NewPersonURN("acme", node.HashEmail("alice@a.com")),
			NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("alice@a.com"),
		Name: "Alice", GithubHandle: "alice",
	}
	if err := r.Upsert(ctx, a); err != nil {
		t.Fatalf("upsert alice: %v", err)
	}
	return s.URN(), tm.URN(), a.URN()
}

// TestPipeline_TwoPassCloseReopen exercita o cenário full-cycle do
// F-011: passada 1 abre 2 edges; passada 2 remove alice → 1 close + 1
// unchanged; passada 3 (idempotente) → 0/0/1.
func TestPipeline_TwoPassCloseReopen(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_, team, alice := seedFullGraph(t, r, now)

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "payments")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Passada 1: alice + team — 2 opened.
	st1, _ := runPipeline(t, r, "acme", repoPath,
		"* @alice @org/payments\n", now.Add(time.Hour))
	if st1.Opened != 2 || st1.Closed != 0 || st1.Unchanged != 0 {
		t.Fatalf("pass1=%+v", st1)
	}

	// Passada 2: só team — alice deve ser fechada.
	st2, _ := runPipeline(t, r, "acme", repoPath,
		"* @org/payments\n", now.Add(2*time.Hour))
	if st2.Opened != 0 || st2.Closed != 1 || st2.Unchanged != 1 {
		t.Fatalf("pass2=%+v", st2)
	}

	// Passada 3: mesmo conteúdo da 2 → idempotente.
	st3, _ := runPipeline(t, r, "acme", repoPath,
		"* @org/payments\n", now.Add(3*time.Hour))
	if st3.Opened != 0 || st3.Closed != 0 || st3.Unchanged != 1 {
		t.Fatalf("pass3=%+v", st3)
	}

	// Confere o grafo: deve haver exatamente 1 edge corrente Owns,
	// vindo do team.
	ctx := context.Background()
	cur, _ := r.AsEdgeRepo().Neighbors(ctx, mustURN(t, r, "payments"), repository.DirIn,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeOwns}})
	if len(cur) != 1 || cur[0].From() != team {
		t.Fatalf("current edges=%v want only team=%s", cur, team)
	}
	_ = alice
}

// TestPipeline_ReopenAfterClose: passada 1 abre alice; passada 2 fecha;
// passada 3 reabre. Confere semântica close-and-reopen.
func TestPipeline_ReopenAfterClose(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_, _, _ = seedFullGraph(t, r, now)

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "payments")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	st1, _ := runPipeline(t, r, "acme", repoPath,
		"* @alice\n", now.Add(time.Hour))
	if st1.Opened != 1 {
		t.Fatalf("pass1=%+v", st1)
	}
	st2, _ := runPipeline(t, r, "acme", repoPath,
		"* @org/payments\n", now.Add(2*time.Hour))
	if st2.Opened != 1 || st2.Closed != 1 {
		t.Fatalf("pass2=%+v", st2)
	}
	st3, _ := runPipeline(t, r, "acme", repoPath,
		"* @alice @org/payments\n", now.Add(3*time.Hour))
	if st3.Opened != 1 || st3.Unchanged != 1 {
		t.Fatalf("pass3=%+v (queria 1 open p/ alice de volta, 1 unchanged p/ team)", st3)
	}
}

// TestPipeline_DuplicateOwnerBothEdges: alice (Person) + alice (Team)
// — se ambos existem no grafo, ambos viram edges separados.
func TestPipeline_DuplicateOwnerBothEdges(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_, _, _ = seedFullGraph(t, r, now)

	// Adiciona um Team também chamado "alice" (sim: handle Person ==
	// slug Team é colisão real em CODEOWNERS, ambos devem virar
	// edges distintos).
	ctx := context.Background()
	tm := node.Team{
		Base: node.Base{
			NodeURN:  node.NewTeamURN("acme", "alice"),
			NodeKind: node.KindTeam,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "alice", Name: "Alice-Team",
	}
	if err := r.Upsert(ctx, tm); err != nil {
		t.Fatalf("upsert team alice: %v", err)
	}

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "payments")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	st, _ := runPipeline(t, r, "acme", repoPath,
		"* @alice @acme/alice\n", now.Add(time.Hour))
	if st.Opened != 2 {
		t.Fatalf("queria 2 opened (Person+Team), got=%+v", st)
	}
}

// TestPipeline_UnresolvedNonFatal: handle inexistente vira `unresolved`
// e o pipeline segue (não aborta).
func TestPipeline_UnresolvedNonFatal(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_, _, _ = seedFullGraph(t, r, now)

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "payments")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	st, emit := runPipeline(t, r, "acme", repoPath,
		"* @alice @ghost-user\n", now.Add(time.Hour))
	if st.Opened != 1 {
		t.Fatalf("queria 1 opened (só alice), got=%+v", st)
	}
	if st.Unresolved != 1 {
		t.Fatalf("queria 1 unresolved, got=%+v", st)
	}
	if len(emit.Unresolved) != 1 || emit.Unresolved[0] != "@ghost-user" {
		t.Fatalf("unresolved=%v", emit.Unresolved)
	}
}

func mustURN(t *testing.T, r *memory.Repo, repoName string) node.URN {
	t.Helper()
	urn := node.NewServiceURN(repoName, ".")
	if _, err := r.GetByURN(context.Background(), urn, repository.AsOf{}); err != nil {
		t.Fatalf("service %s missing: %v", urn, err)
	}
	return urn
}
