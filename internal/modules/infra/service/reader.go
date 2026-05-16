package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra"
	"costEngine/internal/repository"
)

// reader é a implementação de `infra.Service` (F-016 S-007). Convive
// com DiscoverService no mesmo pacote: DiscoverService faz a ingestão
// dos coletores AWS/GCP/Azure → grafo, reader faz a leitura.
//
// Métodos de leitura listam direto via `NodeRepository.List` por Kind
// e (quando há filtros AccountURN/RegionURN) filtram em memória pelos
// campos Account/Region carregados nos próprios resources. Edges
// Contains existem para introspecção do grafo, mas para o port atual
// o caminho mais direto é o próprio campo do resource.
type reader struct {
	nodes repository.NodeRepository
	edges repository.EdgeRepository
}

// NewReader constrói uma `infra.Service` operando sobre os repos
// passados. Pânica em repo nil — fail-fast em wire-up.
func NewReader(nodes repository.NodeRepository, edges repository.EdgeRepository) infra.Service {
	if nodes == nil || edges == nil {
		panic("infra/service: NewReader requires non-nil repos")
	}
	return &reader{nodes: nodes, edges: edges}
}

// ----------------------------------------------------------------------------
// Accounts
// ----------------------------------------------------------------------------

func (r *reader) ListAccounts(ctx context.Context, q infra.ListAccountsQuery) (infra.Page[infra.AccountSummary], error) {
	raw, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind:            node.KindAccount,
		Limit:           q.Limit + 1,
		Offset:          q.Offset,
		AsOf:            q.AsOf,
		IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return infra.Page[infra.AccountSummary]{}, err
	}
	// Filtra por Provider em memória (se solicitado).
	if q.Provider != "" {
		filtered := raw[:0]
		for _, n := range raw {
			a, ok := n.(node.Account)
			if !ok {
				continue
			}
			if a.ProviderID == q.Provider {
				filtered = append(filtered, n)
			}
		}
		raw = filtered
	}
	nextOffset := 0
	if len(raw) > q.Limit {
		raw = raw[:q.Limit]
		nextOffset = q.Offset + q.Limit
	}
	items := make([]infra.AccountSummary, 0, len(raw))
	for _, n := range raw {
		a, ok := n.(node.Account)
		if !ok {
			continue
		}
		rc, _ := r.countRegionsOfAccount(ctx, a.URN(), q.AsOfOptions)
		items = append(items, infra.AccountSummary{Account: a, RegionCount: rc})
	}
	return infra.Page[infra.AccountSummary]{
		Items: items, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

func (r *reader) GetAccount(ctx context.Context, urn node.URN, opts infra.AsOfOptions) (infra.AccountDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return infra.AccountDetail{}, &infra.ErrAccountNotFound{URN: urn}
		}
		return infra.AccountDetail{}, err
	}
	a, ok := n.(node.Account)
	if !ok {
		return infra.AccountDetail{}, &infra.ErrInvalidURN{URN: urn, Reason: "URN does not reference an Account"}
	}
	rc, _ := r.countRegionsOfAccount(ctx, urn, opts)
	cc, _ := r.countResourcesByAccount(ctx, urn, node.KindCompute, opts)
	pc, _ := r.countResourcesByAccount(ctx, urn, node.KindPersistence, opts)
	nc, _ := r.countResourcesByAccount(ctx, urn, node.KindNetwork, opts)
	return infra.AccountDetail{
		AccountSummary:   infra.AccountSummary{Account: a, RegionCount: rc},
		ComputeCount:     cc,
		PersistenceCount: pc,
		NetworkCount:     nc,
	}, nil
}

// ----------------------------------------------------------------------------
// Regions
// ----------------------------------------------------------------------------

func (r *reader) ListRegions(ctx context.Context, q infra.ListRegionsQuery) (infra.Page[infra.RegionSummary], error) {
	raw, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind:            node.KindRegion,
		Limit:           q.Limit + 1,
		Offset:          q.Offset,
		AsOf:            q.AsOf,
		IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return infra.Page[infra.RegionSummary]{}, err
	}
	if q.Provider != "" {
		filtered := raw[:0]
		for _, n := range raw {
			rg, ok := n.(node.Region)
			if !ok {
				continue
			}
			if rg.ProviderID == q.Provider {
				filtered = append(filtered, n)
			}
		}
		raw = filtered
	}
	nextOffset := 0
	if len(raw) > q.Limit {
		raw = raw[:q.Limit]
		nextOffset = q.Offset + q.Limit
	}
	items := make([]infra.RegionSummary, 0, len(raw))
	for _, n := range raw {
		rg, ok := n.(node.Region)
		if !ok {
			continue
		}
		items = append(items, infra.RegionSummary{Region: rg})
	}
	return infra.Page[infra.RegionSummary]{
		Items: items, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

func (r *reader) GetRegion(ctx context.Context, urn node.URN, opts infra.AsOfOptions) (infra.RegionDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return infra.RegionDetail{}, &infra.ErrRegionNotFound{URN: urn}
		}
		return infra.RegionDetail{}, err
	}
	rg, ok := n.(node.Region)
	if !ok {
		return infra.RegionDetail{}, &infra.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Region"}
	}
	cc, _ := r.countResourcesByRegion(ctx, urn, node.KindCompute, opts)
	pc, _ := r.countResourcesByRegion(ctx, urn, node.KindPersistence, opts)
	nc, _ := r.countResourcesByRegion(ctx, urn, node.KindNetwork, opts)
	return infra.RegionDetail{
		RegionSummary:    infra.RegionSummary{Region: rg},
		ComputeCount:     cc,
		PersistenceCount: pc,
		NetworkCount:     nc,
	}, nil
}

