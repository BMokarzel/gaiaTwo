package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/code"
	"costEngine/internal/repository"
)

// reader é a implementação de `code.Service` (F-016 S-006). Convive com
// Writer no mesmo pacote: Writer faz a ingestão (golang collector →
// grafo), reader faz a leitura.
//
// Métodos de leitura listam direto via `NodeRepository.List` por Kind
// e (quando necessário) filtram em memória pelo `ServiceURN` carregado
// no struct concreto. Edges DEFINED_IN existem para introspecção do
// grafo (S-008), mas para o port atual o caminho mais direto é o
// próprio campo `ServiceURN`.
type reader struct {
	nodes repository.NodeRepository
	edges repository.EdgeRepository
}

// New constrói uma `code.Service` operando sobre os repos passados.
// Pánica em repo nil — fail-fast em wire-up.
func New(nodes repository.NodeRepository, edges repository.EdgeRepository) code.Service {
	if nodes == nil || edges == nil {
		panic("code/service: New requires non-nil repos")
	}
	return &reader{nodes: nodes, edges: edges}
}

// ----------------------------------------------------------------------------
// Services
// ----------------------------------------------------------------------------

func (r *reader) ListServices(ctx context.Context, q code.ListServicesQuery) (code.Page[code.ServiceSummary], error) {
	raw, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind:            node.KindService,
		Limit:           q.Limit + 1,
		Offset:          q.Offset,
		AsOf:            q.AsOf,
		IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return code.Page[code.ServiceSummary]{}, err
	}
	nextOffset := 0
	if len(raw) > q.Limit {
		raw = raw[:q.Limit]
		nextOffset = q.Offset + q.Limit
	}
	items := make([]code.ServiceSummary, 0, len(raw))
	for _, n := range raw {
		s, ok := n.(node.Service)
		if !ok {
			continue
		}
		ec, _ := r.countByServiceURN(ctx, s.URN(), node.KindEndpoint, q.AsOfOptions)
		fc, _ := r.countByServiceURN(ctx, s.URN(), node.KindFunction, q.AsOfOptions)
		items = append(items, code.ServiceSummary{
			Service: s, EndpointCount: ec, FunctionCount: fc,
		})
	}
	return code.Page[code.ServiceSummary]{
		Items: items, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

func (r *reader) GetService(ctx context.Context, urn node.URN, opts code.AsOfOptions) (code.ServiceDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return code.ServiceDetail{}, &code.ErrServiceNotFound{URN: urn}
		}
		return code.ServiceDetail{}, err
	}
	s, ok := n.(node.Service)
	if !ok {
		return code.ServiceDetail{}, &code.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Service"}
	}
	ec, err := r.countByServiceURN(ctx, urn, node.KindEndpoint, opts)
	if err != nil {
		return code.ServiceDetail{}, err
	}
	fc, err := r.countByServiceURN(ctx, urn, node.KindFunction, opts)
	if err != nil {
		return code.ServiceDetail{}, err
	}
	return code.ServiceDetail{
		ServiceSummary: code.ServiceSummary{
			Service: s, EndpointCount: ec, FunctionCount: fc,
		},
	}, nil
}

// ----------------------------------------------------------------------------
// Endpoints
// ----------------------------------------------------------------------------

func (r *reader) ListEndpointsOfService(ctx context.Context, svc node.URN, q code.ListEndpointsQuery) (code.Page[code.EndpointDetail], error) {
	if _, err := r.fetchNode(ctx, svc, q.AsOfOptions); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return code.Page[code.EndpointDetail]{}, &code.ErrServiceNotFound{URN: svc}
		}
		return code.Page[code.EndpointDetail]{}, err
	}
	endpoints, err := r.endpointsOfService(ctx, svc, q.AsOfOptions)
	if err != nil {
		return code.Page[code.EndpointDetail]{}, err
	}
	start, end, next := pageBounds(q.Offset, q.Limit, len(endpoints))
	items := make([]code.EndpointDetail, 0, end-start)
	for _, ep := range endpoints[start:end] {
		items = append(items, code.EndpointDetail{Endpoint: ep})
	}
	return code.Page[code.EndpointDetail]{
		Items: items, Limit: q.Limit, NextOffset: next,
	}, nil
}

func (r *reader) GetEndpoint(ctx context.Context, urn node.URN, opts code.AsOfOptions) (code.EndpointDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return code.EndpointDetail{}, &code.ErrEndpointNotFound{URN: urn}
		}
		return code.EndpointDetail{}, err
	}
	ep, ok := n.(node.Endpoint)
	if !ok {
		return code.EndpointDetail{}, &code.ErrInvalidURN{URN: urn, Reason: "URN does not reference an Endpoint"}
	}
	return code.EndpointDetail{Endpoint: ep}, nil
}

