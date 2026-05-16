package shared

import (
	"fmt"
	"sort"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/allocate"
	"costEngine/internal/modules/cost/shared/rules"
)

// Input agrega o que o engine precisa para um período.
//
// `Unalloc` vem de `fct_unallocated_cost`; `AllocatedTotals` vem de
// `fct_cost_by_urn` filtrado por `allocation_type='direct'` (e o
// período em questão). Engine não toca em I/O — caller carrega.
type Input struct {
	Period          time.Time
	Unalloc         []allocate.UnallocatedRow
	AllocatedTotals []TotalByDim
	Rules           []rules.Rule
	AllocatedAt     time.Time // selo monotônico de saída (RMT tie-breaker)
}

// dimKey indexa pools por (dimension, dimension_value) — usado
// internamente para lookup de TotalByDim em O(1).
type dimKey struct{ Dim, Val string }

// TotalByDim é a projeção (urn, dimension, dimension_value → amount)
// usada por regras `proportional_to_allocated` para definir pesos.
// Apenas direct entra; shared não compõe base de proporcional (evita
// recursão entre regras).
type TotalByDim struct {
	URN            node.URN
	Dimension      string
	DimensionValue string
	Amount         float64
	Currency       string
}

// Output é o resultado de Apply.
//
// `Shared` deve ser persistido em `fct_cost_by_urn` (allocation_type='shared').
// `RemainingUnalloc` é o que nenhuma regra matchou — fica em
// `fct_unallocated_cost`. A invariante:
//
//	sum(input.Unalloc) ≈ sum(Shared) + sum(RemainingUnalloc)
//
// (ignorando linhas de drop por share=0 / pool vazio — documentadas em Drops).
type Output struct {
	Shared           []allocate.Row
	RemainingUnalloc []allocate.UnallocatedRow
	Drops            []Drop // diagnóstico — linhas matched mas sem pool
}

// Drop registra um caso em que uma regra matchou linhas mas não pôde
// distribuir (ex.: TypeProportional com pool vazio na dimensão). Vai
// para log do CLI; valor original NÃO é dobrado em RemainingUnalloc
// (fica lá como veio).
type Drop struct {
	RuleID  string
	Reason  string
	Amount  float64
	Lineage int64
}

// Apply orquestra: para cada regra (em ordem de id), filtra linhas que
// ela captura, distribui, marca como consumidas. Sobras vão para
// `RemainingUnalloc`.
//
// **Não-overlap garantido:** uma linha matched por regra X não pode
// ser matched por regra Y. A primeira regra (por id ordenado) ganha.
// Decisão D5 — se duas regras concorrem, é defeito de configuração.
func Apply(in Input) (Output, error) {
	out := Output{}
	if len(in.Unalloc) == 0 {
		return out, nil
	}

	// Filtra regras ativas no período (valid_from/valid_to).
	active := make([]rules.Rule, 0, len(in.Rules))
	for _, r := range in.Rules {
		if r.CoversPeriod(in.Period) {
			active = append(active, r)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })

	consumed := make([]bool, len(in.Unalloc))

	// Index de TotalByDim para lookup O(1) — chave (dimension, dim_value).
	totalsByDim := make(map[dimKey][]TotalByDim, len(in.AllocatedTotals))
	for _, t := range in.AllocatedTotals {
		k := dimKey{Dim: t.Dimension, Val: t.DimensionValue}
		totalsByDim[k] = append(totalsByDim[k], t)
	}
	// Soma por (dim → val → amount) para denominadores rápidos.
	sumByDim := make(map[string]map[string]float64, 4) // dim → val → sum
	for _, t := range in.AllocatedTotals {
		m := sumByDim[t.Dimension]
		if m == nil {
			m = map[string]float64{}
			sumByDim[t.Dimension] = m
		}
		m[t.DimensionValue] += t.Amount
	}

	for _, r := range active {
		// Coleta linhas matched ainda não consumidas.
		var matched []int
		for i, u := range in.Unalloc {
			if consumed[i] {
				continue
			}
			if matchRule(r, u) {
				matched = append(matched, i)
				consumed[i] = true
			}
		}
		if len(matched) == 0 {
			continue
		}
		switch r.Type {
		case rules.TypeProportional:
			emitProportional(&out, r, in, matched, sumByDim, totalsByDim)
		case rules.TypeByDestination:
			emitByDestination(&out, r, in, matched, totalsByDim)
		case rules.TypeStaticOverride:
			emitStaticOverride(&out, r, in, matched)
		default:
			return out, fmt.Errorf("engine: rule %q has unknown type %q", r.ID, r.Type)
		}
	}

	for i, u := range in.Unalloc {
		if !consumed[i] {
			out.RemainingUnalloc = append(out.RemainingUnalloc, u)
		}
	}
	return out, nil
}

