package gov

import (
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Page é a página genérica de resultados. Espelha `org.Page` para
// consistência cross-module. NextOffset = 0 indica fim.
type Page[T any] struct {
	Items      []T
	Limit      int
	NextOffset int
}

// AsOfOptions agrupa opções bitemporais reusadas em todos os reads.
type AsOfOptions struct {
	AsOf            repository.AsOf
	IncludeInactive bool
}

// ListQuery parametriza todas as listagens do módulo. Padrão list+offset
// (consistente com org.ListTeamsQuery).
type ListQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// ---------- Create inputs ----------
//
// `Tenant` é sempre tomado do contexto da request (TenantIDFrom), nunca
// do payload — multi-tenancy é seam de plataforma, não input de usuário.

type CreateCompanyInput struct {
	ShortID string
	Name    string
	Domain  string // opcional
}

type CreateBusinessAreaInput struct {
	ShortID    string
	Name       string
	CompanyURN node.URN // pai obrigatório
}

type CreateDomainInput struct {
	ShortID       string
	Name          string
	Description   string
	ParentAreaURN node.URN // BusinessArea obrigatório
}

type CreateCapabilityInput struct {
	ShortID         string
	Name            string
	Description     string
	ParentDomainURN node.URN // Domain obrigatório
}

type CreateFeatureInput struct {
	ShortID          string
	Name             string
	Description      string
	ParentCapURN     node.URN // Capability obrigatório (raiz da árvore)
	ParentFeatureURN node.URN // opcional (sub-feature)
	Status           string   // opcional
	SLA              string   // opcional
	Priority         string   // opcional
}

// ---------- Update inputs ----------
//
// Ponteiro = "campo enviado"; nil = "não tocar" (PATCH semântico).
// Strings vazias substituem; para limpar campo opcional, mande "".

type UpdateCompanyInput struct {
	Name   *string
	Domain *string
}

type UpdateBusinessAreaInput struct {
	Name *string
}

type UpdateDomainInput struct {
	Name        *string
	Description *string
}

type UpdateCapabilityInput struct {
	Name        *string
	Description *string
}

type UpdateFeatureInput struct {
	Name        *string
	Description *string
	Status      *string
	SLA         *string
	Priority    *string
}
