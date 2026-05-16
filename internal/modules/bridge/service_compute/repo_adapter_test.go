package service_compute_test

import (
	"context"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge/service_compute"
	"costEngine/internal/repository/memory"
)

func newMemBackend(t *testing.T) (*service_compute.RepoAdapter, *memory.Repo) {
	t.Helper()
	m := memory.New()
	src := node.Source{Collector: "test", RunID: "r1", Method: node.MethodAPI}
	return service_compute.NewRepoAdapter(m, m.AsEdgeRepo(), src), m
}

func baseMeta() node.Meta {
	t0 := time.Unix(1, 0)
	return node.Meta{ValidFrom: t0, ObservedAt: t0}
}

func mkCompute(t *testing.T, account, id string, tags map[string]string) node.Compute {
	t.Helper()
	return node.Compute{
		Base: node.Base{
			NodeURN:  node.URN("urn:ce:aws:" + account + ":compute/" + id),
			NodeKind: node.KindCompute,
			NodeMeta: baseMeta(),
		},
		ProviderID: node.ProviderAWS,
		Account:    node.URN("urn:ce:aws:" + account + ":account/" + account),
		Region:     "urn:ce:aws::region/us-east-1",
		ExtID:      id,
		Flavor:     node.ComputeVM,
		Tags:       tags,
	}
}

func mkService(repo, modPath string) node.Service {
	urn := node.NewServiceURN(repo, modPath)
	return node.Service{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindService,
			NodeMeta: baseMeta(),
		},
		Repo:       repo,
		ModulePath: modPath,
		Language:   "go",
	}
}

