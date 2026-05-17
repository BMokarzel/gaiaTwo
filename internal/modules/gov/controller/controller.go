// Package controller é o controller HTTP do módulo gov (F-024 read +
// F-012 write).
//
// Endpoints:
//
//	GET    /v1/gov/companies                — list
//	GET    /v1/gov/companies/{urn}          — get
//	POST   /v1/gov/companies                — create
//	PATCH  /v1/gov/companies/{urn}          — update (PATCH semântico)
//	DELETE /v1/gov/companies/{urn}          — soft delete (close valid_to)
//
//	(idem para business_areas, domains, capabilities, features)
//
//	GET /v1/gov/features/{urn}/children — sub-features (CONTAINS feature→feature)
//
// Convenção: parse → svc → view. Erros via httpserver.WriteError.
// Tenant vem do header `X-Tenant-ID` (auth middleware); payload jamais
// carrega tenant.
package controller

import (
	"net/http"

	"costEngine/internal/modules/gov"
)

// Controller agrega `gov.Service` e satisfaz `httpserver.Registrar`.
type Controller struct {
	svc gov.Service
}

// New cria o Controller. svc nil → panic (fail-fast).
func New(svc gov.Service) *Controller {
	if svc == nil {
		panic("gov/controller: New requires non-nil service")
	}
	return &Controller{svc: svc}
}

// Register registra os endpoints no mux compartilhado.
//
// URNs contêm `/` e `:`; usamos wildcard `{rest...}` (Go 1.22+) e
// dispatch interno por sufixo conhecido (apenas `/children` em features).
func (c *Controller) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/gov/companies", c.handleListCompanies)
	mux.HandleFunc("POST /v1/gov/companies", c.handleCreateCompany)
	mux.HandleFunc("GET /v1/gov/companies/{rest...}", c.handleGetCompany)
	mux.HandleFunc("PATCH /v1/gov/companies/{rest...}", c.dispatchCompany)
	mux.HandleFunc("DELETE /v1/gov/companies/{rest...}", c.dispatchCompany)

	mux.HandleFunc("GET /v1/gov/business_areas", c.handleListBusinessAreas)
	mux.HandleFunc("POST /v1/gov/business_areas", c.handleCreateBusinessArea)
	mux.HandleFunc("GET /v1/gov/business_areas/{rest...}", c.handleGetBusinessArea)
	mux.HandleFunc("PATCH /v1/gov/business_areas/{rest...}", c.dispatchBusinessArea)
	mux.HandleFunc("DELETE /v1/gov/business_areas/{rest...}", c.dispatchBusinessArea)

	mux.HandleFunc("GET /v1/gov/domains", c.handleListDomains)
	mux.HandleFunc("POST /v1/gov/domains", c.handleCreateDomain)
	mux.HandleFunc("GET /v1/gov/domains/{rest...}", c.handleGetDomain)
	mux.HandleFunc("PATCH /v1/gov/domains/{rest...}", c.dispatchDomain)
	mux.HandleFunc("DELETE /v1/gov/domains/{rest...}", c.dispatchDomain)

	mux.HandleFunc("GET /v1/gov/capabilities", c.handleListCapabilities)
	mux.HandleFunc("POST /v1/gov/capabilities", c.handleCreateCapability)
	mux.HandleFunc("GET /v1/gov/capabilities/{rest...}", c.handleGetCapability)
	mux.HandleFunc("PATCH /v1/gov/capabilities/{rest...}", c.dispatchCapability)
	mux.HandleFunc("DELETE /v1/gov/capabilities/{rest...}", c.dispatchCapability)

	mux.HandleFunc("GET /v1/gov/features", c.handleListFeatures)
	mux.HandleFunc("POST /v1/gov/features", c.handleCreateFeature)
	mux.HandleFunc("GET /v1/gov/features/{rest...}", c.dispatchFeature)
	mux.HandleFunc("PATCH /v1/gov/features/{rest...}", c.dispatchFeatureWrite)
	mux.HandleFunc("DELETE /v1/gov/features/{rest...}", c.dispatchFeatureWrite)
}
