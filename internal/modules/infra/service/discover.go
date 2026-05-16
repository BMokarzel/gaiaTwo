// Package service orquestra casos de uso do plano infra.
//
// Camada que conecta coletores (descoberta) a repositórios (persistência
// bitemporal). Nenhuma chamada externa de SDK mora aqui — apenas a
// composição entre Discoverer + Repository.
package service

import (
	"context"
	"errors"
	"fmt"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
	"costEngine/internal/repository"
)

// DiscoveryError captura uma falha pontual durante a descoberta sem
// derrubar a execução inteira (S-008). Stage identifica de onde veio
// (ex.: "compute", "persistence/ebs", "network/lb"), Err é o erro
// original encadeado.
type DiscoveryError struct {
	Stage string
	Err   error
}

func (e DiscoveryError) Error() string { return fmt.Sprintf("%s: %v", e.Stage, e.Err) }
func (e DiscoveryError) Unwrap() error { return e.Err }

// namedPersistenceDiscoverer / namedNetworkDiscoverer rotulam cada
// discoverer registrado para que a camada de relatório possa indicar
// qual fonte específica falhou em caso de erro parcial.
type namedPersistenceDiscoverer struct {
	name string
	disc collector.PersistenceDiscoverer
}

type namedNetworkDiscoverer struct {
	name string
	disc collector.NetworkDiscoverer
}

// DiscoverService orquestra a descoberta de infra para um Scope.
type DiscoverService struct {
	nodes        repository.NodeRepository
	edges        repository.EdgeRepository
	topo         collector.ScopeDiscoverer
	compute      collector.ComputeDiscoverer
	persistences []namedPersistenceDiscoverer
	networks     []namedNetworkDiscoverer
}

// NewDiscoverService cria o serviço com as dependências mínimas
// (repos + ScopeDiscoverer). Discoverers adicionais entram via setters
// (WithComputeDiscoverer, etc.) à medida que stories são implementadas.
func NewDiscoverService(nodes repository.NodeRepository, edges repository.EdgeRepository, topo collector.ScopeDiscoverer) *DiscoverService {
	return &DiscoverService{nodes: nodes, edges: edges, topo: topo}
}

// WithComputeDiscoverer registra o discoverer de Compute (S-003).
func (s *DiscoverService) WithComputeDiscoverer(c collector.ComputeDiscoverer) *DiscoverService {
	s.compute = c
	return s
}

// WithPersistenceDiscoverer registra um discoverer de Persistence com
// um rótulo curto (ex.: "ebs", "s3", "rds") usado em mensagens de erro
// parcial. Pode ser chamado múltiplas vezes; DiscoverPersistence itera
// sobre todos e agrega o relatório.
func (s *DiscoverService) WithPersistenceDiscoverer(name string, p collector.PersistenceDiscoverer) *DiscoverService {
	s.persistences = append(s.persistences, namedPersistenceDiscoverer{name: name, disc: p})
	return s
}

// WithNetworkDiscoverer registra um discoverer de Network com um rótulo
// curto (ex.: "vpc", "sg", "lb"). Pode ser chamado múltiplas vezes;
// DiscoverNetwork itera sobre todos e agrega o relatório.
func (s *DiscoverService) WithNetworkDiscoverer(name string, n collector.NetworkDiscoverer) *DiscoverService {
	s.networks = append(s.networks, namedNetworkDiscoverer{name: name, disc: n})
	return s
}

// ScopeReport sumariza o que foi escrito por EnsureScopeAncestors.
type ScopeReport struct {
	Accounts int
	Regions  int
	Zones    int
	Edges    int
}

// EnsureScopeAncestors descobre e persiste a hierarquia
// Account → Region → Zone + edges Contains para o Scope dado.
//
// É idempotente do ponto de vista do contrato (Upsert bitemporal):
// re-executar não duplica nós/arestas — fecha versão corrente e abre
// nova se algo mudou. Hash determinístico de edges garante mesmo ID.
//
// Esta operação é pré-requisito para qualquer coleta de Resource real:
// stories S-003..S-007 chamam isto primeiro antes de coletar EC2/EBS/etc.
func (s *DiscoverService) EnsureScopeAncestors(ctx context.Context, scope collector.Scope) (ScopeReport, error) {
	rep, _, err := s.EnsureScopeAncestorsWithTopology(ctx, scope)
	return rep, err
}

