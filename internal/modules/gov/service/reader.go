// Package service implementa `gov.Service` (F-024 read + F-012 write).
//
// O nome lowercase `service` segue a convenção do plano F-016: a `New`
// retorna `gov.Service`, não `*service`. Reads em reader.go; writes
// (Create/Update/Delete) em writer.go.
package service

import (
	"context"
	"errors"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/gov"
	"costEngine/internal/repository"
)

type service struct {
	nodes repository.NodeRepository
	edges repository.EdgeRepository
	now   func() time.Time // injetável para testes determinísticos
}

// New constrói uma `gov.Service` operando sobre os repos passados.
// Panica em repo nil — fail-fast em wire-up. `edges` pode ser nil
// quando só reads forem usados, mas writes panicam ao tentar emitir
// CONTAINS — preferimos o panic explícito ao deferred nil-deref.
func New(nodes repository.NodeRepository, edges repository.EdgeRepository) gov.Service {
	if nodes == nil {
		panic("gov/service: New requires non-nil NodeRepository")
	}
	return &service{nodes: nodes, edges: edges, now: time.Now}
}

// ---------- helpers genéricos ----------

// listKind lista nós de um kind com paginação limit+1.
func (r *service) listKind(ctx context.Context, kind node.Kind, q gov.ListQuery) ([]node.Node, int, error) {
	raw, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind:            kind,
		Limit:           q.Limit + 1,
		Offset:          q.Offset,
		AsOf:            q.AsOf,
		IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return nil, 0, err
	}
	nextOffset := 0
	if len(raw) > q.Limit {
		raw = raw[:q.Limit]
		nextOffset = q.Offset + q.Limit
	}
	return raw, nextOffset, nil
}

// fetchKind devolve o nó esperado pelo Kind. Mapeia repository.ErrNotFound
// para gov.ErrNotFound; type-mismatch para gov.ErrInvalidURN.
func (r *service) fetchKind(ctx context.Context, urn node.URN, kind node.Kind, opts gov.AsOfOptions) (node.Node, error) {
	n, err := r.nodes.GetByURN(ctx, urn, opts.AsOf)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, &gov.ErrNotFound{URN: urn, Kind: kind}
		}
		return nil, err
	}
	if n.Kind() != kind {
		return nil, &gov.ErrInvalidURN{
			URN: urn, Expected: kind,
			Reason: "URN references kind " + string(n.Kind()),
		}
	}
	return n, nil
}

// ---------- Company ----------

func (r *service) ListCompanies(ctx context.Context, q gov.ListQuery) (gov.Page[node.Company], error) {
	raw, next, err := r.listKind(ctx, node.KindCompany, q)
	if err != nil {
		return gov.Page[node.Company]{}, err
	}
	items := make([]node.Company, 0, len(raw))
	for _, n := range raw {
		if c, ok := n.(node.Company); ok {
			items = append(items, c)
		}
	}
	return gov.Page[node.Company]{Items: items, Limit: q.Limit, NextOffset: next}, nil
}

func (r *service) GetCompany(ctx context.Context, urn node.URN, opts gov.AsOfOptions) (node.Company, error) {
	n, err := r.fetchKind(ctx, urn, node.KindCompany, opts)
	if err != nil {
		return node.Company{}, err
	}
	return n.(node.Company), nil
}

// ---------- BusinessArea ----------

func (r *service) ListBusinessAreas(ctx context.Context, q gov.ListQuery) (gov.Page[node.BusinessArea], error) {
	raw, next, err := r.listKind(ctx, node.KindBusinessArea, q)
	if err != nil {
		return gov.Page[node.BusinessArea]{}, err
	}
	items := make([]node.BusinessArea, 0, len(raw))
	for _, n := range raw {
		if c, ok := n.(node.BusinessArea); ok {
			items = append(items, c)
		}
	}
	return gov.Page[node.BusinessArea]{Items: items, Limit: q.Limit, NextOffset: next}, nil
}

func (r *service) GetBusinessArea(ctx context.Context, urn node.URN, opts gov.AsOfOptions) (node.BusinessArea, error) {
	n, err := r.fetchKind(ctx, urn, node.KindBusinessArea, opts)
	if err != nil {
		return node.BusinessArea{}, err
	}
	return n.(node.BusinessArea), nil
}

