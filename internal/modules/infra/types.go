package infra

import (
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Page é o envelope genérico paginado (mesmo shape de org/code).
type Page[T any] struct {
	Items      []T
	Limit      int
	NextOffset int
}

// AsOfOptions agrupa as opções bitemporais reusadas em todos os reads.
type AsOfOptions struct {
	AsOf            repository.AsOf
	IncludeInactive bool
}

// ListAccountsQuery parametriza ListAccounts. Provider filtra por
// ProviderID (aws/gcp/azure); vazio = todos.
type ListAccountsQuery struct {
	AsOfOptions
	Provider node.ProviderID
	Limit    int
	Offset   int
}

// ListRegionsQuery parametriza ListRegions. Provider filtra como acima.
type ListRegionsQuery struct {
	AsOfOptions
	Provider node.ProviderID
	Limit    int
	Offset   int
}

// ListComputesQuery parametriza ListComputes. AccountURN/RegionURN são
// filtros opcionais (vazio = sem filtro daquele lado). Casa o uso
// típico de console: "computes desta conta", "computes desta região".
type ListComputesQuery struct {
	AsOfOptions
	AccountURN node.URN
	RegionURN  node.URN
	Limit      int
	Offset     int
}

// ListPersistencesQuery — idem ListComputesQuery para Persistence.
type ListPersistencesQuery struct {
	AsOfOptions
	AccountURN node.URN
	RegionURN  node.URN
	Limit      int
	Offset     int
}

// ListNetworksQuery — idem para Network.
type ListNetworksQuery struct {
	AsOfOptions
	AccountURN node.URN
	RegionURN  node.URN
	Limit      int
	Offset     int
}

// SearchQuery parametriza Search (usado por app/search via outbound port).
type SearchQuery struct {
	Q      string
	Kind   node.Kind // se vazio, todos os Kinds do plano infra
	Limit  int
	Offset int
}

// AccountSummary projeção de Account para listagens. Counts são
// derivados pelo reader (regiões/resources cuja Account URN aponta
// para este).
type AccountSummary struct {
	Account     node.Account
	RegionCount int
}

// AccountDetail estende AccountSummary com counts de resources.
type AccountDetail struct {
	AccountSummary
	ComputeCount     int
	PersistenceCount int
	NetworkCount     int
}

// RegionSummary projeção de Region para listagens.
type RegionSummary struct {
	Region node.Region
}

// RegionDetail estende RegionSummary com counts de resources.
type RegionDetail struct {
	RegionSummary
	ComputeCount     int
	PersistenceCount int
	NetworkCount     int
}

// ComputeDetail projeção de Compute. AccountURN/RegionURN já vivem
// no struct (campos Account/Region).
type ComputeDetail struct {
	Compute node.Compute
}

// PersistenceDetail projeção de Persistence.
type PersistenceDetail struct {
	Persistence node.Persistence
}

// NetworkDetail projeção de Network.
type NetworkDetail struct {
	Network node.Network
}

// SearchResults é o retorno paginado de Search.
type SearchResults struct {
	Items      []node.Node
	Limit      int
	NextOffset int
}
