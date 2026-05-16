// Package code é o módulo do **code plane** (F-007).
//
// Este arquivo declara a inbound port (code.Service) — a única
// interface que controllers e features cross-plane consomem. A
// implementação concreta vive em `internal/modules/code/service/`
// junto do Writer (ingest do golang collector).
//
// Princípios (F-016 ADR-005):
//   - Uma Service por módulo (Bounded Context = Module).
//   - Entidades são structs do `entity/node` (Service, Endpoint,
//     Function) reusados em projeções (Detail/Summary).
//   - Service não conhece HTTP: retorna `code.ErrXxx` tipados.
package code

import (
	"context"

	"costEngine/internal/entity/node"
)

// Service é a inbound port do módulo code. Implementada por
// `modules/code/service.New(...)`.
//
// Métodos:
//   - GetService/Endpoint/Function: lookup por URN com checagem de Kind.
//   - ListServices: paginado, scope global do plano code.
//   - ListEndpointsOfService / ListFunctionsOfService: 1-hop via campo
//     ServiceURN dos próprios nós (Endpoint/Function carregam o pai).
//   - Search: usado pelo app/search outbound adapter (S-009).
type Service interface {
	ListServices(ctx context.Context, q ListServicesQuery) (Page[ServiceSummary], error)

	GetService(ctx context.Context, urn node.URN, opts AsOfOptions) (ServiceDetail, error)

	ListEndpointsOfService(ctx context.Context, svc node.URN, q ListEndpointsQuery) (Page[EndpointDetail], error)

	GetEndpoint(ctx context.Context, urn node.URN, opts AsOfOptions) (EndpointDetail, error)

	ListFunctionsOfService(ctx context.Context, svc node.URN, q ListFunctionsQuery) (Page[FunctionDetail], error)

	GetFunction(ctx context.Context, urn node.URN, opts AsOfOptions) (FunctionDetail, error)

	Search(ctx context.Context, q SearchQuery) (SearchResults, error)
}
