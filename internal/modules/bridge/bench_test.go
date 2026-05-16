package bridge_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// BenchmarkResolveURNBatch_10kIn100k mede o tempo de resolver 10 mil
// externalIDs num grafo de 100 mil nós. F-003/S-004 exige ≤ 5s no n4j
// real com index. Aqui rodamos na impl memory (oráculo) — alvo prático
// para spotting de regressões.
func BenchmarkResolveURNBatch_10kIn100k(b *testing.B) {
	const total = 100_000
	const lookup = 10_000

	ctx := context.Background()
	repo := memory.New()
	br := bridge.New(repo)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed: 100k Compute nodes.
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		eid := fmt.Sprintf("i-%07d", i)
		ids = append(ids, eid)
		c := mkCompute("urn:ce:aws:111:compute/"+eid, "111", eid, now)
		if err := repo.Upsert(ctx, c); err != nil {
			b.Fatalf("upsert %d: %v", i, err)
		}
	}

	// Sample uniforme de 10k IDs para resolver.
	sample := make([]string, lookup)
	step := total / lookup
	for i := 0; i < lookup; i++ {
		sample[i] = ids[i*step]
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		got, failures, err := br.ResolveURNBatch(ctx, node.ProviderAWS, "111", sample, repository.AsOf{})
		if err != nil {
			b.Fatalf("batch: %v", err)
		}
		if len(failures) != 0 {
			b.Fatalf("unexpected failures: %d", len(failures))
		}
		if len(got) != lookup {
			b.Fatalf("got %d resolved, want %d", len(got), lookup)
		}
	}
}

// TestResolveURNBatch_PerfBudget é a versão *teste* do benchmark — corre
// em `go test` sem flag e falha se o budget de 5s for estourado.
// Em CI cobre o critério de aceite sem precisar invocar `go test -bench`.
func TestResolveURNBatch_PerfBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf budget in -short mode")
	}
	const total = 100_000
	const lookup = 10_000
	const budget = 5 * time.Second

	ctx := context.Background()
	repo := memory.New()
	br := bridge.New(repo)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		eid := fmt.Sprintf("i-%07d", i)
		ids = append(ids, eid)
		c := mkCompute("urn:ce:aws:111:compute/"+eid, "111", eid, now)
		if err := repo.Upsert(ctx, c); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	sample := make([]string, lookup)
	step := total / lookup
	for i := 0; i < lookup; i++ {
		sample[i] = ids[i*step]
	}

	start := time.Now()
	got, failures, err := br.ResolveURNBatch(ctx, node.ProviderAWS, "111", sample, repository.AsOf{})
	dur := time.Since(start)

	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(failures) != 0 || len(got) != lookup {
		t.Fatalf("resolved=%d failures=%d", len(got), len(failures))
	}
	if dur > budget {
		t.Fatalf("perf budget: %s > %s", dur, budget)
	}
	t.Logf("resolved %d lookups over %d nodes in %s (budget %s)", lookup, total, dur, budget)
}
