package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/gov"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

// kebabRE valida short_id: minúsculas, dígitos e hífen; sem hífen no
// começo/fim; sem duplos. ADR-009 — Feature.ShortID é chave em
// feature_tags do código.
var kebabRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func validateShortID(s string) error {
	if s == "" {
		return &gov.ErrValidation{Field: "short_id", Reason: "required"}
	}
	if len(s) > 64 {
		return &gov.ErrValidation{Field: "short_id", Reason: "max 64 chars"}
	}
	if !kebabRE.MatchString(s) {
		return &gov.ErrValidation{Field: "short_id", Reason: "must be kebab-case ([a-z0-9-])"}
	}
	return nil
}

func validateName(s string) error {
	if strings.TrimSpace(s) == "" {
		return &gov.ErrValidation{Field: "name", Reason: "required"}
	}
	if len(s) > 200 {
		return &gov.ErrValidation{Field: "name", Reason: "max 200 chars"}
	}
	return nil
}

// requireTenant obriga tenant no ctx. Sem tenant não há como construir
// URN canônica.
func requireTenant(ctx context.Context) (string, error) {
	t := httpserver.TenantIDFrom(ctx)
	if t == "" {
		return "", &gov.ErrValidation{Field: "tenant", Reason: "missing X-Tenant-ID"}
	}
	return t, nil
}

// nowMeta monta a Meta canônica para um node recém-criado.
func (r *service) nowMeta() node.Meta {
	now := r.now().UTC()
	return node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1}
}

// ensureNotExists devolve ErrConflict se já há versão corrente na URN.
// Usado por Create para evitar versionar implicitamente quando o cliente
// quis criar do zero.
func (r *service) ensureNotExists(ctx context.Context, urn node.URN, kind node.Kind) error {
	_, err := r.nodes.GetByURN(ctx, urn, repository.AsOf{})
	if err == nil {
		return &gov.ErrConflict{URN: urn, Kind: kind}
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	return err
}

// emitContains insere o edge CONTAINS pai→filho com ID determinístico.
// Idempotente — o repo bitemporal vai versionar se já existir.
func (r *service) emitContains(ctx context.Context, parent, child node.URN, parentKind, childKind node.Kind, when time.Time) error {
	if r.edges == nil {
		return errors.New("gov/service: edges repo not configured")
	}
	e := edge.Contains{Base: edge.Base{
		EdgeID:   edge.DeterministicID(parent, edge.TypeContains, child, when),
		EdgeType: edge.TypeContains,
		FromURN:  parent, ToURN: child,
		EdgeMeta: edge.Meta{ValidFrom: when, ObservedAt: when, Directional: true, Confidence: 1},
	}}
	return r.edges.Upsert(ctx, e, parentKind, childKind)
}

// expectKind valida que a URN aponta para um nó do kind esperado.
// Usado para validar parents antes de criar filhos.
func (r *service) expectKind(ctx context.Context, urn node.URN, want node.Kind) error {
	if urn == "" {
		return &gov.ErrValidation{Field: "parent_urn", Reason: "required"}
	}
	n, err := r.nodes.GetByURN(ctx, urn, repository.AsOf{})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &gov.ErrValidation{Field: "parent_urn", Reason: "parent not found: " + string(urn)}
		}
		return err
	}
	if n.Kind() != want {
		return &gov.ErrInvalidURN{URN: urn, Expected: want, Reason: "parent kind mismatch (got " + string(n.Kind()) + ")"}
	}
	return nil
}

// ---------- Company ----------