// ----------------------------------------------------------------------------
// Functions
// ----------------------------------------------------------------------------

func (r *reader) ListFunctionsOfService(ctx context.Context, svc node.URN, q code.ListFunctionsQuery) (code.Page[code.FunctionDetail], error) {
	if _, err := r.fetchNode(ctx, svc, q.AsOfOptions); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return code.Page[code.FunctionDetail]{}, &code.ErrServiceNotFound{URN: svc}
		}
		return code.Page[code.FunctionDetail]{}, err
	}
	funcs, err := r.functionsOfService(ctx, svc, q.AsOfOptions)
	if err != nil {
		return code.Page[code.FunctionDetail]{}, err
	}
	start, end, next := pageBounds(q.Offset, q.Limit, len(funcs))
	items := make([]code.FunctionDetail, 0, end-start)
	for _, fn := range funcs[start:end] {
		items = append(items, code.FunctionDetail{Function: fn})
	}
	return code.Page[code.FunctionDetail]{
		Items: items, Limit: q.Limit, NextOffset: next,
	}, nil
}

func (r *reader) GetFunction(ctx context.Context, urn node.URN, opts code.AsOfOptions) (code.FunctionDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return code.FunctionDetail{}, &code.ErrFunctionNotFound{URN: urn}
		}
		return code.FunctionDetail{}, err
	}
	fn, ok := n.(node.Function)
	if !ok {
		return code.FunctionDetail{}, &code.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Function"}
	}
	return code.FunctionDetail{Function: fn}, nil
}

// ----------------------------------------------------------------------------
// Search
// ----------------------------------------------------------------------------

func (r *reader) Search(ctx context.Context, q code.SearchQuery) (code.SearchResults, error) {
	if strings.TrimSpace(q.Q) == "" {
		return code.SearchResults{}, &code.ErrInvalidURN{Reason: "missing q"}
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
		return code.SearchResults{}, err
	}
	// Filtra para Kinds do plano code se nenhum Kind específico foi
	// requisitado — evita poluir o resultado com nós de outros planos.
	if q.Kind == "" {
		filtered := results[:0]
		for _, n := range results {
			switch n.Kind() {
			case node.KindService, node.KindEndpoint, node.KindFunction:
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
	return code.SearchResults{
		Items: results, Limit: limit, NextOffset: nextOffset,
	}, nil
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// endpointsOfService lista todos os Endpoints (corrente ou AsOf) cujo
// ServiceURN aponta para `svc`. Ordenado por URN crescente para
// determinismo. Implementação MVP: List por Kind + filtro em memória.
func (r *reader) endpointsOfService(ctx context.Context, svc node.URN, opts code.AsOfOptions) ([]node.Endpoint, error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: node.KindEndpoint, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return nil, err
	}
	out := make([]node.Endpoint, 0, len(all))
	for _, n := range all {
		ep, ok := n.(node.Endpoint)
		if !ok || ep.ServiceURN != svc {
			continue
		}
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })
	return out, nil
}

func (r *reader) functionsOfService(ctx context.Context, svc node.URN, opts code.AsOfOptions) ([]node.Function, error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: node.KindFunction, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return nil, err
	}
	out := make([]node.Function, 0, len(all))
	for _, n := range all {
		fn, ok := n.(node.Function)
		if !ok || fn.ServiceURN != svc {
			continue
		}
		out = append(out, fn)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })
	return out, nil
}

// countByServiceURN conta Endpoints/Functions vinculados a um service.
// Helper conveniente — para listagens grandes vale denormalizar (TODO),
// mas no MVP a cardinalidade é pequena.
func (r *reader) countByServiceURN(ctx context.Context, svc node.URN, kind node.Kind, opts code.AsOfOptions) (int, error) {
	all, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind: kind, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return 0, err
	}
	c := 0
	for _, n := range all {
		switch v := n.(type) {
		case node.Endpoint:
			if v.ServiceURN == svc {
				c++
			}
		case node.Function:
			if v.ServiceURN == svc {
				c++
			}
		}
	}
	return c, nil
}

// fetchNode busca um nó por URN com fallback opcional via History
// quando IncludeInactive=true e a versão corrente foi fechada. Mesmo
// padrão do reader do org (F-016 S-004).
func (r *reader) fetchNode(ctx context.Context, urn node.URN, opts code.AsOfOptions) (node.Node, error) {
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
