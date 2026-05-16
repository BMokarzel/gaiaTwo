package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/parquet-go/parquet-go"

	"costEngine/internal/entity/cost"
)

// fixture mínima para Parquet em memória.
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

// fakeS3 implementa S3Client em memória.
type fakeS3 struct {
	// keys: full key → bytes
	objects map[string][]byte
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	prefix := ""
	if in.Prefix != nil {
		prefix = *in.Prefix
	}
	delim := ""
	if in.Delimiter != nil {
		delim = *in.Delimiter
	}
	out := &awss3.ListObjectsV2Output{}
	seenCP := map[string]struct{}{}
	for k, v := range f.objects {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		if delim != "" {
			if idx := strings.Index(rest, delim); idx >= 0 {
				cp := prefix + rest[:idx+1]
				if _, ok := seenCP[cp]; !ok {
					seenCP[cp] = struct{}{}
					cpCopy := cp
					out.CommonPrefixes = append(out.CommonPrefixes, s3types.CommonPrefix{Prefix: &cpCopy})
				}
				continue
			}
		}
		kcopy := k
		sz := int64(len(v))
		out.Contents = append(out.Contents, s3types.Object{Key: &kcopy, Size: &sz})
	}
	return out, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	v, ok := f.objects[*in.Key]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(v))}, nil
}

func writeParquet(t *testing.T, rows []curRow) []byte {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.parquet")
	if err := parquet.WriteFile(path, rows); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mkRow(id, rid string) curRow {
	return curRow{
		IdentityLineItemID:         id,
		BillBillingPeriodStartDate: "2026-05-01T00:00:00Z",
		LineItemUsageStartDate:     "2026-05-14T08:00:00Z",
		LineItemUsageEndDate:       "2026-05-14T09:00:00Z",
		LineItemUsageAccountID:     "111111111111",
		LineItemResourceID:         rid,
		LineItemProductCode:        "AmazonEC2",
		LineItemLineItemType:       "Usage",
		LineItemUnblendedCost:      0.1,
	}
}

func TestImporter_DiscoverViaManifest(t *testing.T) {
	parquetBytes := writeParquet(t, []curRow{mkRow("a", "i-1"), mkRow("b", "i-2")})
	manifestKey := "cur/my-report/year=2026/month=05/my-report-Manifest.json"
	dataKey := "cur/my-report/year=2026/month=05/data/00001.parquet"
	mf := manifest{AssemblyID: "assembly-42", ReportKeys: []string{dataKey}}
	mfBytes, _ := json.Marshal(mf)

	fake := &fakeS3{objects: map[string][]byte{
		manifestKey: mfBytes,
		dataKey:     parquetBytes,
	}}

	im := &Importer{
		Client:   fake,
		Bucket:   "test-bucket",
		Prefix:   "cur/my-report",
		ReportID: "my-report",
	}

	parts, err := im.Discover(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("want 1 partition, got %d", len(parts))
	}
	if parts[0].RunID != "assembly-42" {
		t.Errorf("RunID = %q, want assembly-42 (from manifest)", parts[0].RunID)
	}
	if len(parts[0].ObjectKeys) != 1 || parts[0].ObjectKeys[0] != dataKey {
		t.Errorf("ObjectKeys = %v", parts[0].ObjectKeys)
	}
	if parts[0].BillingMonth.Month() != time.May {
		t.Errorf("BillingMonth = %v", parts[0].BillingMonth)
	}
}

func TestImporter_DiscoverFallbackListing(t *testing.T) {
	parquetBytes := writeParquet(t, []curRow{mkRow("a", "i-1")})
	fake := &fakeS3{objects: map[string][]byte{
		"cur/r/year=2026/month=05/x.parquet": parquetBytes,
	}}

	im := &Importer{Client: fake, Bucket: "b", Prefix: "cur/r", ReportID: "r"}
	parts, err := im.Discover(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(parts) != 1 || parts[0].RunID != "s3-2026-05" {
		t.Fatalf("fallback runid: %+v", parts)
	}
}

func TestImporter_Read(t *testing.T) {
	parquetBytes := writeParquet(t, []curRow{mkRow("a", "i-1"), mkRow("b", "i-2")})
	dataKey := "cur/r/year=2026/month=05/data.parquet"
	fake := &fakeS3{objects: map[string][]byte{dataKey: parquetBytes}}

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	im := &Importer{
		Client:   fake,
		Bucket:   "b",
		Prefix:   "cur/r",
		ReportID: "r",
		NowFn:    func() time.Time { return now },
	}
	parts, err := im.Discover(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	res, err := im.Read(context.Background(), parts[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var got []cost.Line
	for l := range res.Lines {
		got = append(got, l)
	}
	for range res.Errors {
		t.Error("unexpected parse error")
	}
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if got[0].IngestedAt != now {
		t.Errorf("IngestedAt not propagated")
	}
	if got[0].SourceRunID != parts[0].RunID {
		t.Errorf("RunID not propagated: %q vs %q", got[0].SourceRunID, parts[0].RunID)
	}
}

func TestImporter_SinceFilter(t *testing.T) {
	parquetBytes := writeParquet(t, []curRow{mkRow("a", "i-1")})
	fake := &fakeS3{objects: map[string][]byte{
		"cur/r/year=2026/month=03/x.parquet": parquetBytes,
		"cur/r/year=2026/month=05/x.parquet": parquetBytes,
	}}
	im := &Importer{Client: fake, Bucket: "b", Prefix: "cur/r", ReportID: "r"}
	parts, err := im.Discover(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(parts) != 1 || parts[0].BillingMonth.Month() != time.May {
		t.Errorf("since filter: %+v", parts)
	}
}
