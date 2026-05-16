package local

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
)

// curRow é uma versão minimalista de linha CUR — suficiente para
// validar que o reader devolve colunas com nomes esperados e tipos
// primitivos Go corretos.
type curRow struct {
	LineItemUsageAccountID string  `parquet:"line_item_usage_account_id"`
	LineItemResourceID     string  `parquet:"line_item_resource_id"`
	LineItemProductCode    string  `parquet:"line_item_product_code"`
	LineItemUsageAmount    float64 `parquet:"line_item_usage_amount"`
	LineItemUnblendedCost  float64 `parquet:"line_item_unblended_cost"`
	ResourceTagsUserEnv    string  `parquet:"resource_tags_user_env,optional"`
}

func writeFixture(t *testing.T, rows []curRow) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cur.parquet")
	if err := parquet.WriteFile(path, rows); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestReader_StreamsRows(t *testing.T) {
	rows := []curRow{
		{
			LineItemUsageAccountID: "111111111111",
			LineItemResourceID:     "i-abc",
			LineItemProductCode:    "AmazonEC2",
			LineItemUsageAmount:    1.5,
			LineItemUnblendedCost:  0.087,
			ResourceTagsUserEnv:    "prod",
		},
		{
			LineItemUsageAccountID: "111111111111",
			LineItemResourceID:     "vol-xyz",
			LineItemProductCode:    "AmazonEC2",
			LineItemUsageAmount:    100,
			LineItemUnblendedCost:  0.50,
			ResourceTagsUserEnv:    "",
		},
	}
	path := writeFixture(t, rows)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if got := r.NumRows(); got != 2 {
		t.Errorf("NumRows = %d, want 2", got)
	}

	cols := r.Columns()
	want := map[string]bool{
		"line_item_usage_account_id": true,
		"line_item_resource_id":      true,
		"line_item_product_code":     true,
		"line_item_usage_amount":     true,
		"line_item_unblended_cost":   true,
		"resource_tags_user_env":     true,
	}
	for _, c := range cols {
		delete(want, c)
	}
	if len(want) != 0 {
		t.Errorf("missing columns: %v (got %v)", want, cols)
	}

	var read []RawRow
	for {
		row, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		read = append(read, row)
	}
	if len(read) != 2 {
		t.Fatalf("read %d rows, want 2", len(read))
	}

	if got := read[0]["line_item_resource_id"]; got != "i-abc" {
		t.Errorf("row0 resource = %v, want i-abc", got)
	}
	if got := read[0]["line_item_usage_amount"]; got != 1.5 {
		t.Errorf("row0 amount = %v, want 1.5", got)
	}
	if got := read[0]["resource_tags_user_env"]; got != "prod" {
		t.Errorf("row0 env = %v, want prod", got)
	}
	// row1 has empty optional tag → may be absent or empty string;
	// the writer produces an empty string for non-nullable string,
	// optional should produce null (key absent).
	if v, ok := read[1]["resource_tags_user_env"]; ok && v != "" {
		t.Errorf("row1 env = %v, want absent or empty", v)
	}
}

func TestReader_LargeBatchSpansBuffer(t *testing.T) {
	// Excede o buffer interno (256) para validar refill.
	rows := make([]curRow, 1000)
	for i := range rows {
		rows[i] = curRow{
			LineItemUsageAccountID: "111111111111",
			LineItemProductCode:    "AmazonS3",
			LineItemUsageAmount:    float64(i),
			LineItemUnblendedCost:  float64(i) * 0.001,
		}
	}
	path := writeFixture(t, rows)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	count := 0
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next at %d: %v", count, err)
		}
		count++
	}
	if count != 1000 {
		t.Errorf("read %d rows, want 1000", count)
	}
}

func TestOpen_MissingFile(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "nope.parquet"))
	if err == nil {
		t.Fatal("expected error opening missing file")
	}
}

func TestOpen_NotParquet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.parquet")
	if err := os.WriteFile(path, []byte("not parquet"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path)
	if err == nil {
		t.Fatal("expected error opening non-parquet file")
	}
}
