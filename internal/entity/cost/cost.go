// Package cost define os tipos de domínio da camada econômica.
//
// Princípios:
//
//   - `cost.Line` é o tipo silver — uma linha normalizada do CUR (ou
//     equivalente cross-cloud). NÃO é nó do grafo: custo é projeção
//     temporal sobre nós (ver `docs/architecture/02-cost.md` §1-2).
//   - Schema enxuto e estável; campos FOCUS-compatíveis quando possível.
//     Colunas exóticas do CUR ficam em `Tags` (livre).
//   - Bitemporal: `BillingPeriodStart`/`UsageStart`/`UsageEnd` são tempo
//     real; `IngestedAt` é tempo do sistema (transaction time).
//
// Pacote zero-dependência: pode ser importado por parser, sink, allocation
// engine e API sem risco de ciclo.
package cost

import "time"

// ChargeCategory alinhado com FOCUS 1.0.
//
// Mapeamento CUR v1 → FOCUS feito no parser:
//
//	Usage / DiscountedUsage / SavingsPlanCoveredUsage  → ChargeUsage
//	RIFee / SavingsPlanRecurringFee                    → ChargePurchase
//	Tax                                                → ChargeTax
//	Credit / Refund                                    → ChargeCredit
//	Fee / EdpDiscount                                  → ChargeAdjustment
type ChargeCategory string

const (
	ChargeUsage      ChargeCategory = "usage"
	ChargePurchase   ChargeCategory = "purchase"
	ChargeTax        ChargeCategory = "tax"
	ChargeAdjustment ChargeCategory = "adjustment"
	ChargeCredit     ChargeCategory = "credit"
)

// PricingModel descreve o regime de preço aplicado à linha.
type PricingModel string

const (
	PricingOnDemand    PricingModel = "on_demand"
	PricingReserved    PricingModel = "reserved"     // RIFee / DiscountedUsage de RI
	PricingSavingsPlan PricingModel = "savings_plan" // SavingsPlan*
	PricingSpot        PricingModel = "spot"
)

// Period representa um intervalo [Start, End).
type Period struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// IsZero reporta se o intervalo é o zero value.
func (p Period) IsZero() bool { return p.Start.IsZero() && p.End.IsZero() }

// Contains reporta se t ∈ [Start, End).
func (p Period) Contains(t time.Time) bool {
	if p.IsZero() {
		return false
	}
	return !t.Before(p.Start) && t.Before(p.End)
}
