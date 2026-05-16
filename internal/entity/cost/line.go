package cost

import (
	"time"

	"costEngine/internal/entity/node"
)

// Line é uma linha normalizada do CUR (silver). Schema FOCUS-aligned com
// extensões mínimas internas (`SourceRunID`, `SourceFile`, `Tags`).
//
// Convenções:
//
//   - Tempos sempre em UTC.
//   - Custos em `Currency` (geralmente "USD"); não converter aqui.
//   - `ResourceURN`/`AccountURN`/`RegionURN` ficam vazios no parser — o
//     resolver/alocação preenche depois (ver `docs/architecture/02-cost.md`
//     §3 — "URN resolution é cache local").
//   - `LineItemID` é o identificador estável da linha dentro de
//     `(ReportID, BillingPeriodStart)`. Forma o triplete usado pelo sink
//     ClickHouse como chave de deduplicação (ReplacingMergeTree).
//
// Campos não-FOCUS exóticos do CUR vão em `Tags` (livre).
type Line struct {
	// Identidade / lineage
	ReportID    string `json:"report_id"`
	LineItemID  string `json:"line_item_id"`
	SourceFile  string `json:"source_file,omitempty"`
	SourceRunID string `json:"source_run_id,omitempty"`

	// Bitemporal
	BillingPeriodStart time.Time `json:"billing_period_start"`
	UsageStart         time.Time `json:"usage_start"`
	UsageEnd           time.Time `json:"usage_end"`
	IngestedAt         time.Time `json:"ingested_at"`

	// Bridge (raw CUR; URN resolution é responsabilidade de F-005)
	Provider    node.ProviderID `json:"provider"`
	AccountID   string          `json:"account_id"`
	AccountURN  node.URN        `json:"account_urn,omitempty"`
	ResourceID  string          `json:"resource_id,omitempty"`
	ResourceURN node.URN        `json:"resource_urn,omitempty"`
	Region      string          `json:"region,omitempty"`
	RegionURN   node.URN        `json:"region_urn,omitempty"`

	// Categorização
	Service   string         `json:"service"`              // ProductCode normalizado
	UsageType string         `json:"usage_type,omitempty"` // ex.: BoxUsage:m6i.large
	Operation string         `json:"operation,omitempty"`  // ex.: RunInstances
	Charge    ChargeCategory `json:"charge_category"`
	Pricing   PricingModel   `json:"pricing_model"`

	// Métrica
	UsageAmount   float64 `json:"usage_amount"`
	UsageUnit     string  `json:"usage_unit,omitempty"`
	ListCost      float64 `json:"list_cost"`      // on-demand público
	BilledCost    float64 `json:"billed_cost"`    // net_unblended
	EffectiveCost float64 `json:"effective_cost"` // amortizado (RI/SP)
	Currency      string  `json:"currency"`

	// Contexto (tags de negócio + cost categories)
	Tags         map[string]string `json:"tags,omitempty"`
	CostCategory map[string]string `json:"cost_category,omitempty"`
}

// HasResource indica se a linha está atribuída a um recurso específico.
// Linhas sem `ResourceID` (taxas, suporte, egress agregado) precisam de
// regra de alocação para serem cobradas a um URN (ver F-005).
func (l Line) HasResource() bool { return l.ResourceID != "" }

// DedupKey retorna a tripla usada pelo sink ClickHouse como chave de
// idempotência. ReplacingMergeTree colapsa entradas com mesma chave,
// mantendo a versão mais recente (`IngestedAt` desempata).
func (l Line) DedupKey() (string, string, time.Time) {
	return l.ReportID, l.LineItemID, l.BillingPeriodStart
}