func TestAdapter_RoundTrip_TagOpensThenIdempotent(t *testing.T) {
	ctx := context.Background()
	a, m := newMemBackend(t)
	account := node.URN("urn:ce:aws:111:account/111")

	if err := m.Upsert(ctx, mkService("payments-repo", ".")); err != nil {
		t.Fatal(err)
	}
	c := mkCompute(t, "111", "i-1", map[string]string{"Service": "payments-repo"})
	if err := m.Upsert(ctx, c); err != nil {
		t.Fatal(err)
	}

	// Round 1: open.
	byName, ambig, err := a.ListServicesByName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byName["payments-repo"]; !ok {
		t.Fatalf("byName missing payments-repo: %+v", byName)
	}
	if len(ambig) != 0 {
		t.Errorf("unexpected ambig: %+v", ambig)
	}
	comps, err := a.ListComputeCurrent(ctx, account)
	if err != nil || len(comps) != 1 {
		t.Fatalf("compute list err=%v len=%d", err, len(comps))
	}
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	out := service_compute.Apply(service_compute.Input{
		Account: account, Computes: comps, ServicesByName: byName, AmbiguousNames: ambig, Now: now,
	})
	if len(out.Opens) != 1 {
		t.Fatalf("opens=%d want 1", len(out.Opens))
	}
	if err := a.ApplyEdgeChanges(ctx, out.Opens, out.Closes, false); err != nil {
		t.Fatal(err)
	}

	// Round 2: idempotente — existing edge corrente já cobre.
	existing, err := a.ListRunsOnCurrent(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if len(existing) != 1 {
		t.Fatalf("existing=%d want 1", len(existing))
	}
	if existing[0].Source != service_compute.SourceTag {
		t.Errorf("existing.source=%q want tag", existing[0].Source)
	}
	out2 := service_compute.Apply(service_compute.Input{
		Account: account, Computes: comps, ServicesByName: byName, AmbiguousNames: ambig,
		ExistingEdges: existing, Now: now.Add(time.Hour),
	})
	if len(out2.Opens) != 0 || len(out2.Closes) != 0 {
		t.Errorf("second pass not idempotent: %+v", out2)
	}
}

func TestAdapter_TagChange_ClosesAndOpens(t *testing.T) {
	ctx := context.Background()
	a, m := newMemBackend(t)
	account := node.URN("urn:ce:aws:111:account/111")

	for _, n := range []node.Node{
		mkService("payments-repo", "."),
		mkService("billing-repo", "."),
	} {
		if err := m.Upsert(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	c := mkCompute(t, "111", "i-1", map[string]string{"Service": "payments-repo"})
	if err := m.Upsert(ctx, c); err != nil {
		t.Fatal(err)
	}

	// Run 1 → cria edge para payments.
	runOnce := func(now time.Time) service_compute.Output {
		byName, ambig, _ := a.ListServicesByName(ctx)
		comps, _ := a.ListComputeCurrent(ctx, account)
		ex, _ := a.ListRunsOnCurrent(ctx, account)
		out := service_compute.Apply(service_compute.Input{
			Account: account, Computes: comps, ServicesByName: byName,
			AmbiguousNames: ambig, ExistingEdges: ex, Now: now,
		})
		if err := a.ApplyEdgeChanges(ctx, out.Opens, out.Closes, false); err != nil {
			t.Fatal(err)
		}
		return out
	}
	t0 := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	runOnce(t0)

	// Muda dono via re-upsert (memory.Upsert fecha versão corrente e
	// cria nova com novo Tags).
	c2 := mkCompute(t, "111", "i-1", map[string]string{"Service": "billing-repo"})
	if err := m.Upsert(ctx, c2); err != nil {
		t.Fatal(err)
	}

	// Run 2 → fecha o antigo, abre o novo.
	t1 := t0.Add(24 * time.Hour)
	out2 := runOnce(t1)
	if len(out2.Closes) != 1 || out2.Closes[0].Reason != "owner_changed" {
		t.Errorf("closes wrong: %+v", out2.Closes)
	}
	if len(out2.Opens) != 1 {
		t.Errorf("opens=%d want 1", len(out2.Opens))
	}

	// Estado final: 1 edge corrente para billing-repo.
	existing, _ := a.ListRunsOnCurrent(ctx, account)
	if len(existing) != 1 {
		t.Fatalf("existing=%d want 1", len(existing))
	}
	if existing[0].ServiceURN != node.NewServiceURN("billing-repo", ".") {
		t.Errorf("service=%q want billing", existing[0].ServiceURN)
	}
}

func TestAdapter_AmbiguousServiceName(t *testing.T) {
	ctx := context.Background()
	a, m := newMemBackend(t)

	// Dois Services com mesmo short name "payments" em repos diferentes
	// (ex.: monorepo subdir payments + repo separado payments).
	if err := m.Upsert(ctx, mkService("monorepo", "services/payments")); err != nil {
		t.Fatal(err)
	}
	if err := m.Upsert(ctx, mkService("payments", ".")); err != nil {
		t.Fatal(err)
	}

	byName, ambig, err := a.ListServicesByName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, in := byName["payments"]; in {
		t.Errorf("ambíguo NÃO deveria estar em byName: %+v", byName)
	}
	if cands, ok := ambig["payments"]; !ok || len(cands) != 2 {
		t.Errorf("ambig: %+v", ambig)
	}
}

func TestAdapter_DryRunValidatesAdjacency(t *testing.T) {
	ctx := context.Background()
	a, _ := newMemBackend(t)
	// dry-run com open válido: deve passar.
	opens := []service_compute.EdgeOpen{{
		ServiceURN: "urn:ce:code:repo:service/payments",
		ComputeURN: "urn:ce:aws:111:compute/i-1",
		ValidFrom:  time.Now(),
		Source:     service_compute.SourceTag,
		Confidence: service_compute.ConfidenceTag,
	}}
	if err := a.ApplyEdgeChanges(ctx, opens, nil, true); err != nil {
		t.Errorf("dry-run valid: %v", err)
	}
}

// Sanity: registry rejeita uma adjacency errada (Compute→Service).
func TestRegistry_RejectsBadAdjacency(t *testing.T) {
	e := edge.ServiceRunsOn{Base: edge.Base{EdgeType: edge.TypeServiceRunsOn}}
	if err := edge.Validate(e, node.KindCompute, node.KindService); err == nil {
		t.Errorf("expected error for Compute→Service RUNS_ON")
	}
	if err := edge.Validate(e, node.KindService, node.KindCompute); err != nil {
		t.Errorf("expected ok for Service→Compute: %v", err)
	}
}
