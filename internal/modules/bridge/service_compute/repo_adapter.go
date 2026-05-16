package service_compute

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// RepoAdapter implementa BridgeRepo sobre as interfaces canônicas
// NodeRepository + EdgeRepository (memory ou n4j). Mantém F-009
// agnóstica ao backend: testes usam memory, produção usa n4j.
type RepoAdapter struct {
	Nodes repository.NodeRepository
	Edges repository.EdgeRepository
	// Source identifica o coletor que está gravando (telemetria).
	NodeSource node.Source
	// Now é injetável; default time.Now.
	Now func() time.Time
}

// NewRepoAdapter retorna um adapter pronto. NodeSource é usado nas
// metas dos edges abertos.
func NewRepoAdapter(nodes repository.NodeRepository, edges repository.EdgeRepository, src node.Source) *RepoAdapter {
	return &RepoAdapter{Nodes: nodes, Edges: edges, NodeSource: src, Now: time.Now}
}

// ListComputeCurrent retorna os Computes correntes da conta.
func (a *RepoAdapter) ListComputeCurrent(ctx context.Context, account node.URN) ([]node.Compute, error) {
	ns, err := a.Nodes.List(ctx, repository.NodeFilter{
		Kind:       node.KindCompute,
		AccountURN: account,
	})
	if err != nil {
		return nil, fmt.Errorf("list computes: %w", err)
	}
	out := make([]node.Compute, 0, len(ns))
	for _, n := range ns {
		if c, ok := n.(node.Compute); ok {
			out = append(out, c)
			continue
		}
		if cp, ok := n.(*node.Compute); ok && cp != nil {
			out = append(out, *cp)
		}
	}
	return out, nil
}

// ListServicesByName lista Services correntes e indexa por nome.
// Nome do service = último segmento do ModulePath (`.` → o nome do
// repo, com qualquer hint do mono-repo descartado). Quando 2+ Services
// disputam o mesmo nome, o nome NÃO entra em byName e os candidatos
// vão para ambiguous (caller decide).
func (a *RepoAdapter) ListServicesByName(ctx context.Context) (map[string]node.URN, map[string][]node.URN, error) {
	ns, err := a.Nodes.List(ctx, repository.NodeFilter{Kind: node.KindService})
	if err != nil {
		return nil, nil, fmt.Errorf("list services: %w", err)
	}
	candidates := make(map[string][]node.URN, len(ns))
	for _, n := range ns {
		s, ok := asService(n)
		if !ok {
			continue
		}
		name := serviceShortName(s)
		if name == "" {
			continue
		}
		candidates[name] = append(candidates[name], s.URN())
	}
	byName := make(map[string]node.URN, len(candidates))
	ambig := map[string][]node.URN{}
	for name, urns := range candidates {
		switch len(urns) {
		case 0:
			// não acontece (criamos só com append)
		case 1:
			byName[name] = urns[0]
		default:
			ambig[name] = urns
		}
	}
	return byName, ambig, nil
}

func asService(n node.Node) (node.Service, bool) {
	if s, ok := n.(node.Service); ok {
		return s, true
	}
	if sp, ok := n.(*node.Service); ok && sp != nil {
		return *sp, true
	}
	return node.Service{}, false
}

// serviceShortName extrai o nome curto do Service. Para Services com
// ModulePath="." usa o Repo; caso contrário usa o último segmento do
// path. Casa com a convenção `Service=<short>` em tags e
// `Name=svc-<short>-...`.
func serviceShortName(s node.Service) string {
	if s.ModulePath == "" || s.ModulePath == "." {
		return strings.ToLower(strings.TrimSpace(s.Repo))
	}
	base := path.Base(s.ModulePath)
	return strings.ToLower(strings.TrimSpace(base))
}

