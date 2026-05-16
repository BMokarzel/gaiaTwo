package shared

import (
	"context"
	"fmt"

	"costEngine/internal/modules/cost/allocate"
)

// SinkConn é o subset de driver.Conn usado pelo Run shim — espelha
// allocate.Batcher (PrepareBatch) e adiciona Exec para mutações
// (ALTER DELETE) usadas pelo `--reset`.
type SinkConn interface {
	allocate.Batcher
	Exec(ctx context.Context, query string, args ...any) error
}

// Run aplica `out` em ClickHouse via o Sink existente do allocate.
// `RemainingUnalloc` é regravado em `fct_unallocated_cost` (RMT dedup
// pela chave da tabela; `allocated_at` mais novo vence).
//
// O efeito desejado: reprocessar um período com o mesmo input produz
// o mesmo estado final em CH (idempotência via RMT). Mudança de
// regra produz novas linhas; as antigas convivem como versões mais
// antigas no MergeTree até OPTIMIZE — semântica padrão do RMT.
func Run(ctx context.Context, conn SinkConn, out Output, batchSize int) error {
	s := &allocate.Sink{Conn: conn, BatchSize: batchSize}
	return s.Write(ctx, out.Shared, out.RemainingUnalloc)
}

// ResetShared apaga via mutação leve todas as linhas
// `allocation_type='shared'` do `period`. Usado pelo CLI quando
// `--reset` é passado, antes de reprocessar — útil para o caso em
// que uma regra desaparece (URN que recebia shared não recebe mais)
// e RMT sozinho não limpa a "órfã".
//
// ClickHouse: `ALTER TABLE ... DELETE` é async; o caller deve assumir
// que pode haver leituras stale por alguns segundos depois.
func ResetShared(ctx context.Context, conn SinkConn, period any) error {
	const q = `ALTER TABLE fct_cost_by_urn DELETE
		WHERE allocation_type = 'shared' AND billing_period = ?`
	if err := conn.Exec(ctx, q, period); err != nil {
		return fmt.Errorf("reset shared: %w", err)
	}
	return nil
}
