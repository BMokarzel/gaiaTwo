//go:build integration

// Integration tests exercitam um ClickHouse real. Setup:
//
//	docker run --rm -p 9000:9000 -p 8123:8123 \
//	    -e CLICKHOUSE_DEFAULT_USER=default \
//	    -e CLICKHOUSE_DEFAULT_PASSWORD= \
//	    clickhouse/clickhouse-server
//
//	export CLICKHOUSE_TEST_URI=localhost:9000
//	make test-integration
//
// Sem `CLICKHOUSE_TEST_URI`, os testes skipam (não derrubam `make test`).
package clickhouse

import (
	"context"
	"os"
	"testing"
	"time"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/sink"
)

func testEnv(t *testing.T) (string, string, string) {
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

func freshDB(t *testing.T) *Client {
	t.Helper()
	uri, user, pass := testEnv(t)

	// Conecta no DB default para criar um DB isolado por test run.
	bootstrap, err := Open(context.Background(), Config{
		Addrs: []string{uri}, Database: "default", Username: user, Password: pass,
	})
	if err != nil {
		t.Fatalf("bootstrap open: %v", err)
	}
	defer bootstrap.Close()

	dbName := "ce_test_" + time.Now().UTC().Format("20060102_150405_000000")
	if err := bootstrap.Conn.Exec(context.Background(), "CREATE DATABASE IF NOT EXISTS "+dbName); err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		b, _ := Open(context.Background(), Config{Addrs: []string{uri}, Database: "default", Username: user, Password: pass})
		if b != nil {
			_ = b.Conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName)
			b.Close()
		}
	})

	c, err := Open(context.Background(), Config{
		Addrs: []string{uri}, Database: dbName, Username: user, Password: pass,
	})
	if err != nil {
		t.Fatalf("open db %s: %v", dbName, err)
	}
	t.Cleanup(func() { c.Close() })
	if err := c.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c
}

func mkLine(id string, ingestedAt time.Time, cost_ float64) cost.Line {
	return cost.Line{
		ReportID:           "rep",
		LineItemID:         id,
		BillingPeriodStart: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		UsageStart:         time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC),
		UsageEnd:           time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC),
		IngestedAt:         ingestedAt,
		Provider:           node.ProviderAWS,
		AccountID:          "111111111111",
		ResourceID:         "i-" + id,
		Service:            "AmazonEC2",
		Charge:             cost.ChargeUsage,
		Pricing:            cost.PricingOnDemand,
		UsageAmount:        1,
		UsageUnit:          "Hrs",
		BilledCost:         cost_,
		EffectiveCost:      cost_,
		Currency:           "USD",
		Tags:               map[string]string{"env": "prod"},
	}
}

// AC: schema migration cria as duas tabelas e é idempotente.
func TestIntegration_MigrationIdempotent(t *testing.T) {
	c := freshDB(t)
	// Roda migrate duas vezes — segunda deve ser no-op.
	if err := c.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}

	var n uint64
	row := c.Conn.QueryRow(context.Background(),
		"SELECT count() FROM system.tables WHERE database = currentDatabase() AND name IN ('fct_cur_lines','fct_cur_errors')")
	if err := row.Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 tables, got %d", n)
	}
}

// AC: sink insere linhas e elas são contadas (sem FINAL ainda).
func TestIntegration_SinkInsertsLines(t *testing.T) {
	c := freshDB(t)
	now := time.Now().UTC()

	s := &sink.ClickHouseSink{Conn: c.Conn, BatchSize: 100}
	lines := make(chan cost.Line, 3)
	errs := make(chan cost.ParseError)
	lines <- mkLine("a", now, 0.10)
	lines <- mkLine("b", now, 0.20)
	lines <- mkLine("c", now, 0.30)
	close(lines)
	close(errs)

	rep, err := s.Run(context.Background(), lines, errs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.LinesInserted != 3 {
		t.Errorf("LinesInserted = %d, want 3", rep.LinesInserted)
	}

	var got uint64
	if err := c.Conn.QueryRow(context.Background(),
		"SELECT count() FROM fct_cur_lines").Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != 3 {
		t.Errorf("count = %d, want 3", got)
	}
}

// AC F-004 D3 / S-006: reprocessar produz a versão mais recente via FINAL.
func TestIntegration_ReplacingMergeTreeDedup(t *testing.T) {
	c := freshDB(t)
	ctx := context.Background()

	t1 := time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 15, 8, 0, 0, 0, time.UTC)
	s := &sink.ClickHouseSink{Conn: c.Conn, BatchSize: 100}

	// Run 1: BilledCost=0.10
	run := func(at time.Time, cost float64) {
		t.Helper()
		lines := make(chan cost.Line, 1)
		errs := make(chan cost.ParseError)
		lines <- mkLine("a", at, cost)
		close(lines)
		close(errs)
		if _, err := s.Run(ctx, lines, errs); err != nil {
			t.Fatal(err)
		}
	}
	run(t1, 0.10)
	run(t2, 0.99)

	// FINAL deve retornar somente a versão de t2 (0.99).
	var billed float64
	row := c.Conn.QueryRow(ctx,
		"SELECT billed_cost FROM fct_cur_lines FINAL WHERE line_item_id = 'a'")
	if err := row.Scan(&billed); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if billed != 0.99 {
		t.Errorf("billed = %v after reprocess, want 0.99 (most recent ingested_at)", billed)
	}

	// CountLinesSQL deve retornar 1.
	var n uint64
	if err := c.Conn.QueryRow(ctx, CountLinesSQL,
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("FINAL count = %d, want 1 (dedup)", n)
	}
}

// AC: erros de parse vão para fct_cur_errors sem abortar o lote.
func TestIntegration_ErrorsTable(t *testing.T) {
	c := freshDB(t)
	s := &sink.ClickHouseSink{Conn: c.Conn, BatchSize: 100}

	lines := make(chan cost.Line)
	errs := make(chan cost.ParseError, 2)
	errs <- cost.ParseError{SourceFile: "x.parquet", RowIndex: 0, Reason: "bad", At: time.Now().UTC()}
	errs <- cost.ParseError{SourceFile: "x.parquet", RowIndex: 1, Reason: "bad", At: time.Now().UTC()}
	close(lines)
	close(errs)

	rep, err := s.Run(context.Background(), lines, errs)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ErrorsInserted != 2 {
		t.Errorf("ErrorsInserted = %d, want 2", rep.ErrorsInserted)
	}
	var n uint64
	if err := c.Conn.QueryRow(context.Background(),
		"SELECT count() FROM fct_cur_errors").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("errors count = %d, want 2", n)
	}
}
