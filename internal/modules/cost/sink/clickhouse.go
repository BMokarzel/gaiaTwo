package sink

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"costEngine/internal/entity/cost"
)

// ClickHouseSink persiste `cost.Line` (e `cost.ParseError`) no ClickHouse.
//
// Padrão de uso:
//
//	sink := &ClickHouseSink{Conn: conn, BatchSize: 5000}
//	report, err := sink.Run(ctx, lines, errors)
//
// `Run` consome ambos os canais até fecharem (ou ctx cancelar) e
// devolve um `Report` com contagens e timings.
//
// Idempotência: a tabela usa `ReplacingMergeTree(ingested_at)` com
// chave `(report_id, line_item_id, billing_period_start)`. Re-inserir
// a mesma chave é seguro — a engine colapsa em background.
type ClickHouseSink struct {
	Conn      Batcher
	BatchSize int           // default 5000
	FlushEach time.Duration // default 5s
}

// Batcher é o subset de `driver.Conn` que o sink precisa. Permite mock
// nos testes sem stub do `driver.Conn` inteiro.
type Batcher interface {
	PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
}

// Report sumariza o resultado de uma execução do sink.
type Report struct {
	LinesInserted  int64
	ErrorsInserted int64
	Batches        int
	Started        time.Time
	Finished       time.Time
}

// Duration é um helper para logs.
func (r Report) Duration() time.Duration { return r.Finished.Sub(r.Started) }

// Run drena `lines` e `errs` até fecharem. Lines não-fatais (ParseError)
// vão para `fct_cur_errors`. Erros fatais (falha de INSERT) propagam.
func (s *ClickHouseSink) Run(ctx context.Context, lines <-chan cost.Line, errs <-chan cost.ParseError) (Report, error) {
	if s.Conn == nil {
		return Report{}, fmt.Errorf("sink: Conn nil")
	}
	batchSize := s.BatchSize
	if batchSize <= 0 {
		batchSize = 5000
	}
	flushEach := s.FlushEach
	if flushEach <= 0 {
		flushEach = 5 * time.Second
	}

	report := Report{Started: time.Now().UTC()}
	defer func() { report.Finished = time.Now().UTC() }()

	pendingLines := make([]cost.Line, 0, batchSize)
	pendingErrors := make([]cost.ParseError, 0, batchSize)
	ticker := time.NewTicker(flushEach)
	defer ticker.Stop()

	flushLines := func() error {
		if len(pendingLines) == 0 {
			return nil
		}
		if err := insertLines(ctx, s.Conn, pendingLines); err != nil {
			return err
		}
		report.LinesInserted += int64(len(pendingLines))
		report.Batches++
		pendingLines = pendingLines[:0]
		return nil
	}
	flushErrors := func() error {
		if len(pendingErrors) == 0 {
			return nil
		}
		if err := insertErrors(ctx, s.Conn, pendingErrors); err != nil {
			return err
		}
		report.ErrorsInserted += int64(len(pendingErrors))
		pendingErrors = pendingErrors[:0]
		return nil
	}

	linesDone, errsDone := lines == nil, errs == nil
	for !linesDone || !errsDone {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		case <-ticker.C:
			if err := flushLines(); err != nil {
				return report, err
			}
			if err := flushErrors(); err != nil {
				return report, err
			}
		case line, ok := <-lines:
			if !ok {
				linesDone = true
				lines = nil
				continue
			}
			pendingLines = append(pendingLines, line)
			if len(pendingLines) >= batchSize {
				if err := flushLines(); err != nil {
					return report, err
				}
			}
		case perr, ok := <-errs:
			if !ok {
				errsDone = true
				errs = nil
				continue
			}
			pendingErrors = append(pendingErrors, perr)
			if len(pendingErrors) >= batchSize {
				if err := flushErrors(); err != nil {
					return report, err
				}
			}
		}
	}

	if err := flushLines(); err != nil {
		return report, err
	}
	if err := flushErrors(); err != nil {
		return report, err
	}
	return report, nil
}

func insertLines(ctx context.Context, conn Batcher, batch []cost.Line) error {
	b, err := conn.PrepareBatch(ctx, insertLinesSQL)
	if err != nil {
		return fmt.Errorf("sink: prepare lines batch: %w", err)
	}
	for _, l := range batch {
		if err := b.Append(
			l.ReportID, l.LineItemID, l.BillingPeriodStart, l.UsageStart, l.UsageEnd, l.IngestedAt,
			l.SourceFile, l.SourceRunID,
			string(l.Provider), l.AccountID, string(l.AccountURN), l.ResourceID, string(l.ResourceURN), l.Region, string(l.RegionURN),
			l.Service, l.UsageType, l.Operation, string(l.Charge), string(l.Pricing),
			l.UsageAmount, l.UsageUnit, l.ListCost, l.BilledCost, l.EffectiveCost, l.Currency,
			nilSafeMap(l.Tags), nilSafeMap(l.CostCategory),
		); err != nil {
			return fmt.Errorf("sink: append line: %w", err)
		}
	}
	return b.Send()
}

func insertErrors(ctx context.Context, conn Batcher, batch []cost.ParseError) error {
	b, err := conn.PrepareBatch(ctx, insertErrorsSQL)
	if err != nil {
		return fmt.Errorf("sink: prepare errors batch: %w", err)
	}
	for _, e := range batch {
		if err := b.Append("", "", e.SourceFile, e.RowIndex, e.Reason, e.Raw, e.At); err != nil {
			return fmt.Errorf("sink: append error: %w", err)
		}
	}
	return b.Send()
}

// insertLinesSQL/insertErrorsSQL espelham o schema. Mantidos como
// strings literais (não via reflection) para evitar surpresas com
// ordem de colunas no ClickHouse — append posicional é estrito.
const (
	insertLinesSQL  = `INSERT INTO fct_cur_lines`
	insertErrorsSQL = `INSERT INTO fct_cur_errors`
)

// nilSafeMap garante que `Map(String, String)` no ClickHouse não receba nil.
func nilSafeMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
