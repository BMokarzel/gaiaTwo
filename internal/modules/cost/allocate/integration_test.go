//go:build integration

// Integration test do F-005 S-008: roda o pipeline fim-a-fim contra um
// ClickHouse real e valida o invariante macro:
//
//	TotalAllocated + TotalUnallocated ≈ TotalCUR  (delta ≤ 1e-6)
//
// Setup:
//
//	docker run --rm -p 9000:9000 -p 8123:8123 clickhouse/clickhouse-server
//	export CLICKHOUSE_TEST_URI=localhost:9000
//	go test -tags=integration ./internal/modules/cost/allocate/...
//
// Sem `CLICKHOUSE_TEST_URI`, o teste skipa.
//
// O resolver usado aqui é um stub in-memory que devolve URN para uma
// lista pré-definida e NotFound para o resto — não exige Neo4j up.
package allocate

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
	"costEngine/internal/repository"
	chrepo "costEngine/internal/repository/clickhouse"
)

func testCHEnv(t *testing.T) (string, string, string) {
	t.Helper()
	uri := os.Getenv("CLICKHOUSE_TEST_URI")
	if uri == "" {
		t.Skip("CLICKHOUSE_TEST_URI not set; skipping integration test")
	}
	user := os.Getenv("CLICKHOUSE_TEST_USER")
	if user == "" {
		user = "default"
	}
	pass := os.Getenv("CLICKHOUSE_TEST_PASS")
	return uri, user, pass
}

func freshCH(t *testing.T) *chrepo.Client {
	t.Helper()
	uri, user, pass := testCHEnv(t)
	bootstrap, err := chrepo.Open(context.Background(), chrepo.Config{
		Addrs: []string{uri}, Database: "default", Username: user, Password: pass,
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	defer bootstrap.Close()

	dbName := "ce_alloc_" + time.Now().UTC().Format("20060102_150405_000000")
	if err := bootstrap.Conn.Exec(context.Background(), "CREATE DATABASE IF NOT EXISTS "+dbName); err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		b, _ := chrepo.Open(context.Background(), chrepo.Config{Addrs: []string{uri}, Database: "default", Username: user, Password: pass})
		if b != nil {
			_ = b.Conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName)
			b.Close()
		}
	})

	c, err := chrepo.Open(context.Background(), chrepo.Config{
		Addrs: []string{uri}, Database: dbName, Username: user, Password: pass,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := c.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c
}

// seedCUR popula fct_cur_lines com linhas fabricadas. Retorna o total
// `effective_cost` inserido (ground truth do invariante).
func seedCUR(t *testing.T, c *chrepo.Client, period time.Time, lines []seedLine) float64 {
	t.Helper()
	ctx := context.Background()
	b, err := c.Conn.PrepareBatch(ctx, "INSERT INTO fct_cur_lines")
	if err != nil {
		t.Fatalf("prepare seed batch: %v", err)
	}
	now := time.Now().UTC()
	total := 0.0
	for i, l := range lines {
		if err := b.Append(
			"rep", "li-"+string(rune('a'+i)), period, period.Add(time.Hour), period.Add(2*time.Hour), now,
			"seed.parquet", "run-1",
			"aws", l.account, "", l.resourceID, "", l.region, "",
			l.service, "", "", "usage", "on_demand",
			1.0, "Hrs", l.cost, l.cost, l.cost, "USD",
			map[string]string{}, map[string]string{},
		); err != nil {
			t.Fatalf("append seed: %v", err)
		}
		total += l.cost
	}
	if err := b.Send(); err != nil {
		t.Fatalf("send seed: %v", err)
	}
	return total
}

type seedLine struct {
	account, resourceID, region, service string
	cost                                 float64
}

// stubResolver: matcha (account, resourceID) → URN para os IDs do mapa;
// o resto retorna ErrNotFound.
type stubResolver struct {
	hits map[[2]string]node.URN
}

func (s *stubResolver) ResolveURN(_ context.Context, _ node.ProviderID, account, externalID string, _ repository.AsOf) (node.URN, error) {
	urn, ok := s.hits[[2]string{account, externalID}]
	if !ok {
		return "", repository.ErrNotFound
	}
	return urn, nil
}

func (s *stubResolver) ResolveURNBatch(ctx context.Context, p node.ProviderID, account string, ids []string, as repository.AsOf) (map[string]node.URN, []bridge.ResolveError, error) {
	out := map[string]node.URN{}
	var failures []bridge.ResolveError
	for _, id := range ids {
		urn, err := s.ResolveURN(ctx, p, account, id, as)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				failures = append(failures, bridge.ResolveError{ExternalID: id, Err: err})
				continue
			}
			return out, failures, err
		}
		out[id] = urn
	}
	return out, failures, nil
}