// ----------------------------------------------------------------------------
// Computes
// ----------------------------------------------------------------------------

func (r *reader) ListComputes(ctx context.Context, q infra.ListComputesQuery) (infra.Page[infra.ComputeDetail], error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: node.KindCompute, AsOf: q.AsOf, IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return infra.Page[infra.ComputeDetail]{}, err
	}
	out := make([]node.Compute, 0, len(all))
	for _, n := range all {
		c, ok := n.(node.Compute)
		if !ok {
			continue
		}
		if q.AccountURN != "" && c.Account != q.AccountURN {
			continue
		}
		if q.RegionURN != "" && c.Region != q.RegionURN {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })
	start, end, next := pageBounds(q.Offset, q.Limit, len(out))
	items := make([]infra.ComputeDetail, 0, end-start)
	for _, c := range out[start:end] {
		items = append(items, infra.ComputeDetail{Compute: c})
	}
	return infra.Page[infra.ComputeDetail]{
		Items: items, Limit: q.Limit, NextOffset: next,
	}, nil
}

func (r *reader) GetCompute(ctx context.Context, urn node.URN, opts infra.AsOfOptions) (infra.ComputeDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return infra.ComputeDetail{}, &infra.ErrComputeNotFound{URN: urn}
		}
		return infra.ComputeDetail{}, err
	}
	c, ok := n.(node.Compute)
	if !ok {
		return infra.ComputeDetail{}, &infra.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Compute"}
	}
	return infra.ComputeDetail{Compute: c}, nil
}

// ----------------------------------------------------------------------------
// Persistences
// ----------------------------------------------------------------------------

func (r *reader) ListPersistences(ctx context.Context, q infra.ListPersistencesQuery) (infra.Page[infra.PersistenceDetail], error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: node.KindPersistence, AsOf: q.AsOf, IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return infra.Page[infra.PersistenceDetail]{}, err
	}
	out := make([]node.Persistence, 0, len(all))
	for _, n := range all {
		p, ok := n.(node.Persistence)
		if !ok {
			continue
		}
		if q.AccountURN != "" && p.Account != q.AccountURN {
			continue
		}
		if q.RegionURN != "" && p.Region != q.RegionURN {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })
	start, end, next := pageBounds(q.Offset, q.Limit, len(out))
	items := make([]infra.PersistenceDetail, 0, end-start)
	for _, p := range out[start:end] {
		items = append(items, infra.PersistenceDetail{Persistence: p})
	}
	return infra.Page[infra.PersistenceDetail]{
		Items: items, Limit: q.Limit, NextOffset: next,
	}, nil
}

func (r *reader) GetPersistence(ctx context.Context, urn node.URN, opts infra.AsOfOptions) (infra.PersistenceDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return infra.PersistenceDetail{}, &infra.ErrPersistenceNotFound{URN: urn}
		}
		return infra.PersistenceDetail{}, err
	}
	p, ok := n.(node.Persistence)
	if !ok {
		return infra.PersistenceDetail{}, &infra.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Persistence"}
	}
	return infra.PersistenceDetail{Persistence: p}, nil
}

// ----------------------------------------------------------------------------
// Networks
// ----------------------------------------------------------------------------

func (r *reader) ListNetworks(ctx context.Context, q infra.ListNetworksQuery) (infra.Page[infra.NetworkDetail], error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: node.KindNetwork, AsOf: q.AsOf, IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return infra.Page[infra.NetworkDetail]{}, err
	}
	out := make([]node.Network, 0, len(all))
	for _, n := range all {
		nw, ok := n.(node.Network)
		if !ok {
			continue
		}
		if q.AccountURN != "" && nw.Account != q.AccountURN {
			continue
		}
		if q.RegionURN != "" && nw.Region != q.RegionURN {
			continue
		}
		out = append(out, nw)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })
	start, end, next := pageBounds(q.Offset, q.Limit, len(out))
	items := make([]infra.NetworkDetail, 0, end-start)
	for _, nw := range out[start:end] {
		items = append(items, infra.NetworkDetail{Network: nw})
	}
	return infra.Page[infra.NetworkDetail]{
		Items: items, Limit: q.Limit, NextOffset: next,
	}, nil
}

