package sink

import (
	"context"
	"testing"
	"time"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
)

// TestIdempotency_SameInputProducesSameDedupKey valida o pré-requisito
// da estratégia ReplacingMergeTree: re-processar o mesmo CUR produz
// linhas com chave de dedup idêntica. A engine então colapsa as
// versões em background, mantendo a `ingested_at` maior.
//
// Este teste é unidade-puro (sem ClickHouse). A validação ponta-a-ponta
// (que o `FINAL` realmente devolve só a versão nova) fica em S-008
// como integration test gated por `CLICKHOUSE_TEST_URI`.
func TestIdempotency_SameInputProducesSameDedupKey(t *testing.T) {
	mk := func(ingestedAt time.Time) cost.Line {
		return cost.Line{
			ReportID:           "rep",
			LineItemID:         "line-42",
			BillingPeriodStart: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			IngestedAt:         ingestedAt,
			Provider:           node.ProviderAWS,
			BilledCost:         0.10,
		}
	}
	run1 := mk(time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC))
	run2 := mk(time.Date(2026, 5, 15, 8, 0, 0, 0, time.UTC))

	r1, id1, p1 := run1.DedupKey()
	r2, id2, p2 := run2.DedupKey()
	if r1 != r2 || id1 != id2 || !p1.Equal(p2) {
		t.Errorf("dedup keys differ across reprocess: (%q,%q,%v) vs (%q,%q,%v)",
			r1, id1, p1, r2, id2, p2)
	}
	if !run2.IngestedAt.After(run1.IngestedAt) {
		t.Error("test setup: run2 must be after run1")
	}
}

// TestSink_ReprocessRespectsBatching valida que reprocessar uma mesma
// partition gera o mesmo número de Append (que serão colapsados pela
// engine, não pelo sink — o sink é cego à dedup).
func TestSink_ReprocessRespectsBatching(t *testing.T) {
	makeLines := func() []cost.Line {
		return []cost.Line{
			{ReportID: "rep", LineItemID: "a", BillingPeriodStart: time.Now()},
			{ReportID: "rep", LineItemID: "b", BillingPeriodStart: time.Now()},
		}
	}
	run := func() int64 {
		bc := &fakeBatcher{failOn: -1}
		s := &ClickHouseSink{Conn: bc, BatchSize: 10, FlushEach: time.Hour}
		lines := make(chan cost.Line, 4)
		errs := make(chan cost.ParseError)
		for _, l := range makeLines() {
			lines <- l
		}
		close(lines)
		close(errs)
		rep, err := s.Run(context.Background(), lines, errs)
		if err != nil {
			t.Fatal(err)
		}
		return rep.LinesInserted
	}
	if a, b := run(), run(); a != b {
		t.Errorf("reprocess inserted different counts: %d vs %d", a, b)
	}
}
