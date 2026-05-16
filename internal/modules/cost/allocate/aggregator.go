package allocate

import (
	"context"
	"time"
)

// RawLine é a projeção mínima de `fct_cur_lines` consumida pelo allocator.
// Mantida separada de `cost.Line` (que carrega 20+ colunas) porque o
// allocator só precisa do essencial para agregar e dimensionar — toda
// outra coluna (UsageType, Operation, Tags, etc.) é dropada na consulta
// para economizar IO no ClickHouse.
type RawLine struct {
	AccountID     string
	ResourceID    string // "" → linha sem recurso (taxa, suporte, ...).
	Region        string
	Service       string
	EffectiveCost float64
	Currency      string
}

// LineSource é o contrato de fonte de linhas CUR. A implementação
// produção (`clickhouseSource`) faz `SELECT ... FROM fct_cur_lines FINAL`
// e streama via callback. Testes usam um fake in-memory.
//
// O contrato é callback (não channel) para não obrigar a impl a girar
// goroutine — leitura de driver SQL já é pull-based.
type LineSource interface {
	// Stream chama yield(line) para cada linha do período. Aborta no
	// primeiro erro do yield (permite cancelar agregação).
	Stream(ctx context.Context, period time.Time, yield func(RawLine) error) error
}

// resourceKey identifica um bucket de agregação. Inclui Region e Service
// porque ambos são dimensões de saída — duas linhas com mesmo
// (account, resource_id) mas Region diferente são raras (geralmente
// impossível para um único recurso), mas se acontecerem em CUR
// dirty/cross-region precisamos preservar para não perder dimensão.
type resourceKey struct {
	AccountID  string
	ResourceID string
	Region     string
	Service    string
}

// bucket acumula custo por resourceKey.
type bucket struct {
	Amount       float64
	LineageCount int64
	Currency     string // se ficar misto, vira "MIXED" e o report sinaliza.
}

// unallocatedKey agrupa linhas sem resource_id para emissão direta em
// fct_unallocated_cost com reason=no_resource_id. NÃO passa pelo
// resolver (não há o que resolver).
type unallocatedKey struct {
	AccountID string
	Service   string
}

// Aggregator percorre as linhas CUR de um período e produz dois mapas:
//
//   - Buckets:     (account, resource_id, region, service) → custo agregado
//   - Unalloc:     (account, service) → custo agregado sem resource_id
//
// É puramente in-memory. Limite prático: ~1M tuplas distintas usam
// ~150MB com overhead de map (chave string × 4 + struct). Para tenants
// maiores, a estratégia futura é shardar por account ou push-down para
// SQL (`GROUP BY` no ClickHouse).
type Aggregator struct {
	Buckets map[resourceKey]*bucket
	Unalloc map[unallocatedKey]*bucket

	// stats observados
	LinesRead int64
	TotalCost float64
}

// NewAggregator constrói um Aggregator vazio.
func NewAggregator() *Aggregator {
	return &Aggregator{
		Buckets: make(map[resourceKey]*bucket, 1024),
		Unalloc: make(map[unallocatedKey]*bucket, 64),
	}
}

// Collect drena `src` para o período `period`. Erros do source abortam.
func (a *Aggregator) Collect(ctx context.Context, src LineSource, period time.Time) error {
	return src.Stream(ctx, period, func(l RawLine) error {
		a.add(l)
		return nil
	})
}

func (a *Aggregator) add(l RawLine) {
	a.LinesRead++
	a.TotalCost += l.EffectiveCost

	if l.ResourceID == "" {
		k := unallocatedKey{AccountID: l.AccountID, Service: l.Service}
		b := a.Unalloc[k]
		if b == nil {
			b = &bucket{Currency: l.Currency}
			a.Unalloc[k] = b
		}
		b.Amount += l.EffectiveCost
		b.LineageCount++
		if b.Currency != l.Currency && l.Currency != "" {
			b.Currency = "MIXED"
		}
		return
	}

	k := resourceKey{
		AccountID:  l.AccountID,
		ResourceID: l.ResourceID,
		Region:     l.Region,
		Service:    l.Service,
	}
	b := a.Buckets[k]
	if b == nil {
		b = &bucket{Currency: l.Currency}
		a.Buckets[k] = b
	}
	b.Amount += l.EffectiveCost
	b.LineageCount++
	if b.Currency != l.Currency && l.Currency != "" {
		b.Currency = "MIXED"
	}
}

// ResourceIDsByAccount retorna o set de externalIDs distintos por
// account_id. Usado para alimentar `Resolver.ResolveURNBatch` (que
// resolve N IDs sob um (provider, account) por chamada).
func (a *Aggregator) ResourceIDsByAccount() map[string][]string {
	seen := make(map[string]map[string]struct{}, 8)
	for k := range a.Buckets {
		s := seen[k.AccountID]
		if s == nil {
			s = make(map[string]struct{}, 256)
			seen[k.AccountID] = s
		}
		s[k.ResourceID] = struct{}{}
	}
	out := make(map[string][]string, len(seen))
	for acc, set := range seen {
		ids := make([]string, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		out[acc] = ids
	}
	return out
}

// UniqueResIDs retorna o # de tuplas (account, resource_id) distintas
// — usado no Report.
func (a *Aggregator) UniqueResIDs() int {
	seen := make(map[[2]string]struct{}, len(a.Buckets))
	for k := range a.Buckets {
		seen[[2]string{k.AccountID, k.ResourceID}] = struct{}{}
	}
	return len(seen)
}