func (r *service) CreateCompany(ctx context.Context, in gov.CreateCompanyInput) (node.Company, error) {
	tenant, err := requireTenant(ctx)
	if err != nil {
		return node.Company{}, err
	}
	if err := validateShortID(in.ShortID); err != nil {
		return node.Company{}, err
	}
	if err := validateName(in.Name); err != nil {
		return node.Company{}, err
	}
	urn := node.NewCompanyURN(tenant, in.ShortID)
	if err := r.ensureNotExists(ctx, urn, node.KindCompany); err != nil {
		return node.Company{}, err
	}
	c := node.Company{
		Base:   node.Base{NodeURN: urn, NodeKind: node.KindCompany, NodeMeta: r.nowMeta()},
		Tenant: tenant, ShortID: in.ShortID, Name: in.Name, Domain: in.Domain,
	}
	if err := r.nodes.Upsert(ctx, c); err != nil {
		return node.Company{}, err
	}
	return c, nil
}

func (r *service) UpdateCompany(ctx context.Context, urn node.URN, in gov.UpdateCompanyInput) (node.Company, error) {
	cur, err := r.GetCompany(ctx, urn, gov.AsOfOptions{})
	if err != nil {
		return node.Company{}, err
	}
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return node.Company{}, err
		}
		cur.Name = *in.Name
	}
	if in.Domain != nil {
		cur.Domain = *in.Domain
	}
	cur.NodeMeta = r.bumpMeta(cur.NodeMeta)
	if err := r.nodes.Upsert(ctx, cur); err != nil {
		return node.Company{}, err
	}
	return cur, nil
}

func (r *service) DeleteCompany(ctx context.Context, urn node.URN) error {
	return r.softDelete(ctx, urn, node.KindCompany)
}

// ---------- BusinessArea ----------

func (r *service) CreateBusinessArea(ctx context.Context, in gov.CreateBusinessAreaInput) (node.BusinessArea, error) {
	tenant, err := requireTenant(ctx)
	if err != nil {
		return node.BusinessArea{}, err
	}
	if err := validateShortID(in.ShortID); err != nil {
		return node.BusinessArea{}, err
	}
	if err := validateName(in.Name); err != nil {
		return node.BusinessArea{}, err
	}
	if err := r.expectKind(ctx, in.CompanyURN, node.KindCompany); err != nil {
		return node.BusinessArea{}, err
	}
	urn := node.NewBusinessAreaURN(tenant, in.ShortID)
	if err := r.ensureNotExists(ctx, urn, node.KindBusinessArea); err != nil {
		return node.BusinessArea{}, err
	}
	b := node.BusinessArea{
		Base:   node.Base{NodeURN: urn, NodeKind: node.KindBusinessArea, NodeMeta: r.nowMeta()},
		Tenant: tenant, ShortID: in.ShortID, Name: in.Name, CompanyURN: in.CompanyURN,
	}
	if err := r.nodes.Upsert(ctx, b); err != nil {
		return node.BusinessArea{}, err
	}
	if err := r.emitContains(ctx, in.CompanyURN, urn, node.KindCompany, node.KindBusinessArea, b.NodeMeta.ValidFrom); err != nil {
		return node.BusinessArea{}, err
	}
	return b, nil
}

func (r *service) UpdateBusinessArea(ctx context.Context, urn node.URN, in gov.UpdateBusinessAreaInput) (node.BusinessArea, error) {
	cur, err := r.GetBusinessArea(ctx, urn, gov.AsOfOptions{})
	if err != nil {
		return node.BusinessArea{}, err
	}
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return node.BusinessArea{}, err
		}
		cur.Name = *in.Name
	}
	cur.NodeMeta = r.bumpMeta(cur.NodeMeta)
	if err := r.nodes.Upsert(ctx, cur); err != nil {
		return node.BusinessArea{}, err
	}
	return cur, nil
}

func (r *service) DeleteBusinessArea(ctx context.Context, urn node.URN) error {
	return r.softDelete(ctx, urn, node.KindBusinessArea)
}

// ---------- Domain ----------

