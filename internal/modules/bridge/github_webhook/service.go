package github_webhook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Stats descreve o efeito de Ingest sobre o grafo. Usado em logs e
// resposta HTTP (auditoria leve).
type Stats struct {
	Features        int    `json:"features"`
	ResolvedServices int   `json:"resolved_services"`
	EdgesUpserted   int    `json:"edges_upserted"`
	Ignored         bool   `json:"ignored,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

// CollectorName aparece em Source.Collector das arestas inseridas.
const CollectorName = "github_webhook"

// Service orquestra HMAC-verify, parse, resolve e upsert.
//
// Não importa httpserver — exposição HTTP fica em controller.go.
type Service struct {
	nodes repository.NodeRepository
	edges repository.EdgeRepository
	now   func() time.Time
}

// New cria o Service. now=nil → time.Now.
func New(nodes repository.NodeRepository, edges repository.EdgeRepository) *Service {
	return &Service{
		nodes: nodes,
		edges: edges,
		now:   time.Now,
	}
}

// Ingest aceita um payload já parseado e materializa as arestas
// `Realizes(Feature → Service)`. Idempotente por
// `DeterministicID(feature, REALIZES, service, mergedAt)`.
//
// Validações:
//   - cada Feature URN da label deve existir e ser Kind=feature;
//     URNs inexistentes geram erro acumulado (Stats.EdgesUpserted
//     ainda reflete os pares que avançaram).
//   - paths que não resolvem para Service são silenciosamente descartados.
//   - se nenhum Service for resolvido, retorna Stats com EdgesUpserted=0
//     e sem erro (PR pode tocar só docs/, por ex.).
func (s *Service) Ingest(ctx context.Context, p *Payload) (Stats, error) {
	if p == nil {
		return Stats{}, fmt.Errorf("nil payload")
	}

	resolver, err := newPathResolver(ctx, s.nodes, p.Repo)
	if err != nil {
		return Stats{}, err
	}
	services := resolver.Resolve(p.Files)

	stats := Stats{
		Features:         len(p.FeatureURNs),
		ResolvedServices: len(services),
	}
	if len(services) == 0 {
		stats.Reason = "no services touched"
		return stats, nil
	}

	now := s.now().UTC()
	observed := now
	validFrom := p.MergedAt.UTC()

	var firstErr error
	for _, fURN := range p.FeatureURNs {
		if err := s.assertFeatureExists(ctx, fURN); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, sURN := range services {
			id := edge.DeterministicID(fURN, edge.TypeRealizes, sURN, validFrom)
			e := edge.Realizes{Base: edge.Base{
				EdgeID:   id,
				EdgeType: edge.TypeRealizes,
				FromURN:  fURN,
				ToURN:    sURN,
				EdgeMeta: edge.Meta{
					ValidFrom:   validFrom,
					ObservedAt:  observed,
					Source:      node.Source{Collector: CollectorName, Method: node.MethodImported},
					Confidence:  0.9,
					Directional: true,
					Properties: map[string]any{
						"pr_url":    p.HTMLURL,
						"merged_at": validFrom.Format(time.RFC3339),
					},
				},
			}}
			if err := s.edges.Upsert(ctx, e, node.KindFeature, node.KindService); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("upsert %s→%s: %w", fURN, sURN, err)
				}
				continue
			}
			stats.EdgesUpserted++
		}
	}
	return stats, firstErr
}

// assertFeatureExists garante que a Feature URN passada na label
// corresponde a um nó corrente do grafo. Retorna ErrFeatureNotFound
// se não houver — caller decide se é 400 (não há fila de pendentes
// no MVP, F-013 §Critérios).
func (s *Service) assertFeatureExists(ctx context.Context, urn node.URN) error {
	parts, err := node.ParseURN(urn)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrFeatureURNInvalid, urn, err)
	}
	if parts.Kind != node.KindFeature {
		return fmt.Errorf("%w: %s is %s, not feature", ErrFeatureURNInvalid, urn, parts.Kind)
	}
	n, err := s.nodes.GetByURN(ctx, urn, repository.AsOf{})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: %s", ErrFeatureNotFound, urn)
		}
		return err
	}
	if n.Kind() != node.KindFeature {
		return fmt.Errorf("%w: %s", ErrFeatureURNInvalid, urn)
	}
	return nil
}

// ErrFeatureNotFound — Feature referenciada por label inexistente no grafo.
var ErrFeatureNotFound = errors.New("github_webhook: feature not found")

// ErrFeatureURNInvalid — label `feature:<...>` não traz uma URN válida.
var ErrFeatureURNInvalid = errors.New("github_webhook: invalid feature URN")
