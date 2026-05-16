package allocate

import (
	"time"

	"costEngine/internal/entity/node"
)

// Project combina o estado do Aggregator com a Resolution e a lista de
// DimensionResolvers para emitir as Rows e UnallocatedRows finais.
//
// É a etapa que materializa o invariante:
//
//	sum(Rows.Amount por dimensão `service` ou `account` ou `region`)
//	  + sum(UnallocatedRows.Amount)
//	  ≈ aggregator.TotalCost
//
// O ≈ admite erro de arredondamento float64; em testes (S-008) usamos
// ε=1e-9 sobre o total e ε=1e-6 sobre cada dimensão.
//
// Custo: O(N × D) onde N = #buckets e D = #dimensões. Para 1M buckets
// × 3 dimensões = 3M iterações in-memory — sub-segundo em hardware
// moderno.
func Project(
	agg *Aggregator,
	res Resolution,
	dims []DimensionResolver,
	period time.Time,
	allocatedAt time.Time,
) ([]Row, []UnallocatedRow) {
	// rowKey é a chave de saída de fct_cost_by_urn.
	type rowKey struct {
		URN       node.URN
		Dim       Dimension
		DimValue  string
	}
	rowAgg := make(map[rowKey]*Row, len(agg.Buckets)*len(dims))

	// unalloc agrupado por (account, service, resource_id, reason). Não
	// agrupamos por currency aqui — se ficou misto, a marca "MIXED" vem
	// do bucket original.
	type ukey struct {
		Account, Service, ResourceID string
		Reason                       UnallocatedReason
	}
	unaggMap := make(map[ukey]*UnallocatedRow, len(agg.Unalloc)+len(res.Failures))

	addRow := func(k rowKey, amount float64, currency string, lineage int64) {
		r := rowAgg[k]
		if r == nil {
			r = &Row{
				URN:            k.URN,
				BillingPeriod:  period,
				Dimension:      k.Dim,
				DimensionValue: k.DimValue,
				AllocationType: AllocationDirect,
				Currency:       currency,
				AllocatedAt:    allocatedAt,
			}
			rowAgg[k] = r
		}
		r.Amount += amount
		r.LineageCount += lineage
		if r.Currency != currency && currency != "" {
			r.Currency = "MIXED"
		}
	}

	addUnalloc := func(k ukey, amount float64, currency string, lineage int64) {
		u := unaggMap[k]
		if u == nil {
			u = &UnallocatedRow{
				BillingPeriod: period,
				AccountID:     k.Account,
				Service:       k.Service,
				ResourceID:    k.ResourceID,
				Reason:        k.Reason,
				Currency:      currency,
				AllocatedAt:   allocatedAt,
			}
			unaggMap[k] = u
		}
		u.Amount += amount
		u.LineageCount += lineage
		if u.Currency != currency && currency != "" {
			u.Currency = "MIXED"
		}
	}

	// 1) Buckets com resource_id: resolver ou desviar para unallocated.
	for rk, b := range agg.Buckets {
		lk := resourceLookup{AccountID: rk.AccountID, ResourceID: rk.ResourceID}
		urn, found := res.URNByResource[lk]
		if !found {
			reason := res.Failures[lk]
			if reason == "" {
				reason = ReasonNotFound
			}
			addUnalloc(ukey{
				Account: rk.AccountID, Service: rk.Service,
				ResourceID: rk.ResourceID, Reason: reason,
			}, b.Amount, b.Currency, b.LineageCount)
			continue
		}
		for _, dr := range dims {
			v := dr.Resolve(rk, urn)
			if v == "" {
				continue
			}
			addRow(rowKey{URN: urn, Dim: dr.Dimension(), DimValue: v},
				b.Amount, b.Currency, b.LineageCount)
		}
	}

	// 2) Buckets sem resource_id: já são unallocated por construção.
	for k, b := range agg.Unalloc {
		addUnalloc(ukey{
			Account: k.AccountID, Service: k.Service,
			ResourceID: "", Reason: ReasonNoResourceID,
		}, b.Amount, b.Currency, b.LineageCount)
	}

	rows := make([]Row, 0, len(rowAgg))
	for _, r := range rowAgg {
		rows = append(rows, *r)
	}
	unalloc := make([]UnallocatedRow, 0, len(unaggMap))
	for _, u := range unaggMap {
		unalloc = append(unalloc, *u)
	}
	return rows, unalloc
}
