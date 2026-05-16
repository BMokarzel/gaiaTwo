package clickhouse

// Migrations define o schema da camada de custo no ClickHouse.
//
// Idempotentes (IF NOT EXISTS), seguros para rodar a cada boot. Não
// removem nada — schema evolution é manual por enquanto.
//
// Tabelas:
//
//   - `fct_cur_lines` — linhas normalizadas do CUR. Engine
//     `ReplacingMergeTree(ingested_at)` colapsa registros com mesma
//     chave (`report_id, line_item_id, billing_period_start`),
//     mantendo o `ingested_at` mais recente. Isso casa com o
//     comportamento do CUR de reescrever o mês corrente (F-004 D3).
//
//   - `fct_cur_errors` — linhas que falharam o parse, persistidas para
//     auditoria. Não bloqueia ingestão.
//
//   - `fct_cost_by_urn` — saída do allocator (F-005). Custo agregado
//     por URN × dimensão × período. Engine
//     `ReplacingMergeTree(allocated_at)` dedup por
//     `(urn, billing_period, dimension, dimension_value)` — reprocessar
//     um período sobrescreve a alocação anterior (F-005 D1).
//
//   - `fct_unallocated_cost` — linhas CUR que não couberam em nenhuma
//     URN (sem `resource_id`, recurso não encontrado, ou ambíguo).
//     Consumido por F-006 (shared cost allocation). Dedup por
//     `(billing_period, account_id, service, resource_id, reason)`.
var Migrations = []string{
	`CREATE TABLE IF NOT EXISTS fct_cur_lines (
		report_id              LowCardinality(String),
		line_item_id           String,
		billing_period_start   DateTime64(3, 'UTC'),
		usage_start            DateTime64(3, 'UTC'),
		usage_end              DateTime64(3, 'UTC'),
		ingested_at            DateTime64(3, 'UTC'),

		source_file            String,
		source_run_id          String,

		provider               LowCardinality(String),
		account_id             LowCardinality(String),
		account_urn            String,
		resource_id            String,
		resource_urn           String,
		region                 LowCardinality(String),
		region_urn             String,

		service                LowCardinality(String),
		usage_type             LowCardinality(String),
		operation              LowCardinality(String),
		charge_category        LowCardinality(String),
		pricing_model          LowCardinality(String),

		usage_amount           Float64,
		usage_unit             LowCardinality(String),
		list_cost              Float64,
		billed_cost            Float64,
		effective_cost         Float64,
		currency               LowCardinality(String),

		tags                   Map(String, String),
		cost_category          Map(String, String)
	)
	ENGINE = ReplacingMergeTree(ingested_at)
	PARTITION BY toYYYYMM(billing_period_start)
	ORDER BY (report_id, line_item_id, billing_period_start)
	SETTINGS index_granularity = 8192`,

	`CREATE TABLE IF NOT EXISTS fct_cur_errors (
		report_id     LowCardinality(String),
		run_id        String,
		source_file   String,
		row_index     Int64,
		reason        String,
		raw           String,
		at            DateTime64(3, 'UTC')
	)
	ENGINE = MergeTree
	PARTITION BY toYYYYMM(at)
	ORDER BY (report_id, run_id, source_file, row_index)`,

	`CREATE TABLE IF NOT EXISTS fct_cost_by_urn (
		urn               String,
		billing_period    DateTime64(3, 'UTC'),
		dimension         LowCardinality(String),
		dimension_value   LowCardinality(String),
		allocation_type   LowCardinality(String) DEFAULT 'direct',
		rule_id           String DEFAULT '',
		rule_version      UInt32 DEFAULT 0,
		amount            Float64,
		currency          LowCardinality(String),
		lineage_count     Int64,
		allocated_at      DateTime64(3, 'UTC')
	)
	ENGINE = ReplacingMergeTree(allocated_at)
	PARTITION BY toYYYYMM(billing_period)
	ORDER BY (urn, billing_period, dimension, dimension_value, allocation_type)
	SETTINGS index_granularity = 8192`,

	// F-006: migrações aditivas idempotentes para instalações existentes
	// (CREATE IF NOT EXISTS acima cobre fresh-installs; ALTERs abaixo
	// alinham bases que já existiam antes de F-006). ClickHouse aceita
	// ADD COLUMN IF NOT EXISTS e MODIFY ORDER BY estendendo a chave.
	`ALTER TABLE fct_cost_by_urn ADD COLUMN IF NOT EXISTS allocation_type LowCardinality(String) DEFAULT 'direct'`,
	`ALTER TABLE fct_cost_by_urn ADD COLUMN IF NOT EXISTS rule_id String DEFAULT ''`,
	`ALTER TABLE fct_cost_by_urn ADD COLUMN IF NOT EXISTS rule_version UInt32 DEFAULT 0`,
	`ALTER TABLE fct_cost_by_urn MODIFY ORDER BY (urn, billing_period, dimension, dimension_value, allocation_type)`,

	`CREATE TABLE IF NOT EXISTS fct_unallocated_cost (
		billing_period    DateTime64(3, 'UTC'),
		account_id        LowCardinality(String),
		service           LowCardinality(String),
		resource_id       String,
		reason            LowCardinality(String),
		amount            Float64,
		currency          LowCardinality(String),
		lineage_count     Int64,
		allocated_at      DateTime64(3, 'UTC')
	)
	ENGINE = ReplacingMergeTree(allocated_at)
	PARTITION BY toYYYYMM(billing_period)
	ORDER BY (billing_period, account_id, service, resource_id, reason)
	SETTINGS index_granularity = 8192`,
}

// SQL utilitários reutilizados pelo sink — definidos como constantes
// para inspeção fácil em testes.
const (
	insertLinesSQL = `INSERT INTO fct_cur_lines (
		report_id, line_item_id, billing_period_start, usage_start, usage_end, ingested_at,
		source_file, source_run_id,
		provider, account_id, account_urn, resource_id, resource_urn, region, region_urn,
		service, usage_type, operation, charge_category, pricing_model,
		usage_amount, usage_unit, list_cost, billed_cost, effective_cost, currency,
		tags, cost_category
	)`

	insertErrorsSQL = `INSERT INTO fct_cur_errors (
		report_id, run_id, source_file, row_index, reason, raw, at
	)`

	// InsertCostByURNSQL é usado pelo allocator (F-005 — allocation_type='direct')
	// e pelo shared allocator (F-006 — allocation_type='shared'+rule_id+rule_version).
	// Dedup pelo RMT ocorre na ORDER BY estendida: (urn, billing_period,
	// dimension, dimension_value, allocation_type). 'direct' e 'shared' não
	// colidem; rule_id é audit-only e não participa da chave de dedup.
	InsertCostByURNSQL = `INSERT INTO fct_cost_by_urn (
		urn, billing_period, dimension, dimension_value,
		allocation_type, rule_id, rule_version,
		amount, currency, lineage_count, allocated_at
	)`

	// InsertUnallocatedSQL é usado pelo allocator (F-005) para custo
	// que não foi atribuído a nenhuma URN.
	InsertUnallocatedSQL = `INSERT INTO fct_unallocated_cost (
		billing_period, account_id, service, resource_id, reason,
		amount, currency, lineage_count, allocated_at
	)`
)