func (r *service) CreateDomain(ctx context.Context, in gov.CreateDomainInput) (node.Domain, error) {
	tenant, err := requireTenant(ctx)
	if err != nil {
		return node.Domain{}, err
	}
	if err := validateShortID(in.ShortID); err != nil {
		return node.Domain{}, err
	}
	if err := validateName(in.Name); err != nil {
		return node.Domain{}, err
	}
	if err := r.expectKind(ctx, in.ParentAreaURN, node.KindBusinessArea); err != nil {
		return node.Domain{}, err
	}
	urn := node.NewDomainURN(tenant, in.ShortID)
	if err := r.ensureNotExists(ctx, urn, node.KindDomain); err != nil {
		return node.Domain{}, err
	}
	d := node.Domain{
		Base:   node.Base{NodeURN: urn, NodeKind: node.KindDomain, NodeMeta: r.nowMeta()},
		Tenant: tenant, ShortID: in.ShortID, Name: in.Name,
		Description: in.Description, ParentAreaURN: in.ParentAreaURN,
	}
	if err := r.nodes.Upsert(ctx, d); err != nil {
		return node.Domain{}, err
	}
	if err := r.emitContains(ctx, in.ParentAreaURN, urn, node.KindBusinessArea, node.KindDomain, d.NodeMeta.ValidFrom); err != nil {
		return node.Domain{}, err
	}
	return d, nil
}

func (r *service) UpdateDomain(ctx context.Context, urn node.URN, in gov.UpdateDomainInput) (node.Domain, error) {
	cur, err := r.GetDomain(ctx, urn, gov.AsOfOptions{})
	if err != nil {
		return node.Domain{}, err
	}
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return node.Domain{}, err
		}
		cur.Name = *in.Name
	}
	if in.Description != nil {
		cur.Description = *in.Description
	}
	cur.NodeMeta = r.bumpMeta(cur.NodeMeta)
	if err := r.nodes.Upsert(ctx, cur); err != nil {
		return node.Domain{}, err
	}
	return cur, nil
}

func (r *service) DeleteDomain(ctx context.Context, urn node.URN) error {
	return r.softDelete(ctx, urn, node.KindDomain)
}

// ---------- Capability ----------

func (r *service) CreateCapability(ctx context.Context, in gov.CreateCapabilityInput) (node.Capability, error) {
	tenant, err := requireTenant(ctx)
	if err != nil {
		return node.Capability{}, err
	}
	if err := validateShortID(in.ShortID); err != nil {
		return node.Capability{}, err
	}
	if err := validateName(in.Name); err != nil {
		return node.Capability{}, err
	}
	if err := r.expectKind(ctx, in.ParentDomainURN, node.KindDomain); err != nil {
		return node.Capability{}, err
	}
	urn := node.NewCapabilityURN(tenant, in.ShortID)
	if err := r.ensureNotExists(ctx, urn, node.KindCapability); err != nil {
		return node.Capability{}, err
	}
	c := node.Capability{
		Base:   node.Base{NodeURN: urn, NodeKind: node.KindCapability, NodeMeta: r.nowMeta()},
		Tenant: tenant, ShortID: in.ShortID, Name: in.Name,
		Description: in.Description, ParentDomainURN: in.ParentDomainURN,
	}
	if err := r.nodes.Upsert(ctx, c); err != nil {
		return node.Capability{}, err
	}
	if err := r.emitContains(ctx, in.ParentDomainURN, urn, node.KindDomain, node.KindCapability, c.NodeMeta.ValidFrom); err != nil {
		return node.Capability{}, err
	}
	return c, nil
}

func (r *service) UpdateCapability(ctx context.Context, urn node.URN, in gov.UpdateCapabilityInput) (node.Capability, error) {
	cur, err := r.GetCapability(ctx, urn, gov.AsOfOptions{})
	if err != nil {
		return node.Capability{}, err
	}
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return node.Capability{}, err
		}
		cur.Name = *in.Name
	}
	if in.Description != nil {
		cur.Description = *in.Description
	}
	cur.NodeMeta = r.bumpMeta(cur.NodeMeta)
	if err := r.nodes.Upsert(ctx, cur); err != nil {
		return node.Capability{}, err
	}
	return cur, nil
}

func (r *service) DeleteCapability(ctx context.Context, urn node.URN) error {
	return r.softDelete(ctx, urn, node.KindCapability)
}

