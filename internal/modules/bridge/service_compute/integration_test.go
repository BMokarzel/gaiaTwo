package service_compute_test

import (
	"context"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge/service_compute"
	"costEngine/internal/repository/memory"
)

// TestIntegration_FullCycle exercita o invariante central de F-009:
// idempotência (mesma run 2x ⇒ 0 mudanças) + drift detection (mudou
// dono ⇒ fecha+abre) + orphan tracking (sem sinal ⇒ não emite edge),
// tudo end-to-end pelo adapter de memory (sem mock fino).
func TestIntegration_FullCycle(t *testing.T) {
	ctx := context.Background()
	m := memory.New()
	src := node.Source{Collector: "it", RunID: "r1", Method: node.MethodAPI}
	adapter := service_compute.NewRepoAdapter(m, m.AsEdgeRepo(), src)
	account := node.URN("urn:ce:aws:111:account/111")

	// Semente: 3 Services + 4 Computes (3 resolvíveis + 1 órfão).
	if err := m.Upsert(ctx, mkService("payments-repo", ".")); err != nil {
		t.Fatal(err)
	}
	if err := m.Upsert(ctx, mkService("billing-repo", ".")); err != nil {
		t.Fatal(err)
	}
	if err := m.Upsert(ctx, mkService("orders-repo", ".")); err != nil {
		t.Fatal(err)
	}

	mustUp := func(c node.Compute) {
		t.Helper()
		if err := m.Upsert(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	mustUp(mkCompute(t, "111", "i-a", map[string]string{"Service": "payments-repo"}))
	mustUp(mkCompute(t, "111", "i-b", map[string]string{"Name": "svc-billing-repo-prod-01"}))
	mustUp(mkCompute(t, "111", "i-c", map[string]string{"service.urn": "urn:ce:code:orders-repo:service/."}))
	mustUp(mkCompute(t, "111", "i-d", map[string]string{"role": "ad-hoc"})) // órfão

	// Run 1: deve abrir 3 edges, deixar 1 órfão.
	runFn := func(now time.Time) service_compute.Output {
		byName, ambig, _ := adapter.ListServicesByName(ctx)
		comps, _ := adapter.ListComputeCurrent(ctx, account)
		ex, _ := adapter.ListRunsOnCurrent(ctx, account)
		out := service_compute.Apply(service_compute.Input{
			Account: account, Computes: comps, ServicesByName: byName,
			AmbiguousNames: ambig, ExistingEdges: ex, Now: now,
		})
		if err := adapter.ApplyEdgeChanges(ctx, out.Opens, out.Closes, false); err != nil {
			t.Fatal(err)
		}
		return out
	}

	t0 := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	r1 := runFn(t0)
	if r1.Stats.Opens != 3 {
		t.Errorf("r1.opens=%d want 3", r1.Stats.Opens)
	}
	if r1.Stats.NoSignal != 1 {
		t.Errorf("r1.orphans=%d want 1", r1.Stats.NoSignal)
	}
	if r1.Stats.Closes != 0 {
		t.Errorf("r1.closes=%d want 0", r1.Stats.Closes)
	}
	// ResolvedTag deve contar i-a (Service=...) e i-c (service.urn=...).
	if r1.Stats.ResolvedTag != 2 {
		t.Errorf("r1.resolved_tag=%d want 2", r1.Stats.ResolvedTag)
	}
	if r1.Stats.ResolvedName != 1 {
		t.Errorf("r1.resolved_name=%d want 1", r1.Stats.ResolvedName)
	}

	// Run 2 (idempotência): mesmo estado, sem mudanças.
	r2 := runFn(t0.Add(time.Hour))
	if r2.Stats.Opens != 0 || r2.Stats.Closes != 0 {
		t.Errorf("r2 not idempotent: opens=%d closes=%d", r2.Stats.Opens, r2.Stats.Closes)
	}

	// Drift: i-a perde sinal (untag). i-b muda Service via name convention
	// para orders. i-c continua igual.
	mustUp(mkCompute(t, "111", "i-a", map[string]string{"env": "prod"})) // perdeu Service tag
	mustUp(mkCompute(t, "111", "i-b", map[string]string{"Name": "svc-orders-repo-prod-02"}))

	// Run 3: deve fechar i-a (orphaned) e re-rotear i-b (owner_changed).
	t1 := t0.Add(24 * time.Hour)
	r3 := runFn(t1)
	if r3.Stats.Closes < 2 {
		t.Errorf("r3.closes=%d want >=2 (orphan + reroute)", r3.Stats.Closes)
	}
	if r3.Stats.Opens != 1 {
		t.Errorf("r3.opens=%d want 1 (reroute)", r3.Stats.Opens)
	}

	// Estado final via adapter: 2 edges correntes (i-b→orders, i-c→orders).
	existing, _ := adapter.ListRunsOnCurrent(ctx, account)
	if len(existing) != 2 {
		t.Errorf("final existing=%d want 2", len(existing))
	}
	for _, e := range existing {
		if e.ServiceURN != node.NewServiceURN("orders-repo", ".") {
			// i-b passou para orders; i-c já estava em orders.
			t.Errorf("unexpected edge: %+v", e)
		}
	}

	// Run 4 (idempotência pós-drift): nada muda.
	r4 := runFn(t1.Add(time.Hour))
	if r4.Stats.Opens != 0 || r4.Stats.Closes != 0 {
		t.Errorf("r4 not idempotent: opens=%d closes=%d", r4.Stats.Opens, r4.Stats.Closes)
	}
}

// TestIntegration_AmbiguousServiceProtectsExisting — quando um Service
// passa a ser ambíguo (segundo Service com mesmo nome aparece), o edge
// corrente NÃO é fechado e nenhum novo é aberto. Operador resolve via
// service.urn antes de qualquer mutação automática.
func TestIntegration_AmbiguousServiceProtectsExisting(t *testing.T) {
	ctx := context.Background()
	m := memory.New()
	src := node.Source{Collector: "it", RunID: "r", Method: node.MethodAPI}
	adapter := service_compute.NewRepoAdapter(m, m.AsEdgeRepo(), src)
	account := node.URN("urn:ce:aws:111:account/111")

	if err := m.Upsert(ctx, mkService("payments", ".")); err != nil {
		t.Fatal(err)
	}
	if err := m.Upsert(ctx, mkCompute(t, "111", "i-1", map[string]string{"Service": "payments"})); err != nil {
		t.Fatal(err)
	}

	// Run 1: cria edge.
	runOnce := func(now time.Time) service_compute.Output {
		byName, ambig, _ := adapter.ListServicesByName(ctx)
		comps, _ := adapter.ListComputeCurrent(ctx, account)
		ex, _ := adapter.ListRunsOnCurrent(ctx, account)
		out := service_compute.Apply(service_compute.Input{
			Account: account, Computes: comps, ServicesByName: byName,
			AmbiguousNames: ambig, ExistingEdges: ex, Now: now,
		})
		if err := adapter.ApplyEdgeChanges(ctx, out.Opens, out.Closes, false); err != nil {
			t.Fatal(err)
		}
		return out
	}
	runOnce(time.Unix(1000, 0))

	// Agora apareceu um Service homônimo em outro repo.
	if err := m.Upsert(ctx, mkService("monorepo", "services/payments")); err != nil {
		t.Fatal(err)
	}

	// Run 2: nome ambíguo. NÃO mexer no estado, registrar.
	r2 := runOnce(time.Unix(2000, 0))
	if r2.Stats.AmbiguousCount != 1 {
		t.Errorf("ambiguous=%d want 1", r2.Stats.AmbiguousCount)
	}
	if r2.Stats.Closes != 0 || r2.Stats.Opens != 0 {
		t.Errorf("ambiguous should not mutate state: closes=%d opens=%d", r2.Stats.Closes, r2.Stats.Opens)
	}

	// Edge corrente continua intacto (segurança: preserva info do dono real).
	existing, _ := adapter.ListRunsOnCurrent(ctx, account)
	if len(existing) != 1 {
		t.Fatalf("existing=%d want 1", len(existing))
	}
}
