package node

import (
	"crypto/sha256"
	"encoding/hex"
)

// =============================================================================
// Governance plane (E-008) — eixos product-arch, delivery, audience.
// URN: urn:ce:gov:<company>:<kind>/<short-id>
// =============================================================================

// Company representa a entidade jurídica raiz no governance plane.
// O slot `<account>` da URN reusa o `<company>` (tenant).
//
// URN: urn:ce:gov:<company>:company/<short-id>
type Company struct {
	Base
	Tenant   string `json:"tenant"`
	ShortID  string `json:"short_id"`
	Name     string `json:"name"`
	Domain   string `json:"domain,omitempty"` // domínio web canônico (opcional)
}

func NewCompanyURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindCompany, shortID)
}

func (c Company) ContentHash() string {
	return hexSum(c.ShortID + "|" + c.Name + "|" + c.Domain)
}

// BusinessArea é a divisão de alto nível dentro de uma Company
// (ex.: "Marketplace", "Logistics"). Aresta CONTAINS organiza Times
// (eixo org) e Domains (eixo product-arch).
//
// URN: urn:ce:gov:<company>:business_area/<short-id>
type BusinessArea struct {
	Base
	Tenant     string `json:"tenant"`
	ShortID    string `json:"short_id"`
	Name       string `json:"name"`
	CompanyURN URN    `json:"company_urn,omitempty"`
}

func NewBusinessAreaURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindBusinessArea, shortID)
}

func (b BusinessArea) ContentHash() string {
	return hexSum(b.ShortID + "|" + b.Name + "|" + string(b.CompanyURN))
}

// Domain é um bloco de negócio coeso (DDD-ish) dentro de uma
// BusinessArea (F-024). Agrupa Capabilities.
//
// URN: urn:ce:gov:<company>:domain/<short-id>
type Domain struct {
	Base
	Tenant         string `json:"tenant"`
	ShortID        string `json:"short_id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ParentAreaURN  URN    `json:"parent_area_urn,omitempty"`
}

func NewDomainURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindDomain, shortID)
}

func (d Domain) ContentHash() string {
	return hexSum(d.ShortID + "|" + d.Name + "|" + string(d.ParentAreaURN))
}

// Capability representa uma capacidade do negócio dentro de um Domain
// (F-024). Não tem código direto — é categorização. Features dela
// herdam.
//
// URN: urn:ce:gov:<company>:capability/<short-id>
type Capability struct {
	Base
	Tenant          string `json:"tenant"`
	ShortID         string `json:"short_id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	ParentDomainURN URN    `json:"parent_domain_urn,omitempty"`
}

func NewCapabilityURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindCapability, shortID)
}

func (c Capability) ContentHash() string {
	return hexSum(c.ShortID + "|" + c.Name + "|" + string(c.ParentDomainURN))
}

// Feature é a unidade entregável de uma Capability. Código referencia
// Feature via `feature_tags` (denormalização — ADR-009). Suporta
// aninhamento: Feature -CONTAINS-> Feature.
//
// URN: urn:ce:gov:<company>:feature/<short-id>
type Feature struct {
	Base
	Tenant          string `json:"tenant"`
	ShortID         string `json:"short_id"` // kebab-case único por company (ADR-009)
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	ParentCapURN    URN    `json:"parent_capability_urn,omitempty"`
	ParentFeatureURN URN   `json:"parent_feature_urn,omitempty"`
	Status          string `json:"status,omitempty"` // proposed|active|deprecated|sunset
	SLA             string `json:"sla,omitempty"`
	Priority        string `json:"priority,omitempty"`
}

func NewFeatureURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindFeature, shortID)
}

func (f Feature) ContentHash() string {
	return hexSum(f.ShortID + "|" + f.Name + "|" + string(f.ParentCapURN) +
		"|" + string(f.ParentFeatureURN) + "|" + f.Status + "|" + f.SLA + "|" + f.Priority)
}

// Epic é um agrupador de delivery (F-025). Contém UserStories. Tem
// motivação de negócio, métricas, prazo.
//
// URN: urn:ce:gov:<company>:epic/<short-id>
type Epic struct {
	Base
	Tenant         string `json:"tenant"`
	ShortID        string `json:"short_id"`
	Name           string `json:"name"`
	BusinessGoal   string `json:"business_goal,omitempty"`
	SuccessMetrics string `json:"success_metrics,omitempty"`
	TargetDate     string `json:"target_date,omitempty"` // ISO 8601
	Status         string `json:"status,omitempty"`      // draft|active|done|cancelled
}

func NewEpicURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindEpic, shortID)
}

func (e Epic) ContentHash() string {
	return hexSum(e.ShortID + "|" + e.Name + "|" + e.BusinessGoal +
		"|" + e.SuccessMetrics + "|" + e.TargetDate + "|" + e.Status)
}

// UserStory é a unidade de delivery (F-025). Assinada a uma Persona
// (via SERVES) e a uma Person (ASSIGNED_TO). Conectada a Feature via
// DELIVERS.
//
// URN: urn:ce:gov:<company>:user_story/<short-id>
type UserStory struct {
	Base
	Tenant              string `json:"tenant"`
	ShortID             string `json:"short_id"`
	Title               string `json:"title"`
	Description         string `json:"description,omitempty"`
	AcceptanceCriteria  string `json:"acceptance_criteria,omitempty"`
	Status              string `json:"status,omitempty"`       // backlog|in-progress|done
	StoryPoints         int    `json:"story_points,omitempty"`
	ParentEpicURN       URN    `json:"parent_epic_urn,omitempty"`
}

func NewUserStoryURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindUserStory, shortID)
}

func (u UserStory) ContentHash() string {
	return hexSum(u.ShortID + "|" + u.Title + "|" + u.AcceptanceCriteria +
		"|" + u.Status + "|" + string(u.ParentEpicURN))
}

// Persona é um perfil de usuário (F-026). Conectada a UserStories
// via SERVES. NÃO é denormalizada em código — disciplina deliberada
// (ADR-009).
//
// URN: urn:ce:gov:<company>:persona/<short-id>
type Persona struct {
	Base
	Tenant      string `json:"tenant"`
	ShortID     string `json:"short_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Segment     string `json:"segment,omitempty"`   // "B2B", "B2C", "internal"
}

func NewPersonaURN(tenant, shortID string) URN {
	return NewURN(ProviderGov, tenant, KindPersona, shortID)
}

func (p Persona) ContentHash() string {
	return hexSum(p.ShortID + "|" + p.Name + "|" + p.Segment + "|" + p.Description)
}

// hexSum é helper local para nós governance — todos seguem o mesmo
// padrão de hash sobre uma string canônica.
func hexSum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