func (r *reader) GetNetwork(ctx context.Context, urn node.URN, opts infra.AsOfOptions) (infra.NetworkDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return infra.NetworkDetail{}, &infra.ErrNetworkNotFound{URN: urn}
		}
		return infra.NetworkDetail{}, err
	}
	nw, ok := n.(node.Network)
	if !ok {
		return infra.NetworkDetail{}, &infra.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Network"}
	}
	return infra.NetworkDetail{Network: nw}, nil
}

// ----------------------------------------------------------------------------
// Search
// ----------------------------------------------------------------------------

func (r *reader) Search(ctx context.Context, q infra.SearchQuery) (infra.SearchResults, error) {
	if strings.TrimSpace(q.Q) == "" {
		return infra.SearchResults{}, &infra.ErrInvalidURN{Reason: "missing q"}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	results, err := r.nodes.Search(ctx, repository.SearchQuery{
		Q:      q.Q,
		Kind:   q.Kind,
		Limit:  limit + 1,
		Offset: q.Offset,
	})
	if err != nil {
		return infra.SearchResults{}, err
	}
	// Filtra para Kinds do plano infra se nenhum Kind específico foi
	// requisitado — evita poluir com nós de outros planos.
	if q.Kind == "" {
		filtered := results[:0]
		for _, n := range results {
			if isInfraKind(n.Kind()) {
				filtered = append(filtered, n)
			}
		}
		results = filtered
	}
	nextOffset := 0
	if len(results) > limit {
		results = results[:limit]
		nextOffset = q.Offset + limit
	}
	return infra.SearchResults{
		Items: results, Limit: limit, NextOffset: nextOffset,
	}, nil
}

func isInfraKind(k node.Kind) bool {
	switch k {
	case node.KindProvider, node.KindAccount, node.KindRegion, node.KindZone,
		node.KindEnvironment, node.KindCompute, node.KindPersistence,
		node.KindMessaging, node.KindNetwork:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// countRegionsOfAccount conta regiões que pertencem a um Account.
// Region não carrega Account URN no struct, então a relação é via
// edges Contains. No MVP, percorre Regions e checa edge inverso.
// Para o port atual basta retornar zero quando edges não existem
// (collectors podem nem ter criado o Account ainda); a contagem é
// melhor-esforço.
func (r *reader) countRegionsOfAccount(ctx context.Context, account node.URN, opts infra.AsOfOptions) (int, error) {
	neighbors, err := r.edges.Neighbors(ctx, account, repository.DirOut, repository.EdgeFilter{
		AsOf: opts.AsOf,
	})
	if err != nil {
		return 0, err
	}
	c := 0
	for _, e := range neighbors {
		// Resolve o destino e verifica Kind=Region.
		nb, err := r.nodes.GetByURN(ctx, e.To(), opts.AsOf)
		if err != nil {
			continue
		}
		if _, ok := nb.(node.Region); ok {
			c++
		}
	}
	return c, nil
}

// countResourcesByAccount conta resources de um Kind cuja Account URN
// aponta para `account`. Lista por Kind e filtra em memória.
func (r *reader) countResourcesByAccount(ctx context.Context, account node.URN, kind node.Kind, opts infra.AsOfOptions) (int, error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: kind, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return 0, err
	}
	c := 0
	for _, n := range all {
		switch v := n.(type) {
		case node.Compute:
			if v.Account == account {
				c++
			}
		case node.Persistence:
			if v.Account == account {
				c++
			}
		case node.Network:
			if v.Account == account {
				c++
			}
		}
	}
	return c, nil
}

// countResourcesByRegion idem para Region URN.
func (r *reader) countResourcesByRegion(ctx context.Context, region node.URN, kind node.Kind, opts infra.AsOfOptions) (int, error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: kind, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return 0, err
	}
	c := 0
	for _, n := range all {
		switch v := n.(type) {
		case node.Compute:
			if v.Region == region {
				c++
			}
		case node.Persistence:
			if v.Region == region {
				c++
			}
		case node.Network:
			if v.Region == region {
				c++
			}
		}
	}
	return c, nil
}

// fetchNode busca um nó por URN com fallback opcional via History
// quando IncludeInactive=true e a versão corrente foi fechada. Mesmo
// padrão do reader do org/code (F-016 S-004).
func (r *reader) fetchNode(ctx context.Context, urn node.URN, opts infra.AsOfOptions) (node.Node, error) {
	n, err := r.nodes.GetByURN(ctx, urn, opts.AsOf)
	if err == nil {
		return n, nil
	}
	if !opts.IncludeInactive || !errors.Is(err, repository.ErrNotFound) || !opts.AsOf.IsZero() {
		return nil, err
	}
	history, herr := r.nodes.History(ctx, urn)
	if herr != nil || len(history) == 0 {
		return nil, err
	}
	return history[len(history)-1], nil
}

// pageBounds aplica offset+limit em memória sobre slice ordenada e
// devolve (start, end, nextOffset). nextOffset = 0 indica fim.
func pageBounds(offset, limit, total int) (int, int, int) {
	if offset > total {
		offset = total
	}
	end := offset + limit
	next := 0
	if end < total {
		next = end
	} else {
		end = total
	}
	return offset, end, next
}