// EnsureScopeAncestorsWithTopology é como EnsureScopeAncestors mas também
// devolve a ScopeTopology descoberta. Útil para callers (CLI, outros
// services) que precisam continuar com descoberta de Resources e querem
// reaproveitar a hierarquia já materializada sem chamar DiscoverScope
// duas vezes.
func (s *DiscoverService) EnsureScopeAncestorsWithTopology(ctx context.Context, scope collector.Scope) (ScopeReport, collector.ScopeTopology, error) {
	topo, err := s.topo.DiscoverScope(ctx, scope)
	if err != nil {
		return ScopeReport{}, collector.ScopeTopology{}, fmt.Errorf("topology discover: %w", err)
	}

	rep := ScopeReport{}

	if err := s.nodes.Upsert(ctx, topo.Account); err != nil {
		return rep, topo, fmt.Errorf("upsert account: %w", err)
	}
	rep.Accounts++

	if err := s.nodes.Upsert(ctx, topo.Region); err != nil {
		return rep, topo, fmt.Errorf("upsert region: %w", err)
	}
	rep.Regions++

	for _, z := range topo.Zones {
		if err := s.nodes.Upsert(ctx, z); err != nil {
			return rep, topo, fmt.Errorf("upsert zone %s: %w", z.Code, err)
		}
		rep.Zones++
	}

	for _, e := range topo.Edges {
		fromKind, toKind, err := endpointKinds(e)
		if err != nil {
			return rep, topo, err
		}
		if err := s.edges.Upsert(ctx, e, fromKind, toKind); err != nil {
			return rep, topo, fmt.Errorf("upsert edge %s: %w", e.ID(), err)
		}
		rep.Edges++
	}

	return rep, topo, nil
}

// ComputeReport sumariza o que aconteceu em DiscoverCompute.
//
// New      → URN nunca antes vista (versão 1 criada).
// Updated  → URN existia mas ContentHash mudou; nova versão criada,
//            versão anterior fechada (ValidTo = now).
// Unchanged→ URN existia e ContentHash bate com a corrente; só ObservedAt
//            é atualizado (via Touch). Não há nova versão.
type ComputeReport struct {
	New       int
	Updated   int
	Unchanged int
	Edges     int
}

