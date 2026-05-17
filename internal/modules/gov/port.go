package gov

import (
	"context"

	"costEngine/internal/entity/node"
)

// Service é a inbound port do módulo gov. Implementada por
// `modules/gov/service.New(...)`.
//
// Reads: List* e Get* por kind. Hierarquia via `ListFeatureChildren`.
//
// Writes (F-012): Create*/Update*/Delete* por kind. Bitemporal:
// Update versiona; Delete fecha valid_to (soft). Tenant vem do ctx
// (`httpserver.TenantIDFrom`), nunca do payload.
type Service interface {
	ListCompanies(ctx context.Context, q ListQuery) (Page[node.Company], error)
	GetCompany(ctx context.Context, urn node.URN, opts AsOfOptions) (node.Company, error)
	CreateCompany(ctx context.Context, in CreateCompanyInput) (node.Company, error)
	UpdateCompany(ctx context.Context, urn node.URN, in UpdateCompanyInput) (node.Company, error)
	DeleteCompany(ctx context.Context, urn node.URN) error

	ListBusinessAreas(ctx context.Context, q ListQuery) (Page[node.BusinessArea], error)
	GetBusinessArea(ctx context.Context, urn node.URN, opts AsOfOptions) (node.BusinessArea, error)
	CreateBusinessArea(ctx context.Context, in CreateBusinessAreaInput) (node.BusinessArea, error)
	UpdateBusinessArea(ctx context.Context, urn node.URN, in UpdateBusinessAreaInput) (node.BusinessArea, error)
	DeleteBusinessArea(ctx context.Context, urn node.URN) error

	ListDomains(ctx context.Context, q ListQuery) (Page[node.Domain], error)
	GetDomain(ctx context.Context, urn node.URN, opts AsOfOptions) (node.Domain, error)
	CreateDomain(ctx context.Context, in CreateDomainInput) (node.Domain, error)
	UpdateDomain(ctx context.Context, urn node.URN, in UpdateDomainInput) (node.Domain, error)
	DeleteDomain(ctx context.Context, urn node.URN) error

	ListCapabilities(ctx context.Context, q ListQuery) (Page[node.Capability], error)
	GetCapability(ctx context.Context, urn node.URN, opts AsOfOptions) (node.Capability, error)
	CreateCapability(ctx context.Context, in CreateCapabilityInput) (node.Capability, error)
	UpdateCapability(ctx context.Context, urn node.URN, in UpdateCapabilityInput) (node.Capability, error)
	DeleteCapability(ctx context.Context, urn node.URN) error

	ListFeatures(ctx context.Context, q ListQuery) (Page[node.Feature], error)
	GetFeature(ctx context.Context, urn node.URN, opts AsOfOptions) (node.Feature, error)
	CreateFeature(ctx context.Context, in CreateFeatureInput) (node.Feature, error)
	UpdateFeature(ctx context.Context, urn node.URN, in UpdateFeatureInput) (node.Feature, error)
	DeleteFeature(ctx context.Context, urn node.URN) error

	// ListFeatureChildren devolve Features que têm `ParentFeatureURN`
	// = parent. Útil para navegar hierarquia de features aninhadas.
	ListFeatureChildren(ctx context.Context, parent node.URN, q ListQuery) (Page[node.Feature], error)
}
