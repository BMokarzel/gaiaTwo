// Package allocate é o motor de alocação de custos (F-005).
//
// Lê linhas normalizadas de `fct_cur_lines` (F-004), resolve cada
// `resource_id` para a URN canônica via `modules/bridge` (F-003), e
// projeta o custo em N dimensões (service, account, region, ...),
// gravando em `fct_cost_by_urn`. Linhas sem URN resolvível vão para
// `fct_unallocated_cost` (consumido por F-006).
//
// Pipeline:
//
//	fct_cur_lines (FINAL)
//	    → Aggregator: stream + group by (account, resource_id)
//	    → Resolver: batch resolve URN com AsOf = period_end
//	    → DimensionResolver(*): expande dimensões por URN
//	    → Sink: batch insert em fct_cost_by_urn / fct_unallocated_cost
//
// Idempotência: `ReplacingMergeTree(allocated_at)` por
// `(urn, billing_period, dimension, dimension_value)` — reprocessar
// um período sobrescreve a alocação anterior.
//
// Imports permitidos: `entity/cost`, `entity/node`, `modules/bridge`,
// `repository`, `repository/clickhouse`.
package allocate
