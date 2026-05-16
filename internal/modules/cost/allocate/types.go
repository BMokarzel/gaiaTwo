package allocate

import (
	"time"

	"costEngine/internal/entity/node"
)

// Dimension é um eixo de agregação de custo. Cada execução do allocator
// expande uma linha CUR em N linhas em `fct_cost_by_urn` — uma por
// dimensão habilitada.
//
// Dimensões MVP (D3):
//
//   - DimensionService — vem direto de `cost.Line.Service` (ProductCode).
//   - DimensionAccount — derivado da URN (`urn:ce:aws:<account>:...`).
//   - DimensionRegion  — derivado da URN/Region da linha CUR.
//
// Dimensões futuras (DimensionTeam, DimensionCapability) ficam para
// quando F-010/F-011 (org plane) entregarem. O contrato `DimensionResolver`
// é estável.
type Dimension string

const (
	DimensionService Dimension = "service"
	DimensionAccount Dimension = "account"
	DimensionRegion  Dimension = "region"
)

// AllRequestedDimensions é o set default do CLI (sem dependência de org plane).
var AllRequestedDimensions = []Dimension{DimensionService, DimensionAccount, DimensionRegion}

// AllocationType distingue a origem da linha em `fct_cost_by_urn`:
//
//   - "direct" — F-005 atribuiu pela URN do recurso (1 CUR line → 1 URN).
//   - "shared" — F-006 rateou via SharedCostRule (taxa, suporte, egress).
//
// A coluna participa da ORDER BY do RMT, então direct e shared coexistem
// sem colidir em dedup.
type AllocationType string

const (
	AllocationDirect AllocationType = "direct"
	AllocationShared AllocationType = "shared"
)

// Row é uma linha de saída do allocator gravada em `fct_cost_by_urn`.
// Uma `cost.Line` de entrada gera N `Row`s — uma por dimensão habilitada.
//
// Chave de dedup no ClickHouse (ReplacingMergeTree):
//
//	(URN, BillingPeriod, Dimension, DimensionValue, AllocationType)
//
// `AllocatedAt` desempata (mais recente vence). `RuleID`/`RuleVersion`
// são audit-only para linhas `shared` (vazios em `direct`).
type Row struct {
	URN            node.URN       `json:"urn"`
	BillingPeriod  time.Time      `json:"billing_period"` // sempre dia 1 UTC do mês
	Dimension      Dimension      `json:"dimension"`
	DimensionValue string         `json:"dimension_value"`
	AllocationType AllocationType `json:"allocation_type"`
	RuleID         string         `json:"rule_id,omitempty"`
	RuleVersion    uint32         `json:"rule_version,omitempty"`
	Amount         float64        `json:"amount"`        // soma de EffectiveCost
	Currency       string         `json:"currency"`
	LineageCount   int64          `json:"lineage_count"` // # linhas CUR somadas
	AllocatedAt    time.Time      `json:"allocated_at"`
}

// UnallocatedRow representa custo que não pôde ser atribuído a uma URN.
// Causas: linha CUR sem `resource_id` (taxa, suporte, egress agregado),
// `resource_id` não encontrado no grafo (late-arriving infra), ou
// resolução ambígua (mesmo ARN em múltiplas contas).
//
// Consumido por F-006 (shared cost allocation).
type UnallocatedRow struct {
	BillingPeriod time.Time          `json:"billing_period"`
	AccountID     string             `json:"account_id"`
	Service       string             `json:"service"`
	ResourceID    string             `json:"resource_id,omitempty"`
	Reason        UnallocatedReason  `json:"reason"`
	Amount        float64            `json:"amount"`
	Currency      string             `json:"currency"`
	LineageCount  int64              `json:"lineage_count"`
	AllocatedAt   time.Time          `json:"allocated_at"`
}

// UnallocatedReason classifica por que uma linha não pôde ser atribuída.
type UnallocatedReason string

const (
	// ReasonNoResourceID — linha CUR sem `resource_id` (taxa, suporte,
	// egress agregado, fee de SP/RI). Caso mais comum (~30% do CUR).
	ReasonNoResourceID UnallocatedReason = "no_resource_id"

	// ReasonNotFound — `resource_id` presente mas sem URN correspondente
	// no grafo na AsOf usada. Pode ser late-arriving infra (o coletor
	// ainda não passou) ou recurso já decomissionado.
	ReasonNotFound UnallocatedReason = "not_found"

	// ReasonAmbiguous — wildcard cross-account resolveu múltiplas URNs.
	// Operador precisa desambiguar manualmente (ou esperar coletor cobrir
	// a conta que falta).
	ReasonAmbiguous UnallocatedReason = "ambiguous"
)

// Report sumariza o resultado de uma execução do allocator.
type Report struct {
	BillingPeriod time.Time
	CURLinesRead  int64 // # linhas FINAL lidas de fct_cur_lines
	UniqueResIDs  int   // # distintos (account, resource_id) agrupados
	Allocated     int64 // # Rows escritas em fct_cost_by_urn
	Unallocated   int64 // # UnallocatedRows escritas em fct_unallocated_cost

	// Invariante: TotalAllocated + TotalUnallocated ≈ TotalCUR
	// (diferença ≤ ε para arredondamento). Verificado em S-008.
	TotalCUR         float64
	TotalAllocated   float64
	TotalUnallocated float64

	Started, Finished time.Time
}

// Duration helper.
func (r Report) Duration() time.Duration { return r.Finished.Sub(r.Started) }
