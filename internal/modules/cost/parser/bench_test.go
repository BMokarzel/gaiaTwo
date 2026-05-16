package parser

import (
	"testing"
	"time"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
)

// BenchmarkParse mede throughput de Parse num único row representativo.
// Útil para detectar regressões macro — não substitui benchmark
// end-to-end com Parquet (cabe em S3/local source).
func BenchmarkParse(b *testing.B) {
	ctx := Context{
		Provider:   node.ProviderAWS,
		ReportID:   "rep",
		RunID:      "run-1",
		SourceFile: "f.parquet",
		IngestedAt: time.Now().UTC(),
	}
	row := map[string]any{
		"identity_line_item_id":          "abc-line-42",
		"bill_billing_period_start_date": "2026-05-01T00:00:00Z",
		"line_item_usage_start_date":     "2026-05-14T08:00:00Z",
		"line_item_usage_end_date":       "2026-05-14T09:00:00Z",
		"line_item_usage_account_id":     "111111111111",
		"line_item_resource_id":          "i-abc",
		"line_item_product_code":         "AmazonEC2",
		"line_item_usage_type":           "BoxUsage:m6i.large",
		"line_item_operation":            "RunInstances",
		"line_item_line_item_type":       "Usage",
		"line_item_usage_amount":         1.0,
		"line_item_unblended_cost":       0.1,
		"line_item_net_unblended_cost":   0.09,
		"pricing_public_on_demand_cost":  0.10,
		"pricing_unit":                   "Hrs",
		"line_item_currency_code":        "USD",
		"product_region":                 "us-east-1",
		"resource_tags_user_env":         "prod",
		"resource_tags_user_team":        "platform",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(ctx, row); err != nil {
			b.Fatal(err)
		}
	}
}

// TestParseRate_BudgetCheck garante que Parse mantém >= 50k linhas/seg
// em condições "frias" (sem cache). 50k/s × 8 cores ≈ 400k lines/s
// agregado — bastante margem para o budget de 1M linhas em <30s do
// F-004 (S-008 AC).
func TestParseRate_BudgetCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := Context{
		Provider:   node.ProviderAWS,
		ReportID:   "rep",
		IngestedAt: time.Now().UTC(),
	}
	row := map[string]any{
		"identity_line_item_id":          "abc",
		"bill_billing_period_start_date": "2026-05-01T00:00:00Z",
		"line_item_usage_start_date":     "2026-05-14T08:00:00Z",
		"line_item_usage_end_date":       "2026-05-14T09:00:00Z",
		"line_item_usage_account_id":     "111111111111",
		"line_item_resource_id":          "i-abc",
		"line_item_product_code":         "AmazonEC2",
		"line_item_line_item_type":       "Usage",
		"line_item_unblended_cost":       0.1,
	}
	const n = 50_000
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := Parse(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	rate := float64(n) / elapsed.Seconds()
	t.Logf("parsed %d rows in %v → %.0f rows/s", n, elapsed, rate)
	if rate < 50_000 {
		t.Errorf("parser rate %.0f < budget 50000 rows/s", rate)
	}

	// Silence unused suspicion.
	_ = cost.Line{}
}
