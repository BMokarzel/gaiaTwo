package allocate

import (
	"context"
	"fmt"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
)

// Engine orquestra um run de alocação fim-a-fim para um BillingPeriod.
//
// Pipeline (já documentado em doc.go):
//
//	Source.Stream → Aggregator.Collect
//	            → Resolve (bridge)
//	            → Project (dimensions)
//	            → Sink.Write (ou descarte se DryRun)
//
// Por que um struct e não free func: facilita injetar dependências em
// testes (cada peça é interface), e mantém o `Run` curto/legível.
type Engine struct {
	Source    LineSource
	Resolver  bridge.Resolver
	Sink      *Sink
	Provider  node.ProviderID
	Dims      []DimensionResolver
	DryRun    bool // true → calcula tudo mas não escreve no ClickHouse
}

// Run executa o pipeline para `period` (espera dia 1 UTC do mês).
func (e *Engine) Run(ctx context.Context, period time.Time) (Report, error) {
	rep := Report{BillingPeriod: period, Started: time.Now().UTC()}
	defer func() { rep.Finished = time.Now().UTC() }()

	if e.Source == nil {
		return rep, fmt.Errorf("engine: Source nil")
	}
	if e.Resolver == nil {
		return rep, fmt.Errorf("engine: Resolver nil")
	}
	if !e.DryRun && e.Sink == nil {
		return rep, fmt.Errorf("engine: Sink nil (and not dry-run)")
	}
	dims := e.Dims
	if len(dims) == 0 {
		dims = DefaultResolvers()
	}

	// 1) Agrega.
	agg := NewAggregator()
	if err := agg.Collect(ctx, e.Source, period); err != nil {
		return rep, fmt.Errorf("engine: collect: %w", err)
	}
	rep.CURLinesRead = agg.LinesRead
	rep.UniqueResIDs = agg.UniqueResIDs()
	rep.TotalCUR = agg.TotalCost

	// 2) Resolve URNs em batch.
	res, err := Resolve(ctx, e.Resolver, e.Provider, agg.ResourceIDsByAccount(), period)
	if err != nil {
		return rep, fmt.Errorf("engine: resolve: %w", err)
	}

	// 3) Project — converte buckets em Rows/UnallocatedRows.
	allocAt := time.Now().UTC()
	rows, unalloc := Project(agg, res, dims, period, allocAt)

	// 4) Sink (a menos que DryRun).
	if !e.DryRun {
		if err := e.Sink.Write(ctx, rows, unalloc); err != nil {
			return rep, fmt.Errorf("engine: sink: %w", err)
		}
	}

	rep.Allocated = int64(len(rows))
	rep.Unallocated = int64(len(unalloc))

	// Invariante macro: TotalAllocated por dimensão (escolhemos a
	// primeira dim como referência — todas devem ter mesma soma) +
	// TotalUnallocated ≈ TotalCUR.
	if len(dims) > 0 {
		ref := dims[0].Dimension()
		for _, r := range rows {
			if r.Dimension == ref {
				rep.TotalAllocated += r.Amount
			}
		}
	}
	for _, u := range unalloc {
		rep.TotalUnallocated += u.Amount
	}

	return rep, nil
}
