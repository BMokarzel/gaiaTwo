package allocate

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"costEngine/internal/repository/clickhouse"
)

// Batcher é o subset de driver.Conn que o sink usa — espelha o mesmo
// padrão do sink de CUR (F-004) para permitir mock em testes sem
// stub do driver.Conn inteiro.
type Batcher interface {
	PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
}

// Sink persiste o resultado do allocator em ClickHouse. Bounded —
// recebe slices, não canais — porque `Project` já agrega tudo
// in-memory.
//
// Tabelas alvo:
//   - fct_cost_by_urn (Rows)
//   - fct_unallocated_cost (UnallocatedRows)
//
// Idempotência: ReplacingMergeTree(allocated_at). Caller garante que
// `allocated_at` é monotônico crescente por execução (uma execução do
// allocator > all prior allocated_at) — ver F-005 D1.
type Sink struct {
	Conn      Batcher
	BatchSize int // default 5000
}

// Write insere `rows` e `unalloc`. Erro fatal aborta — não há partial
// commit. Como ambas as tabelas usam RMT(allocated_at), reexecutar
// dedup automaticamente.
func (s *Sink) Write(ctx context.Context, rows []Row, unalloc []UnallocatedRow) error {
	if s.Conn == nil {
		return fmt.Errorf("allocate sink: Conn nil")
	}
	batchSize := s.BatchSize
	if batchSize <= 0 {
		batchSize = 5000
	}

	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		if err := s.insertRows(ctx, rows[i:end]); err != nil {
			return err
		}
	}
	for i := 0; i < len(unalloc); i += batchSize {
		end := i + batchSize
		if end > len(unalloc) {
			end = len(unalloc)
		}
		if err := s.insertUnalloc(ctx, unalloc[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sink) insertRows(ctx context.Context, batch []Row) error {
	if len(batch) == 0 {
		return nil
	}
	b, err := s.Conn.PrepareBatch(ctx, clickhouse.InsertCostByURNSQL)
	if err != nil {
		return fmt.Errorf("allocate sink: prepare rows batch: %w", err)
	}
	for _, r := range batch {
		// allocation_type vazio (compatibilidade com chamadores anteriores
		// a F-006) defaulta para "direct".
		at := r.AllocationType
		if at == "" {
			at = AllocationDirect
		}
		if err := b.Append(
			string(r.URN), r.BillingPeriod, string(r.Dimension), r.DimensionValue,
			string(at), r.RuleID, r.RuleVersion,
			r.Amount, r.Currency, r.LineageCount, r.AllocatedAt,
		); err != nil {
			return fmt.Errorf("allocate sink: append row: %w", err)
		}
	}
	return b.Send()
}

func (s *Sink) insertUnalloc(ctx context.Context, batch []UnallocatedRow) error {
	if len(batch) == 0 {
		return nil
	}
	b, err := s.Conn.PrepareBatch(ctx, clickhouse.InsertUnallocatedSQL)
	if err != nil {
		return fmt.Errorf("allocate sink: prepare unalloc batch: %w", err)
	}
	for _, u := range batch {
		if err := b.Append(
			u.BillingPeriod, u.AccountID, u.Service, u.ResourceID, string(u.Reason),
			u.Amount, u.Currency, u.LineageCount, u.AllocatedAt,
		); err != nil {
			return fmt.Errorf("allocate sink: append unalloc: %w", err)
		}
	}
	return b.Send()
}