// DiscoverCompute descobre Compute para o Scope e aplica versionamento
// bitemporal: compara ContentHash com a versão corrente para decidir
// entre Upsert (mudança) e Touch (só ObservedAt). É a operação que
// garante o critério de aceitação de S-003: re-runs sem mudança real
// não criam novas versões.
//
// Requer que ScopeAncestors já tenha rodado (precisa de Zones para os
// edges Zone→Compute). Tipicamente o caller orquestra os dois.
func (s *DiscoverService) DiscoverCompute(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (ComputeReport, error) {
	if s.compute == nil {
		return ComputeReport{}, fmt.Errorf("DiscoverCompute: no ComputeDiscoverer registered")
	}

	batch, err := s.compute.DiscoverCompute(ctx, scope, topo)
	if err != nil {
		return ComputeReport{}, fmt.Errorf("compute discover: %w", err)
	}

	rep := ComputeReport{}
	for _, c := range batch.Computes {
		cur, err := s.nodes.GetByURN(ctx, c.URN(), repository.AsOf{})
		if err != nil {
			// Não existe ainda → versão 1.
			if errors.Is(err, repository.ErrNotFound) {
				if err := s.nodes.Upsert(ctx, c); err != nil {
					return rep, fmt.Errorf("upsert new compute %s: %w", c.URN(), err)
				}
				rep.New++
				continue
			}
			return rep, fmt.Errorf("get current %s: %w", c.URN(), err)
		}

		// Existe corrente. Compara ContentHash.
		prev, ok := cur.(node.Compute)
		if !ok {
			return rep, fmt.Errorf("urn %s is not a Compute (got %T)", c.URN(), cur)
		}
		if prev.ContentHash() == c.ContentHash() {
			// Sem mudança real: só atualiza observed_at.
			if err := s.nodes.Touch(ctx, c.URN()); err != nil {
				return rep, fmt.Errorf("touch %s: %w", c.URN(), err)
			}
			rep.Unchanged++
			continue
		}
		// Mudou: bump de versão.
		next := c
		next.NodeMeta.Version = prev.Meta().Version + 1
		if err := s.nodes.Upsert(ctx, next); err != nil {
			return rep, fmt.Errorf("upsert changed compute %s: %w", c.URN(), err)
		}
		rep.Updated++
	}

	// Edges Zone→Compute. Upsert é idempotente por ID determinístico:
	// re-run com mesmo (from,type,to,validFrom) reusa o mesmo ID e
	// o backend bitemporal trata como no-op estrutural. Cada novo dia
	// (ou mudança de validFrom) produz ID novo — comportamento desejado.
	for _, e := range batch.Edges {
		fromKind, toKind, err := endpointKinds(e)
		if err != nil {
			return rep, err
		}
		if err := s.edges.Upsert(ctx, e, fromKind, toKind); err != nil {
			return rep, fmt.Errorf("upsert edge %s: %w", e.ID(), err)
		}
		rep.Edges++
	}

	return rep, nil
}

// PersistenceReport sumariza o que aconteceu em DiscoverPersistence.
// Semântica idêntica a ComputeReport (New/Updated/Unchanged via ContentHash).
// Errors lista falhas por-discoverer (S-008): erros pontuais não derrubam
// a execução; ficam aqui para o caller reportar.
type PersistenceReport struct {
	New       int
	Updated   int
	Unchanged int
	Edges     int
	Errors    []DiscoveryError
}

// DiscoverPersistence executa cada PersistenceDiscoverer registrado e
// aplica versionamento bitemporal idêntico ao DiscoverCompute: usa
// ContentHash para decidir Upsert vs Touch. Retorna o relatório
// agregado de todos os discoverers (EBS + S3 + RDS, etc.).
//
// Resiliência (S-008): falha em um discoverer (ou em Upsert/Touch
// de um dos seus itens) é capturada em rep.Errors e a iteração segue
// para o próximo discoverer. Erros de configuração — nenhum discoverer
// registrado — continuam sendo fatais.
func (s *DiscoverService) DiscoverPersistence(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (PersistenceReport, error) {
	if len(s.persistences) == 0 {
		return PersistenceReport{}, fmt.Errorf("DiscoverPersistence: no PersistenceDiscoverer registered")
	}

	rep := PersistenceReport{}
	for _, np := range s.persistences {
		stage := "persistence/" + np.name
		batch, err := np.disc.DiscoverPersistence(ctx, scope, topo)
		if err != nil {
			rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("discover: %w", err)})
			continue
		}
		failed := false
		for _, p := range batch.Persistences {
			cur, err := s.nodes.GetByURN(ctx, p.URN(), repository.AsOf{})
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					if err := s.nodes.Upsert(ctx, p); err != nil {
						rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("upsert new %s: %w", p.URN(), err)})
						failed = true
						break
					}
					rep.New++
					continue
				}
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("get current %s: %w", p.URN(), err)})
				failed = true
				break
			}
			prev, ok := cur.(node.Persistence)
			if !ok {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("urn %s is not a Persistence (got %T)", p.URN(), cur)})
				failed = true
				break
			}
			if prev.ContentHash() == p.ContentHash() {
				if err := s.nodes.Touch(ctx, p.URN()); err != nil {
					rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("touch %s: %w", p.URN(), err)})
					failed = true
					break
				}
				rep.Unchanged++
				continue
			}
			next := p
			next.NodeMeta.Version = prev.Meta().Version + 1
			if err := s.nodes.Upsert(ctx, next); err != nil {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("upsert changed %s: %w", p.URN(), err)})
				failed = true
				break
			}
			rep.Updated++
		}
		if failed {
			continue
		}

		// Edges (Contains Zone→Persistence + AttachedTo Persistence→Compute).
		for _, e := range batch.Edges {
			fromKind, toKind, err := endpointKinds(e)
			if err != nil {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: err})
				break
			}
			if err := s.edges.Upsert(ctx, e, fromKind, toKind); err != nil {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("upsert edge %s: %w", e.ID(), err)})
				break
			}
			rep.Edges++
		}
	}
	return rep, nil
}