// ListRunsOnCurrent retorna os edges RUNS_ON correntes que entram em
// Computes da conta. Faz por-Compute usando Neighbors(DirIn) — o cost
// é O(#computes) chamadas; para MVP é aceitável (volume típico <
// 10k/conta).
func (a *RepoAdapter) ListRunsOnCurrent(ctx context.Context, account node.URN) ([]ExistingEdge, error) {
	comps, err := a.ListComputeCurrent(ctx, account)
	if err != nil {
		return nil, err
	}
	out := make([]ExistingEdge, 0, len(comps))
	filter := repository.EdgeFilter{Types: []edge.Type{edge.TypeServiceRunsOn}}
	for _, c := range comps {
		es, err := a.Edges.Neighbors(ctx, c.URN(), repository.DirIn, filter)
		if err != nil {
			return nil, fmt.Errorf("neighbors %s: %w", c.URN(), err)
		}
		for _, e := range es {
			if !e.Meta().IsCurrent() {
				continue
			}
			out = append(out, ExistingEdge{
				EdgeID:     e.ID(),
				ComputeURN: e.To(),
				ServiceURN: e.From(),
				ValidFrom:  e.Meta().ValidFrom,
				Confidence: e.Meta().Confidence,
				Source:     sourceFromMeta(e.Meta()),
			})
		}
	}
	return out, nil
}

func sourceFromMeta(m edge.Meta) Source {
	if m.Properties == nil {
		return ""
	}
	if v, ok := m.Properties["source"].(string); ok {
		return Source(v)
	}
	return ""
}

// ApplyEdgeChanges aplica fecha-depois-abre: fecha primeiro para evitar
// que um Upsert idempotente confunda o edge novo com o antigo. dryRun
// true ⇒ só valida adjacency (Validate é chamado pelo Upsert real, mas
// dry-run pula a ida ao backend).
func (a *RepoAdapter) ApplyEdgeChanges(ctx context.Context, opens []EdgeOpen, closes []EdgeClose, dryRun bool) error {
	if dryRun {
		// Valida sem gravar: monta o edge e roda edge.Validate.
		for _, o := range opens {
			e := a.buildEdge(o)
			if err := edge.Validate(e, node.KindService, node.KindCompute); err != nil {
				return fmt.Errorf("validate open %s→%s: %w", o.ServiceURN, o.ComputeURN, err)
			}
		}
		return nil
	}
	for _, cl := range closes {
		if cl.EdgeID == "" {
			continue // defensivo: engine só gera EdgeID quando ExistingEdge tem id.
		}
		if err := a.Edges.Delete(ctx, cl.EdgeID); err != nil {
			return fmt.Errorf("close %s: %w", cl.EdgeID, err)
		}
	}
	for _, o := range opens {
		e := a.buildEdge(o)
		if err := a.Edges.Upsert(ctx, e, node.KindService, node.KindCompute); err != nil {
			return fmt.Errorf("open %s→%s: %w", o.ServiceURN, o.ComputeURN, err)
		}
	}
	return nil
}

// buildEdge materializa um EdgeOpen em um edge.ServiceRunsOn concreto.
// O ID é determinístico (from|type|to|validFrom) — idempotência em
// reprocessamento.
func (a *RepoAdapter) buildEdge(o EdgeOpen) edge.Edge {
	meta := edge.Meta{
		ValidFrom:   o.ValidFrom,
		ObservedAt:  o.ValidFrom,
		Source:      a.NodeSource,
		Confidence:  o.Confidence,
		Directional: true,
		Properties: map[string]any{
			"source": string(o.Source),
		},
	}
	id := edge.DeterministicID(o.ServiceURN, edge.TypeServiceRunsOn, o.ComputeURN, o.ValidFrom)
	return edge.ServiceRunsOn{
		Base: edge.Base{
			EdgeID:   id,
			EdgeType: edge.TypeServiceRunsOn,
			FromURN:  o.ServiceURN,
			ToURN:    o.ComputeURN,
			EdgeMeta: meta,
		},
	}
}
