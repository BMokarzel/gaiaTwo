package shared

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/allocate"
	"costEngine/internal/repository/clickhouse"
)

// fakeBatch — mesmo padrão de allocate/sink_test.go.
type fakeBatch struct{ rows int; sent bool }

func (b *fakeBatch) Append(v ...any) error         { b.rows++; return nil }
func (b *fakeBatch) AppendStruct(any) error        { return nil }
func (b *fakeBatch) Abort() error                  { return nil }
func (b *fakeBatch) Send() error                   { b.sent = true; return nil }
func (b *fakeBatch) Flush() error                  { return nil }
func (b *fakeBatch) IsSent() bool                  { return b.sent }
func (b *fakeBatch) Rows() int                     { return b.rows }
func (b *fakeBatch) Columns() []column.Interface   { return nil }
func (b *fakeBatch) Column(int) driver.BatchColumn { return nil }

type fakeConn struct {
	byQuery map[string]*fakeBatch
	execs   []string
}

func (f *fakeConn) PrepareBatch(_ context.Context, q string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
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
func (f *fakeConn) Exec(_ context.Context, q string, _ ...any) error {
	f.execs = append(f.execs, q)
	return nil
}

func TestRun_WritesBothTables(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Unix(1700000000, 0).UTC()
	out := Output{
		Shared: []allocate.Row{
			{URN: node.URN("u1"), BillingPeriod: period, Dimension: allocate.DimensionRegion, DimensionValue: "us", AllocationType: allocate.AllocationShared, RuleID: "r", RuleVersion: 1, Amount: 1, Currency: "USD", AllocatedAt: now},
		},
		RemainingUnalloc: []allocate.UnallocatedRow{
			{BillingPeriod: period, AccountID: "111", Service: "Y", Reason: allocate.ReasonNotFound, Amount: 0.5, Currency: "USD", AllocatedAt: now},
		},
	}
	c := &fakeConn{}
	if err := Run(context.Background(), c, out, 100); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := c.byQuery[clickhouse.InsertCostByURNSQL]; got == nil || got.Rows() != 1 || !got.IsSent() {
		t.Errorf("cost batch wrong: %v", got)
	}
	if got := c.byQuery[clickhouse.InsertUnallocatedSQL]; got == nil || got.Rows() != 1 || !got.IsSent() {
		t.Errorf("unalloc batch wrong: %v", got)
	}
}

func TestResetShared_IssuesExec(t *testing.T) {
	c := &fakeConn{}
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := ResetShared(context.Background(), c, period); err != nil {
		t.Fatal(err)
	}
	if len(c.execs) != 1 {
		t.Fatalf("execs=%d want 1", len(c.execs))
	}
	if want := "ALTER TABLE fct_cost_by_urn DELETE"; !contains(c.execs[0], want) {
		t.Errorf("exec=%q want contain %q", c.execs[0], want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