// NetworkReport sumariza o que aconteceu em DiscoverNetwork.
// Semântica idêntica a ComputeReport (New/Updated/Unchanged via ContentHash).
// Errors lista falhas por-discoverer (S-008).
type NetworkReport struct {
	New       int
	Updated   int
	Unchanged int
	Edges     int
	Errors    []DiscoveryError
}

// DiscoverNetwork executa cada NetworkDiscoverer registrado e aplica
// versionamento bitemporal idêntico ao DiscoverCompute/Persistence: usa
// ContentHash para decidir Upsert vs Touch. Retorna o relatório
// agregado de todos os discoverers (VPC/Subnet, SG/LB, ...).
//
// Resiliência (S-008): falha em um discoverer (ou em Upsert/Touch
// de um item) é capturada em rep.Errors; iteração segue para o próximo.
func (s *DiscoverService) DiscoverNetwork(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (NetworkReport, error) {
	if len(s.networks) == 0 {
		return NetworkReport{}, fmt.Errorf("DiscoverNetwork: no NetworkDiscoverer registered")
	}

	rep := NetworkReport{}
	for _, nd := range s.networks {
		stage := "network/" + nd.name
		batch, err := nd.disc.DiscoverNetwork(ctx, scope, topo)
		if err != nil {
			rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("discover: %w", err)})
			continue
		}
		failed := false
		for _, n := range batch.Networks {
			cur, err := s.nodes.GetByURN(ctx, n.URN(), repository.AsOf{})
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					if err := s.nodes.Upsert(ctx, n); err != nil {
						rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("upsert new %s: %w", n.URN(), err)})
						failed = true
						break
					}
					rep.New++
					continue
				}
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("get current %s: %w", n.URN(), err)})
				failed = true
				break
			}
			prev, ok := cur.(node.Network)
			if !ok {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("urn %s is not a Network (got %T)", n.URN(), cur)})
				failed = true
				break
			}
			if prev.ContentHash() == n.ContentHash() {
				if err := s.nodes.Touch(ctx, n.URN()); err != nil {
					rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("touch %s: %w", n.URN(), err)})
					failed = true
					break
				}
				rep.Unchanged++
				continue
			}
			next := n
			next.NodeMeta.Version = prev.Meta().Version + 1
			if err := s.nodes.Upsert(ctx, next); err != nil {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("upsert changed %s: %w", n.URN(), err)})
				failed = true
				break
			}
			rep.Updated++
		}
		if failed {
			continue
		}

		for _, e := range batch.Edges {
			fromKind, toKind, err := endpointKinds(e)
			if err != nil {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: err})
				break
			}
			if err := s.edges.Upsert(ctx, e, fromKind, toKind); err != nil {
				rep.Errors = append(rep.Errors, DiscoveryError{Stage: stage, Err: fmt.Errorf("upsert edge %s: %w", e.ID(), err)})
				break
			}
			rep.Edges++
		}
	}
	return rep, nil
}

// endpointKinds extrai os Kinds das URNs dos dois lados de uma aresta —
// necessário para edge.Validate(matriz de adjacência) que o repositório
// aplica em Upsert.
func endpointKinds(e edge.Edge) (node.Kind, node.Kind, error) {
	from, err := node.ParseURN(e.From())
	if err != nil {
		return "", "", fmt.Errorf("parse From URN %q: %w", e.From(), err)
	}
	to, err := node.ParseURN(e.To())
	if err != nil {
		return "", "", fmt.Errorf("parse To URN %q: %w", e.To(), err)
	}
	return from.Kind, to.Kind, nil
}