// ---------- Feature ----------

func (r *service) CreateFeature(ctx context.Context, in gov.CreateFeatureInput) (node.Feature, error) {
	tenant, err := requireTenant(ctx)
	if err != nil {
		return node.Feature{}, err
	}
	if err := validateShortID(in.ShortID); err != nil {
		return node.Feature{}, err
	}
	if err := validateName(in.Name); err != nil {
		return node.Feature{}, err
	}
	if err := r.expectKind(ctx, in.ParentCapURN, node.KindCapability); err != nil {
		return node.Feature{}, err
	}
	if in.ParentFeatureURN != "" {
		if err := r.expectKind(ctx, in.ParentFeatureURN, node.KindFeature); err != nil {
			return node.Feature{}, err
		}
	}
	urn := node.NewFeatureURN(tenant, in.ShortID)
	if err := r.ensureNotExists(ctx, urn, node.KindFeature); err != nil {
		return node.Feature{}, err
	}
	f := node.Feature{
		Base:   node.Base{NodeURN: urn, NodeKind: node.KindFeature, NodeMeta: r.nowMeta()},
		Tenant: tenant, ShortID: in.ShortID, Name: in.Name, Description: in.Description,
		ParentCapURN:     in.ParentCapURN,
		ParentFeatureURN: in.ParentFeatureURN,
		Status:           in.Status, SLA: in.SLA, Priority: in.Priority,
	}
	if err := r.nodes.Upsert(ctx, f); err != nil {
		return node.Feature{}, err
	}
	// CONTAINS: pai imediato (sub-feature herda da feature-pai; senão da capability).
	parent, parentKind := in.ParentCapURN, node.KindCapability
	if in.ParentFeatureURN != "" {
		parent, parentKind = in.ParentFeatureURN, node.KindFeature
	}
	if err := r.emitContains(ctx, parent, urn, parentKind, node.KindFeature, f.NodeMeta.ValidFrom); err != nil {
		return node.Feature{}, err
	}
	return f, nil
}

func (r *service) UpdateFeature(ctx context.Context, urn node.URN, in gov.UpdateFeatureInput) (node.Feature, error) {
	cur, err := r.GetFeature(ctx, urn, gov.AsOfOptions{})
	if err != nil {
		return node.Feature{}, err
	}
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return node.Feature{}, err
		}
		cur.Name = *in.Name
	}
	if in.Description != nil {
		cur.Description = *in.Description
	}
	if in.Status != nil {
		cur.Status = *in.Status
	}
	if in.SLA != nil {
		cur.SLA = *in.SLA
	}
	if in.Priority != nil {
		cur.Priority = *in.Priority
	}
	cur.NodeMeta = r.bumpMeta(cur.NodeMeta)
	if err := r.nodes.Upsert(ctx, cur); err != nil {
		return node.Feature{}, err
	}
	return cur, nil
}

func (r *service) DeleteFeature(ctx context.Context, urn node.URN) error {
	return r.softDelete(ctx, urn, node.KindFeature)
}

// ---------- shared write helpers ----------

// bumpMeta atualiza ValidFrom/ObservedAt para now mantendo Version (o
// repo bitemporal incrementa internamente). É idempotente do ponto de
// vista do conteúdo: se o struct ficar idêntico ao corrente, o Upsert
// faz no-op.
func (r *service) bumpMeta(m node.Meta) node.Meta {
	now := r.now().UTC()
	m.ValidFrom = now
	m.ObservedAt = now
	return m
}

// softDelete fecha valid_to do nó corrente sem criar nova versão.
// Map repository.ErrNotFound → gov.ErrNotFound.
func (r *service) softDelete(ctx context.Context, urn node.URN, kind node.Kind) error {
	// Confirma kind primeiro — evita fechar nó alheio por engano.
	if _, err := r.fetchKind(ctx, urn, kind, gov.AsOfOptions{}); err != nil {
		return err
	}
	return r.nodes.Delete(ctx, urn)
}