func matchRule(r rules.Rule, u allocate.UnallocatedRow) bool {
	m := r.Match
	if m.Service != "" && m.Service != u.Service {
		return false
	}
	if m.AccountID != "" && m.AccountID != u.AccountID {
		return false
	}
	if m.ResourceIDEmpty && u.ResourceID != "" {
		return false
	}
	if len(m.Reasons) > 0 {
		found := false
		for _, want := range m.Reasons {
			if want == string(u.Reason) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// emitProportional rateia o total matched proporcional ao já-alocado
// na dimensão alvo. Pool vazio → Drop (não vira shared, não é
// re-arquivado como unalloc; cabe ao operador adicionar regra
// fallback ou redirecionar).
func emitProportional(out *Output, r rules.Rule, in Input, matched []int,
	sumByDim map[string]map[string]float64,
	totalsByDim map[dimKey][]TotalByDim,
) {
	dim := r.Target.Dimension
	pool := sumByDim[dim]
	if len(pool) == 0 || sumOf(pool) == 0 {
		var sum float64
		var lin int64
		for _, i := range matched {
			sum += in.Unalloc[i].Amount
			lin += in.Unalloc[i].LineageCount
		}
		out.Drops = append(out.Drops, Drop{
			RuleID: r.ID, Reason: "empty pool for dimension " + dim,
			Amount: sum, Lineage: lin,
		})
		return
	}

	totalMatched := 0.0
	currency := ""
	var lineage int64
	for _, i := range matched {
		u := in.Unalloc[i]
		totalMatched += u.Amount
		lineage += u.LineageCount
		if currency == "" {
			currency = u.Currency
		} else if currency != u.Currency {
			currency = "MIXED"
		}
	}

	// Re-projeta o pool em URNs (uma linha shared por URN-dimensão).
	// Cada URN na dimensão pega: total_matched * (urn_amount / pool_sum).
	denom := sumOf(pool)
	for dimVal, dimAmount := range pool {
		share := dimAmount / denom
		if share <= 0 {
			continue
		}
		// Para cada URN dentro desse dim_value:
		urns := totalsByDim[struct{ Dim, Val string }{Dim: dim, Val: dimVal}]
		urnDenom := 0.0
		for _, t := range urns {
			urnDenom += t.Amount
		}
		if urnDenom == 0 {
			continue
		}
		dimAllocation := totalMatched * share
		for _, t := range urns {
			urnShare := t.Amount / urnDenom
			amt := dimAllocation * urnShare
			if amt == 0 {
				continue
			}
			out.Shared = append(out.Shared, allocate.Row{
				URN:            t.URN,
				BillingPeriod:  in.Period,
				Dimension:      allocate.Dimension(dim),
				DimensionValue: dimVal,
				AllocationType: allocate.AllocationShared,
				RuleID:         r.ID,
				RuleVersion:    r.Version,
				Amount:         amt,
				Currency:       currency,
				LineageCount:   lineage, // lineage da regra (não por URN)
				AllocatedAt:    in.AllocatedAt,
			})
		}
	}
}

// emitByDestination atribui o matched ao pool de recursos da região
// alvo (proporcional ao já-alocado naquela região). Dimensão fixa =
// "region".
func emitByDestination(out *Output, r rules.Rule, in Input, matched []int,
	totalsByDim map[dimKey][]TotalByDim,
) {
	region := r.Target.Region
	pool := totalsByDim[struct{ Dim, Val string }{Dim: "region", Val: region}]
	if len(pool) == 0 {
		var sum float64
		var lin int64
		for _, i := range matched {
			sum += in.Unalloc[i].Amount
			lin += in.Unalloc[i].LineageCount
		}
		out.Drops = append(out.Drops, Drop{
			RuleID: r.ID, Reason: "no resources in destination region " + region,
			Amount: sum, Lineage: lin,
		})
		return
	}

	var total float64
	var lineage int64
	currency := ""
	for _, i := range matched {
		u := in.Unalloc[i]
		total += u.Amount
		lineage += u.LineageCount
		if currency == "" {
			currency = u.Currency
		} else if currency != u.Currency {
			currency = "MIXED"
		}
	}

	denom := 0.0
	for _, t := range pool {
		denom += t.Amount
	}
	if denom == 0 {
		out.Drops = append(out.Drops, Drop{
			RuleID: r.ID, Reason: "zero allocation in destination region " + region,
			Amount: total, Lineage: lineage,
		})
		return
	}
	for _, t := range pool {
		amt := total * (t.Amount / denom)
		if amt == 0 {
			continue
		}
		out.Shared = append(out.Shared, allocate.Row{
			URN:            t.URN,
			BillingPeriod:  in.Period,
			Dimension:      allocate.DimensionRegion,
			DimensionValue: region,
			AllocationType: allocate.AllocationShared,
			RuleID:         r.ID,
			RuleVersion:    r.Version,
			Amount:         amt,
			Currency:       currency,
			LineageCount:   lineage,
			AllocatedAt:    in.AllocatedAt,
		})
	}
}

// emitStaticOverride distribui pelas Shares fixas — URNs explícitas
// na regra. Sem pool, sem proporção: confiança total na configuração.
func emitStaticOverride(out *Output, r rules.Rule, in Input, matched []int) {
	var total float64
	var lineage int64
	currency := ""
	for _, i := range matched {
		u := in.Unalloc[i]
		total += u.Amount
		lineage += u.LineageCount
		if currency == "" {
			currency = u.Currency
		} else if currency != u.Currency {
			currency = "MIXED"
		}
	}
	// Ordena URNs para emissão determinística (testabilidade).
	urns := make([]string, 0, len(r.Target.Shares))
	for k := range r.Target.Shares {
		urns = append(urns, k)
	}
	sort.Strings(urns)
	for _, urn := range urns {
		amt := total * r.Target.Shares[urn]
		if amt == 0 {
			continue
		}
		out.Shared = append(out.Shared, allocate.Row{
			URN:            node.URN(urn),
			BillingPeriod:  in.Period,
			Dimension:      allocate.Dimension("static"),
			DimensionValue: r.ID,
			AllocationType: allocate.AllocationShared,
			RuleID:         r.ID,
			RuleVersion:    r.Version,
			Amount:         amt,
			Currency:       currency,
			LineageCount:   lineage,
			AllocatedAt:    in.AllocatedAt,
		})
	}
}

func sumOf(m map[string]float64) float64 {
	var s float64
	for _, v := range m {
		s += v
	}
	return s
}
