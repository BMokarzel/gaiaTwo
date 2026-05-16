package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
)

type curRow struct {
	IdentityLineItemID         string  `parquet:"identity_line_item_id"`
	BillBillingPeriodStartDate string  `parquet:"bill_billing_period_start_date"`
	LineItemUsageStartDate     string  `parquet:"line_item_usage_start_date"`
	LineItemUsageEndDate       string  `parquet:"line_item_usage_end_date"`
	LineItemUsageAccountID     string  `parquet:"line_item_usage_account_id"`
	LineItemResourceID         string  `parquet:"line_item_resource_id"`
	LineItemProductCode        string  `parquet:"line_item_product_code"`
	LineItemLineItemType       string  `parquet:"line_item_line_item_type"`
	LineItemUnblendedCost      float64 `parquet:"line_item_unblended_cost"`
}

func TestRunIngestCUR_DryRunLocal(t *testing.T) {
	dir := t.TempDir()
	row := curRow{
		IdentityLineItemID:         "a",
		BillBillingPeriodStartDate: "2026-05-01T00:00:00Z",
		LineItemUsageStartDate:     "2026-05-14T08:00:00Z",
		LineItemUsageEndDate:       "2026-05-14T09:00:00Z",
		LineItemUsageAccountID:     "111111111111",
		LineItemResourceID:         "i-1",
		LineItemProductCode:        "AmazonEC2",
		LineItemLineItemType:       "Usage",
		LineItemUnblendedCost:      0.1,
	}
	path := filepath.Join(dir, "x.parquet")
	if err := parquet.WriteFile(path, []curRow{row, row, row}); err != nil {
		t.Fatal(err)
	}

	// Redirect stdout to capture output.
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	err := runIngestCUR(context.Background(), []string{
		"--source=local",
		"--root=" + dir,
		"--report=test",
		"--dry-run",
	})
	w.Close()
	out, _ := readAll(r)

	if err != nil {
		t.Fatalf("runIngestCUR: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "3 linhas") {
		t.Errorf("expected '3 linhas' in output, got: %s", out)
	}
}

func TestRunIngestCUR_RequiresReport(t *testing.T) {
	err := runIngestCUR(context.Background(), []string{"--source=local", "--root=/tmp", "--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "report") {
		t.Fatalf("expected --report error, got: %v", err)
	}
}

func readAll(r *os.File) (string, error) {
	b, err := os.ReadFile(r.Name())
	if err != nil {
		// pipe — read until EOF
		var buf [4096]byte
		out := ""
		for {
			n, err := r.Read(buf[:])
			if n > 0 {
				out += string(buf[:n])
			}
			if err != nil {
				return out, nil
			}
		}
	}
	return string(b), nil
}
