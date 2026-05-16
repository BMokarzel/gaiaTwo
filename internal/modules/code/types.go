package code

import (
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Page é o envelope genérico paginado (mesmo formato do módulo org —
// não compartilhamos por enquanto para preservar autonomia entre
// módulos, mas o shape é estável e barato de manter idêntico).
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

// ListServicesQuery parametriza ListServices.
type ListServicesQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// ListEndpointsQuery parametriza ListEndpointsOfService.
type ListEndpointsQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// ListFunctionsQuery parametriza ListFunctionsOfService.
type ListFunctionsQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// SearchQuery parametriza Search (usado por app/search via outbound port).
type SearchQuery struct {
	Q      string
	Kind   node.Kind // se vazio, todos os Kinds do plano code
	Limit  int
	Offset int
}

// ServiceSummary é a projeção de Service para listagens. Counts são
// derivados pelo reader (endpoints/functions cujo ServiceURN aponta
// para este).
type ServiceSummary struct {
	Service       node.Service
	EndpointCount int
	FunctionCount int
}

// ServiceDetail estende ServiceSummary; mantido para simetria com o
// padrão dos outros módulos (TeamDetail/SquadDetail).
type ServiceDetail struct {
	ServiceSummary
}

// EndpointDetail é a projeção de Endpoint. ServiceURN já vive no struct.
type EndpointDetail struct {
	Endpoint node.Endpoint
}

// FunctionDetail é a projeção de Function.
type FunctionDetail struct {
	Function node.Function
}

// SearchResults é o retorno paginado de Search.
type SearchResults struct {
	Items      []node.Node
	Limit      int
	NextOffset int
}