// ---------- Domain ----------

func (r *service) ListDomains(ctx context.Context, q gov.ListQuery) (gov.Page[node.Domain], error) {
	raw, next, err := r.listKind(ctx, node.KindDomain, q)
	if err != nil {
		return gov.Page[node.Domain]{}, err
	}
	items := make([]node.Domain, 0, len(raw))
	for _, n := range raw {
		if c, ok := n.(node.Domain); ok {
			items = append(items, c)
		}
	}
	return gov.Page[node.Domain]{Items: items, Limit: q.Limit, NextOffset: next}, nil
}

func (r *service) GetDomain(ctx context.Context, urn node.URN, opts gov.AsOfOptions) (node.Domain, error) {
	n, err := r.fetchKind(ctx, urn, node.KindDomain, opts)
	if err != nil {
		return node.Domain{}, err
	}
	return n.(node.Domain), nil
}

// ---------- Capability ----------

func (r *service) ListCapabilities(ctx context.Context, q gov.ListQuery) (gov.Page[node.Capability], error) {
	raw, next, err := r.listKind(ctx, node.KindCapability, q)
	if err != nil {
		return gov.Page[node.Capability]{}, err
	}
	items := make([]node.Capability, 0, len(raw))
	for _, n := range raw {
		if c, ok := n.(node.Capability); ok {
			items = append(items, c)
		}
	}
	return gov.Page[node.Capability]{Items: items, Limit: q.Limit, NextOffset: next}, nil
}

func (r *service) GetCapability(ctx context.Context, urn node.URN, opts gov.AsOfOptions) (node.Capability, error) {
	n, err := r.fetchKind(ctx, urn, node.KindCapability, opts)
	if err != nil {
		return node.Capability{}, err
	}
	return n.(node.Capability), nil
}

// ---------- Feature ----------

func (r *service) ListFeatures(ctx context.Context, q gov.ListQuery) (gov.Page[node.Feature], error) {
	raw, next, err := r.listKind(ctx, node.KindFeature, q)
	if err != nil {
		return gov.Page[node.Feature]{}, err
	}
	items := make([]node.Feature, 0, len(raw))
	for _, n := range raw {
		if c, ok := n.(node.Feature); ok {
			items = append(items, c)
		}
	}
	return gov.Page[node.Feature]{Items: items, Limit: q.Limit, NextOffset: next}, nil
}

func (r *service) GetFeature(ctx context.Context, urn node.URN, opts gov.AsOfOptions) (node.Feature, error) {
	n, err := r.fetchKind(ctx, urn, node.KindFeature, opts)
	if err != nil {
		return node.Feature{}, err
	}
	return n.(node.Feature), nil
}

// ListFeatureChildren: filtra Features cujo ParentFeatureURN == parent.
//
// Implementação MVP: pega todas as Features e filtra in-memory.
// Tradeoff: simples, sem traversal de edges (CONTAINS Feature→Feature
// é derivável do atributo). Em catálogos grandes, considerar uma query
// dedicada no NodeRepository — backlog.
func (r *service) ListFeatureChildren(ctx context.Context, parent node.URN, q gov.ListQuery) (gov.Page[node.Feature], error) {
	// Confirma que o parent existe e é Feature.
	if _, err := r.fetchKind(ctx, parent, node.KindFeature, q.AsOfOptions); err != nil {
		return gov.Page[node.Feature]{}, err
	}
	raw, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind:            node.KindFeature,
		AsOf:            q.AsOf,
		IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return gov.Page[node.Feature]{}, err
	}
	var filtered []node.Feature
	for _, n := range raw {
		if f, ok := n.(node.Feature); ok && f.ParentFeatureURN == parent {
			filtered = append(filtered, f)
		}
	}
	// Aplica offset+limit pós-filtro (MVP).
	start := q.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + q.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := filtered[start:end]
	nextOffset := 0
	if end < len(filtered) {
		nextOffset = end
	}
	return gov.Page[node.Feature]{Items: page, Limit: q.Limit, NextOffset: nextOffset}, nil
}
