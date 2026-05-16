package allocate

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"costEngine/internal/repository/clickhouse"
)

// ClickHouseSource lê linhas CUR diretamente do ClickHouse para um período.
// É uma `LineSource` que streama via cursor — sem materializar tudo em
// memória do lado do driver (o `Rows` do clickhouse-go é pull-based).
//
// Por que usar a query `SelectCURLinesForAllocateSQL` (em vez de
// `SelectLatestLinesSQL`):
//   - Allocator não precisa de 25 colunas; pegando só 6 reduz IO no
//     ClickHouse em ~70% para tabelas com tags/cost_category pesados.
//   - `FINAL` é caro e roda uma vez por execução do allocator.
type ClickHouseSource struct {
	Conn driver.Conn
}

// Stream implementa LineSource. `period` é o início do BillingMonth
// (dia 1 UTC) — bate com a partição do schema.
func (s *ClickHouseSource) Stream(ctx context.Context, period time.Time, yield func(RawLine) error) error {
	if s.Conn == nil {
		return fmt.Errorf("clickhouse source: Conn is nil")
	}
	rows, err := s.Conn.Query(ctx, clickhouse.SelectCURLinesForAllocateSQL, period)
	if err != nil {
		return fmt.Errorf("clickhouse source: query: %w", err)
	}
	defer rows.Close()

	var (
		accountID, resourceID, region, service, currency string
		effective                                        float64
	)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rows.Scan(&accountID, &resourceID, &region, &service, &effective, &currency); err != nil {
			return fmt.Errorf("clickhouse source: scan: %w", err)
		}
		if err := yield(RawLine{
			AccountID:     accountID,
			ResourceID:    resourceID,
			Region:        region,
			Service:       service,
			EffectiveCost: effective,
			Currency:      currency,
		}); err != nil {
			return err
		}
	}
	return rows.Err()
}
