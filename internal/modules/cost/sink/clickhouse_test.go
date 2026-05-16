package sink

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
)

// fakeBatch implementa driver.Batch acumulando rows em memória.
type fakeBatch struct {
	rows   [][]any
	sent   bool
	failOn int // -1 ⇒ nunca falha; >=0 ⇒ Append(idx==failOn) retorna erro
	idx    int
}

func (b *fakeBatch) Append(v ...any) error {
	if b.failOn >= 0 && b.idx == b.failOn {
		return errors.New("forced append failure")
	}
	b.idx++
	row := make([]any, len(v))
	copy(row, v)
	b.rows = append(b.rows, row)
	return nil
}
func (b *fakeBatch) AppendStruct(any) error  { return nil }
func (b *fakeBatch) Abort() error            { return nil }
func (b *fakeBatch) Send() error             { b.sent = true; return nil }
func (b *fakeBatch) Flush() error            { return nil }
func (b *fakeBatch) IsSent() bool            { return b.sent }
func (b *fakeBatch) Rows() int               { return len(b.rows) }
func (b *fakeBatch) Columns() []column.Interface { return nil }
func (b *fakeBatch) Column(int) driver.BatchColumn { return nil }

type fakeBatcher struct {
	batches []*fakeBatch
	failNew bool
	failOn  int
}

func (f *fakeBatcher) PrepareBatch(_ context.Context, _ string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	if f.failNew {
		return nil, errors.New("prepare failed")
	}
	b := &fakeBatch{failOn: f.failOn - 1}
	f.failOn = -1
	f.batches = append(f.batches, b)
	return b, nil
}

func mkLine(id string) cost.Line {
	t := time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC)
	return cost.Line{
		ReportID:           "rep",
		LineItemID:         id,
		BillingPeriodStart: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		UsageStart:         t,
		UsageEnd:           t.Add(time.Hour),
		IngestedAt:         t,
		Provider:           node.ProviderAWS,
		AccountID:          "111111111111",
		ResourceID:         "i-" + id,
		Service:            "AmazonEC2",
		Charge:             cost.ChargeUsage,
		Pricing:            cost.PricingOnDemand,
		BilledCost:         0.1,
		EffectiveCost:      0.1,
		Currency:           "USD",
		Tags:               map[string]string{"env": "prod"},
	}
}

func TestSink_BatchesAndFlushes(t *testing.T) {
	bc := &fakeBatcher{failOn: -1}
	s := &ClickHouseSink{Conn: bc, BatchSize: 2, FlushEach: time.Hour}

	lines := make(chan cost.Line, 5)
	errs := make(chan cost.ParseError)
	for i := 0; i < 5; i++ {
		lines <- mkLine(string(rune('a' + i)))
	}
	close(lines)
	close(errs)

	rep, err := s.Run(context.Background(), lines, errs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.LinesInserted != 5 {
		t.Errorf("LinesInserted = %d, want 5", rep.LinesInserted)
	}
	// 5 linhas / batch 2 ⇒ 3 batches (2,2,1)
	if rep.Batches != 3 {
		t.Errorf("Batches = %d, want 3", rep.Batches)
	}
	// Cada batch enviado deve ter IsSent=true.
	for i, b := range bc.batches {
		if !b.sent {
			t.Errorf("batch %d not sent", i)
		}
	}
}

func TestSink_FlushesErrors(t *testing.T) {
	bc := &fakeBatcher{failOn: -1}
	s := &ClickHouseSink{Conn: bc, BatchSize: 100, FlushEach: time.Hour}

	lines := make(chan cost.Line)
	close(lines)
	errs := make(chan cost.ParseError, 3)
	errs <- cost.ParseError{SourceFile: "f.parquet", RowIndex: 0, Reason: "bad row", At: time.Now()}
	errs <- cost.ParseError{SourceFile: "f.parquet", RowIndex: 1, Reason: "bad row", At: time.Now()}
	close(errs)

	rep, err := s.Run(context.Background(), lines, errs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.ErrorsInserted != 2 {
		t.Errorf("ErrorsInserted = %d, want 2", rep.ErrorsInserted)
	}
}

func TestSink_NilConn(t *testing.T) {
	s := &ClickHouseSink{}
	lines := make(chan cost.Line)
	close(lines)
	errs := make(chan cost.ParseError)
	close(errs)
	_, err := s.Run(context.Background(), lines, errs)
	if err == nil {
		t.Fatal("expected error for nil Conn")
	}
}

func TestSink_ContextCancellation(t *testing.T) {
	bc := &fakeBatcher{failOn: -1}
	s := &ClickHouseSink{Conn: bc, BatchSize: 100, FlushEach: time.Hour}

	lines := make(chan cost.Line)
	errs := make(chan cost.ParseError)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Run(ctx, lines, errs)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
