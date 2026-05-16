package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"costEngine/internal/entity/cost"
)

// fixtureRow espelha o subset CUR usado pelos testes do parser.
type fixtureRow struct {
	IdentityLineItemID         string  `parquet:"identity_line_item_id"`
	BillBillingPeriodStartDate string  `parquet:"bill_billing_period_start_date"`
	LineItemUsageStartDate     string  `parquet:"line_item_usage_start_date"`
	LineItemUsageEndDate       string  `parquet:"line_item_usage_end_date"`
	LineItemUsageAccountID     string  `parquet:"line_item_usage_account_id"`
	LineItemResourceID         string  `parquet:"line_item_resource_id"`
	LineItemProductCode        string  `parquet:"line_item_product_code"`
	LineItemLineItemType       string  `parquet:"line_item_line_item_type"`
	LineItemUsageAmount        float64 `parquet:"line_item_usage_amount"`
	LineItemUnblendedCost      float64 `parquet:"line_item_unblended_cost"`
	LineItemCurrencyCode       string  `parquet:"line_item_currency_code"`
	ProductRegion              string  `parquet:"product_region"`
	ResourceTagsUserEnv        string  `parquet:"resource_tags_user_env,optional"`
}

func makeFixture(t *testing.T, dir string, rows []fixtureRow) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "data.parquet")
	if err := parquet.WriteFile(path, rows); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func threeRows() []fixtureRow {
	mk := func(id, rid string, cost float64) fixtureRow {
		return fixtureRow{
			IdentityLineItemID:         id,
			BillBillingPeriodStartDate: "2026-05-01T00:00:00Z",
			LineItemUsageStartDate:     "2026-05-14T08:00:00Z",
			LineItemUsageEndDate:       "2026-05-14T09:00:00Z",
			LineItemUsageAccountID:     "111111111111",
			LineItemResourceID:         rid,
			LineItemProductCode:        "AmazonEC2",
			LineItemLineItemType:       "Usage",
			LineItemUsageAmount:        1,
			LineItemUnblendedCost:      cost,
			LineItemCurrencyCode:       "USD",
			ProductRegion:              "us-east-1",
			ResourceTagsUserEnv:        "prod",
		}
	}
	return []fixtureRow{mk("a", "i-1", 0.10), mk("b", "i-2", 0.20), mk("c", "i-3", 0.30)}
}

func TestImporter_DiscoverFlatLayout(t *testing.T) {
	dir := t.TempDir()
	makeFixture(t, dir, threeRows())

	im := &Importer{Root: dir, ReportID: "rep"}
	parts, err := im.Discover(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("want 1 partition, got %d", len(parts))
	}
	if parts[0].RunID != "local-flat" || len(parts[0].ObjectKeys) != 1 {
		t.Errorf("unexpected partition: %+v", parts[0])
	}
}

func TestImporter_DiscoverPartitionedLayout(t *testing.T) {
	root := t.TempDir()
	makeFixture(t, filepath.Join(root, "year=2026", "month=05"), threeRows())
	makeFixture(t, filepath.Join(root, "year=2026", "month=04"), threeRows()[:1])

	im := &Importer{Root: root, ReportID: "rep"}
	parts, err := im.Discover(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("want 2 partitions, got %d", len(parts))
	}
	// Sorted by month ascending.
	if parts[0].BillingMonth.Month() != time.April || parts[1].BillingMonth.Month() != time.May {
		t.Errorf("order: %+v", parts)
	}
	if parts[1].RunID != "local-2026-05" {
		t.Errorf("RunID = %q", parts[1].RunID)
	}
}

func TestImporter_DiscoverSinceFilter(t *testing.T) {
	root := t.TempDir()
	makeFixture(t, filepath.Join(root, "year=2026", "month=03"), threeRows()[:1])
	makeFixture(t, filepath.Join(root, "year=2026", "month=05"), threeRows()[:1])

	im := &Importer{Root: root, ReportID: "rep"}
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	parts, err := im.Discover(context.Background(), since)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(parts) != 1 || parts[0].BillingMonth.Month() != time.May {
		t.Errorf("since filter failed: %+v", parts)
	}
}

func TestImporter_ReadStreamsLines(t *testing.T) {
	dir := t.TempDir()
	makeFixture(t, dir, threeRows())
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	im := &Importer{Root: dir, ReportID: "rep", NowFn: func() time.Time { return now }}
	parts, _ := im.Discover(context.Background(), time.Time{})

	res, err := im.Read(context.Background(), parts[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var got []cost.Line
	for line := range res.Lines {
		got = append(got, line)
	}
	for range res.Errors {
		t.Error("unexpected parse error")
	}

	if len(got) != 3 {
		t.Fatalf("got %d lines, want 3", len(got))
	}
	if got[0].IngestedAt != now {
		t.Errorf("IngestedAt not propagated: %v", got[0].IngestedAt)
	}
	if got[0].ReportID != "rep" || got[0].SourceRunID != "local-flat" {
		t.Errorf("lineage missing: %+v", got[0])
	}
	if got[0].Tags["env"] != "prod" {
		t.Errorf("tags missing: %v", got[0].Tags)
	}
}

func TestImporter_ReadCancelsOnContext(t *testing.T) {
	dir := t.TempDir()
	// Mais linhas para garantir que cancelamento ocorre antes do fim.
	rows := make([]fixtureRow, 5000)
	base := threeRows()[0]
	for i := range rows {
		base.IdentityLineItemID = "row-" + string(rune('a'+i%26))
		rows[i] = base
	}
	makeFixture(t, dir, rows)

	im := &Importer{Root: dir, ReportID: "rep", BufferSize: 4}
	parts, _ := im.Discover(context.Background(), time.Time{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := im.Read(ctx, parts[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// Lê algumas linhas e cancela.
	count := 0
	for line := range res.Lines {
		_ = line
		count++
		if count == 10 {
			cancel()
		}
	}
	for range res.Errors {
	}
	if count < 10 {
		t.Errorf("got %d before cancel", count)
	}
}

func TestImporter_ZeroPartitionReturnsErr(t *testing.T) {
	im := &Importer{Root: t.TempDir(), ReportID: "rep"}
	_, err := im.Read(context.Background(), cost.Partition{})
	if err != cost.ErrPartitionNotFound {
		t.Fatalf("err = %v, want ErrPartitionNotFound", err)
	}
}
