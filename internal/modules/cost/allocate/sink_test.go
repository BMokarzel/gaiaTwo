package allocate

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository/clickhouse"
)

// fakeBatch implementa driver.Batch acumulando rows em memória.
type fakeBatch struct {
	rows [][]any
	sent bool
}

func (b *fakeBatch) Append(v ...any) error {
	row := make([]any, len(v))
	copy(row, v)
	b.rows = append(b.rows, row)
	return nil
}
func (b *fakeBatch) AppendStruct(any) error        { return nil }
func (b *fakeBatch) Abort() error                  { return nil }
func (b *fakeBatch) Send() error                   { b.sent = true; return nil }
func (b *fakeBatch) Flush() error                  { return nil }
func (b *fakeBatch) IsSent() bool                  { return b.sent }
func (b *fakeBatch) Rows() int                     { return len(b.rows) }
func (b *fakeBatch) Columns() []column.Interface   { return nil }
func (b *fakeBatch) Column(int) driver.BatchColumn { return nil }

type fakeBatcher struct {
	byQuery map[string]*fakeBatch
}

func (f *fakeBatcher) PrepareBatch(_ context.Context, q string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	if f.byQuery == nil {
		f.byQuery = map[string]*fakeBatch{}
	}
	b := f.byQuery[q]
	if b == nil {
		b = &fakeBatch{}
		f.byQuery[q] = b
	}
	return b, nil
}

func TestSink_WriteEmitsBothTables(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	bat := &fakeBatcher{}
	s := &Sink{Conn: bat, BatchSize: 100}

	rows := []Row{
		{URN: node.URN("urn:ce:aws:111:compute/i-1"), BillingPeriod: period, Dimension: DimensionService, DimensionValue: "AmazonEC2", Amount: 1.0, Currency: "USD", LineageCount: 1, AllocatedAt: now},
	}
	unalloc := []UnallocatedRow{
		{BillingPeriod: period, AccountID: "111", Service: "AmazonEC2", ResourceID: "", Reason: ReasonNoResourceID, Amount: 0.5, Currency: "USD", LineageCount: 1, AllocatedAt: now},
	}
	if err := s.Write(context.Background(), rows, unalloc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := bat.byQuery[clickhouse.InsertCostByURNSQL]; got == nil || got.Rows() != 1 || !got.IsSent() {
		t.Errorf("cost batch missing or unsent: %v", got)
	}
	if got := bat.byQuery[clickhouse.InsertUnallocatedSQL]; got == nil || got.Rows() != 1 || !got.IsSent() {
		t.Errorf("unalloc batch missing or unsent: %v", got)
	}
}

func TestSink_WriteEmpty(t *testing.T) {
	bat := &fakeBatcher{}
	s := &Sink{Conn: bat}
	if err := s.Write(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(bat.byQuery) != 0 {
		t.Errorf("no batch should have been prepared, got %v", bat.byQuery)
	}
}