// TestIntegration_AllocateInvariant valida que TotalAllocated +
// TotalUnallocated == TotalCUR num run real.
func TestIntegration_AllocateInvariant(t *testing.T) {
	c := freshCH(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	totalCUR := seedCUR(t, c, period, []seedLine{
		{account: "111", resourceID: "i-1", region: "us-east-1", service: "AmazonEC2", cost: 1.00},
		{account: "111", resourceID: "i-2", region: "us-east-1", service: "AmazonEC2", cost: 2.00},
		{account: "111", resourceID: "vol-1", region: "us-east-1", service: "AmazonEBS", cost: 0.50},
		{account: "222", resourceID: "i-1", region: "us-west-2", service: "AmazonEC2", cost: 3.00},
		{account: "111", resourceID: "missing", region: "us-east-1", service: "AmazonEC2", cost: 0.10}, // → unalloc
		{account: "111", resourceID: "", region: "", service: "AWSSupport", cost: 5.00},                // no_resource_id
	})

	resolver := &stubResolver{hits: map[[2]string]node.URN{
		{"111", "i-1"}:   "urn:ce:aws:111:compute/i-1",
		{"111", "i-2"}:   "urn:ce:aws:111:compute/i-2",
		{"111", "vol-1"}: "urn:ce:aws:111:persistence/vol-1",
		{"222", "i-1"}:   "urn:ce:aws:222:compute/i-1",
	}}

	engine := &Engine{
		Source:   &ClickHouseSource{Conn: c.Conn},
		Resolver: resolver,
		Sink:     &Sink{Conn: c.Conn, BatchSize: 1000},
		Provider: node.ProviderAWS,
		Dims:     DefaultResolvers(),
	}
	rep, err := engine.Run(context.Background(), period)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if math.Abs(rep.TotalCUR-totalCUR) > 1e-9 {
		t.Errorf("TotalCUR=%v, seed=%v", rep.TotalCUR, totalCUR)
	}
	delta := math.Abs(rep.TotalCUR - (rep.TotalAllocated + rep.TotalUnallocated))
	if delta > 1e-6 {
		t.Errorf("invariant violated: TotalCUR=%v alloc=%v unalloc=%v delta=%v",
			rep.TotalCUR, rep.TotalAllocated, rep.TotalUnallocated, delta)
	}

	// Confirma persistência: read-back via aggregação no ClickHouse.
	ctx := context.Background()
	var sumByService float64
	if err := c.Conn.QueryRow(ctx,
		chrepo.SumCostByURNSQL, period, string(DimensionService),
	).Scan(&sumByService); err != nil {
		t.Fatalf("read-back service: %v", err)
	}
	if math.Abs(sumByService-rep.TotalAllocated) > 1e-6 {
		t.Errorf("read-back sum(service)=%v want %v", sumByService, rep.TotalAllocated)
	}
	var sumUn float64
	if err := c.Conn.QueryRow(ctx,
		chrepo.SumUnallocatedSQL, period,
	).Scan(&sumUn); err != nil {
		t.Fatalf("read-back unalloc: %v", err)
	}
	if math.Abs(sumUn-rep.TotalUnallocated) > 1e-6 {
		t.Errorf("read-back unalloc=%v want %v", sumUn, rep.TotalUnallocated)
	}
}

// TestIntegration_AllocateIdempotent: rodar duas vezes o allocator no
// mesmo período não duplica linhas (RMT(allocated_at) deduplica).
func TestIntegration_AllocateIdempotent(t *testing.T) {
	c := freshCH(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	_ = seedCUR(t, c, period, []seedLine{
		{account: "111", resourceID: "i-1", region: "us-east-1", service: "AmazonEC2", cost: 1.00},
	})

	resolver := &stubResolver{hits: map[[2]string]node.URN{
		{"111", "i-1"}: "urn:ce:aws:111:compute/i-1",
	}}
	engine := &Engine{
		Source:   &ClickHouseSource{Conn: c.Conn},
		Resolver: resolver,
		Sink:     &Sink{Conn: c.Conn, BatchSize: 1000},
		Provider: node.ProviderAWS,
		Dims:     DefaultResolvers(),
	}

	// Run 1
	if _, err := engine.Run(context.Background(), period); err != nil {
		t.Fatalf("run1: %v", err)
	}
	// Sleep mínimo para garantir allocated_at distinto (RMT desempata por versão).
	time.Sleep(50 * time.Millisecond)
	// Run 2
	if _, err := engine.Run(context.Background(), period); err != nil {
		t.Fatalf("run2: %v", err)
	}

	ctx := context.Background()
	var n uint64
	if err := c.Conn.QueryRow(ctx,
		"SELECT count() FROM fct_cost_by_urn FINAL WHERE billing_period = ?", period,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	// 1 URN × 3 dims = 3 rows distintas após dedup FINAL.
	if n != 3 {
		t.Errorf("rows after dedup = %d, want 3", n)
	}
}
