package clickhouse

// Queries de leitura usadas por consumidores da camada cost.
//
// Importante: como a engine `ReplacingMergeTree(ingested_at)` faz merge
// em background, leituras sem `FINAL` podem retornar versões obsoletas
// até que a fusão ocorra. As consultas abaixo usam `FINAL` para garantir
// view determinística — trade-off é custo de query maior (CPU/IO), mas
// é o que casa com a expectativa de "reprocessei o CUR, quero ver o
// novo valor imediatamente" (F-004 D3).
const (
	// SelectLatestLinesSQL retorna a versão mais recente de cada linha
	// dentro de um BillingMonth. Use com:
	//
	//   SELECT * FROM fct_cur_lines FINAL
	//   WHERE billing_period_start = ?
	//
	// O ORDER BY do schema garante que apenas a maior `ingested_at`
	// sobrevive por chave (report_id, line_item_id, billing_period_start).
	SelectLatestLinesSQL = `SELECT * FROM fct_cur_lines FINAL WHERE billing_period_start = ?`

	// CountLinesSQL conta linhas distintas (pós-dedup) num período.
	// Útil em testes de idempotência e em sanity-checks pós-ingest.
	CountLinesSQL = `SELECT count() FROM fct_cur_lines FINAL WHERE billing_period_start = ?`

	// SelectCURLinesForAllocateSQL é o feed do allocator (F-005).
	// Retorna o conjunto mínimo de colunas necessárias para agregar
	// custo por `(account_id, resource_id)` num período.
	//
	// `FINAL` garante leitura pós-dedup do RMT — caro, mas usado uma
	// vez por execução do allocator.
	SelectCURLinesForAllocateSQL = `SELECT
		account_id, resource_id, region, service,
		effective_cost, currency
	FROM fct_cur_lines FINAL
	WHERE billing_period_start = ?`

	// SumCostByURNSQL agrega o custo já alocado num período (sanity check).
	SumCostByURNSQL = `SELECT sum(amount) FROM fct_cost_by_urn FINAL
		WHERE billing_period = ? AND dimension = ?`

	// SumUnallocatedSQL agrega o custo unallocated num período.
	SumUnallocatedSQL = `SELECT sum(amount) FROM fct_unallocated_cost FINAL
		WHERE billing_period = ?`

	// SelectUnallocatedSQL feeds F-006: lê toda linha unalloc de um
	// período (FINAL = pós-dedup do RMT). O shared allocator reaproveita
	// essas linhas como input.
	SelectUnallocatedSQL = `SELECT
		billing_period, account_id, service, resource_id, reason,
		amount, currency, lineage_count, allocated_at
	FROM fct_unallocated_cost FINAL
	WHERE billing_period = ?`

	// SelectDirectTotalsSQL feeds F-006: lê o já-alocado direct num
	// período, agregado por (urn, dimension, dimension_value). Usado
	// como denominador para regras `proportional_to_allocated` e
	// `by_destination`.
	SelectDirectTotalsSQL = `SELECT
		urn, dimension, dimension_value,
		sum(amount) AS amount, any(currency) AS currency
	FROM fct_cost_by_urn FINAL
	WHERE billing_period = ? AND allocation_type = 'direct'
	GROUP BY urn, dimension, dimension_value`
)
