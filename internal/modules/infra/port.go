// Package infra é o módulo do **infra plane** (F-009).
//
// Este arquivo declara a inbound port (infra.Service) — a única
// interface que controllers e features cross-plane consomem. A
// implementação concreta vive em `internal/modules/infra/service/`
// junto do DiscoverService (ingest dos collectors).
//
// Princípios (F-016 ADR-005):
//   - Uma Service por módulo (Bounded Context = Module).
//   - Entidades são structs do `entity/node` (Account, Region, Compute,
//     Persistence, Network) reusados em projeções (Detail/Summary).
//   - Service não conhece HTTP: retorna `infra.ErrXxx` tipados.
package infra

import (
	"context"

	"costEngine/internal/entity/node"
)

// Service é a inbound port do módulo infra. Implementada por
// `modules/infra/service.NewReader(...)`.
//
// Lista as Kinds-folha (Compute/Persistence/Network) com filtros
// opcionais por Account/Region. Lista também os nós-escopo
// (Account/Region) para navegação. Edges entre eles (Contains) são
// usados internamente para counts, mas o port expõe filtros por URN
// dos próprios resources (que carregam Account/Region nos campos).
type Service interface {
	ListAccounts(ctx context.Context, q ListAccountsQuery) (Page[AccountSummary], error)
	GetAccount(ctx context.Context, urn node.URN, opts AsOfOptions) (AccountDetail, error)

	ListRegions(ctx context.Context, q ListRegionsQuery) (Page[RegionSummary], error)
	GetRegion(ctx context.Context, urn node.URN, opts AsOfOptions) (RegionDetail, error)

	ListComputes(ctx context.Context, q ListComputesQuery) (Page[ComputeDetail], error)
	GetCompute(ctx context.Context, urn node.URN, opts AsOfOptions) (ComputeDetail, error)

	ListPersistences(ctx context.Context, q ListPersistencesQuery) (Page[PersistenceDetail], error)
	GetPersistence(ctx context.Context, urn node.URN, opts AsOfOptions) (PersistenceDetail, error)

	ListNetworks(ctx context.Context, q ListNetworksQuery) (Page[NetworkDetail], error)
	GetNetwork(ctx context.Context, urn node.URN, opts AsOfOptions) (NetworkDetail, error)

	Search(ctx context.Context, q SearchQuery) (SearchResults, error)
}
