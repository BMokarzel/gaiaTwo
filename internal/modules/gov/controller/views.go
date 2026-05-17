package controller

import (
	"costEngine/internal/entity/node"
)

// Views convertem entidades em map[string]any para JSON. ValidTo é
// renderizado só se preenchido — mesma convenção dos org views.

func baseView(n node.Node) map[string]any {
	m := n.Meta()
	v := map[string]any{
		"urn":         n.URN(),
		"kind":        n.Kind(),
		"valid_from":  m.ValidFrom,
		"observed_at": m.ObservedAt,
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
		v["active"] = false
	} else {
		v["active"] = true
	}
	return v
}

func companyView(c node.Company) map[string]any {
	v := baseView(c)
	v["tenant"] = c.Tenant
	v["short_id"] = c.ShortID
	v["name"] = c.Name
	if c.Domain != "" {
		v["domain"] = c.Domain
	}
	return v
}

func businessAreaView(b node.BusinessArea) map[string]any {
	v := baseView(b)
	v["tenant"] = b.Tenant
	v["short_id"] = b.ShortID
	v["name"] = b.Name
	if b.CompanyURN != "" {
		v["company_urn"] = b.CompanyURN
	}
	return v
}

func domainView(d node.Domain) map[string]any {
	v := baseView(d)
	v["tenant"] = d.Tenant
	v["short_id"] = d.ShortID
	v["name"] = d.Name
	if d.Description != "" {
		v["description"] = d.Description
	}
	if d.ParentAreaURN != "" {
		v["parent_area_urn"] = d.ParentAreaURN
	}
	return v
}

func capabilityView(c node.Capability) map[string]any {
	v := baseView(c)
	v["tenant"] = c.Tenant
	v["short_id"] = c.ShortID
	v["name"] = c.Name
	if c.Description != "" {
		v["description"] = c.Description
	}
	if c.ParentDomainURN != "" {
		v["parent_domain_urn"] = c.ParentDomainURN
	}
	return v
}

func featureView(f node.Feature) map[string]any {
	v := baseView(f)
	v["tenant"] = f.Tenant
	v["short_id"] = f.ShortID
	v["name"] = f.Name
	if f.Description != "" {
		v["description"] = f.Description
	}
	if f.ParentCapURN != "" {
		v["parent_capability_urn"] = f.ParentCapURN
	}
	if f.ParentFeatureURN != "" {
		v["parent_feature_urn"] = f.ParentFeatureURN
	}
	if f.Status != "" {
		v["status"] = f.Status
	}
	if f.SLA != "" {
		v["sla"] = f.SLA
	}
	if f.Priority != "" {
		v["priority"] = f.Priority
	}
	return v
}
