// Package sink persiste `cost.Line` no storage analítico.
//
// Implementação inicial: ClickHouse com tabela `fct_cur_lines`
// (ReplacingMergeTree por `(report_id, line_item_id, billing_period_start)`)
// — ver `repository/clickhouse/schema.go`.
//
// Linhas malformadas (`cost.ParseError`) vão para `fct_cur_errors` para
// auditoria, sem abortar o stream.
//
// Idempotência:
//
//   - O sink aceita reprocessamento de uma mesma partition: linhas com
//     mesma `(report_id, line_item_id, billing_period_start)` são
//     colapsadas pela `ReplacingMergeTree`, mantendo a com `ingested_at`
//     mais recente. Isso casa com o comportamento do CUR de reescrever
//     o mês corrente a cada update.
package sink
