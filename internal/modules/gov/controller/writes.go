package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/gov"
	"costEngine/internal/platform/httpserver"
)

// decodeBody decodifica JSON com DisallowUnknownFields. Body grande não
// é problema neste módulo (payloads são pequenos) — sem MaxBytesReader
// no MVP.
func decodeBody(r *http.Request, out any) error {
	if r.Body == nil {
		return httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing body")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return httpserver.Errorf(http.StatusBadRequest, "bad_request", "empty body")
		}
		return httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid json: %s", err.Error())
	}
	return nil
}

// ---------- Company ----------

type createCompanyBody struct {
	ShortID string `json:"short_id"`
	Name    string `json:"name"`
	Domain  string `json:"domain"`
}

type updateCompanyBody struct {
	Name   *string `json:"name"`
	Domain *string `json:"domain"`
}

func (c *Controller) handleCreateCompany(w http.ResponseWriter, r *http.Request) {
	var b createCompanyBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.CreateCompany(r.Context(), gov.CreateCompanyInput{
		ShortID: b.ShortID, Name: b.Name, Domain: b.Domain,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, companyView(x))
}

func (c *Controller) handlePatchCompany(w http.ResponseWriter, r *http.Request, urn node.URN) {
	var b updateCompanyBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.UpdateCompany(r.Context(), urn, gov.UpdateCompanyInput{
		Name: b.Name, Domain: b.Domain,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, companyView(x))
}

func (c *Controller) handleDeleteCompany(w http.ResponseWriter, r *http.Request, urn node.URN) {
	if err := c.svc.DeleteCompany(r.Context(), urn); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- BusinessArea ----------

type createBusinessAreaBody struct {
	ShortID    string   `json:"short_id"`
	Name       string   `json:"name"`
	CompanyURN node.URN `json:"company_urn"`
}

type updateBusinessAreaBody struct {
	Name *string `json:"name"`
}

func (c *Controller) handleCreateBusinessArea(w http.ResponseWriter, r *http.Request) {
	var b createBusinessAreaBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.CreateBusinessArea(r.Context(), gov.CreateBusinessAreaInput{
		ShortID: b.ShortID, Name: b.Name, CompanyURN: b.CompanyURN,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, businessAreaView(x))
}

func (c *Controller) handlePatchBusinessArea(w http.ResponseWriter, r *http.Request, urn node.URN) {
	var b updateBusinessAreaBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.UpdateBusinessArea(r.Context(), urn, gov.UpdateBusinessAreaInput{Name: b.Name})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, businessAreaView(x))
}

func (c *Controller) handleDeleteBusinessArea(w http.ResponseWriter, r *http.Request, urn node.URN) {
	if err := c.svc.DeleteBusinessArea(r.Context(), urn); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Domain ----------

type createDomainBody struct {
	ShortID       string   `json:"short_id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	ParentAreaURN node.URN `json:"parent_area_urn"`
}

type updateDomainBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (c *Controller) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	var b createDomainBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.CreateDomain(r.Context(), gov.CreateDomainInput{
		ShortID: b.ShortID, Name: b.Name, Description: b.Description,
		ParentAreaURN: b.ParentAreaURN,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, domainView(x))
}

func (c *Controller) handlePatchDomain(w http.ResponseWriter, r *http.Request, urn node.URN) {
	var b updateDomainBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.UpdateDomain(r.Context(), urn, gov.UpdateDomainInput{
		Name: b.Name, Description: b.Description,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, domainView(x))
}

func (c *Controller) handleDeleteDomain(w http.ResponseWriter, r *http.Request, urn node.URN) {
	if err := c.svc.DeleteDomain(r.Context(), urn); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Capability ----------

type createCapabilityBody struct {
	ShortID         string   `json:"short_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ParentDomainURN node.URN `json:"parent_domain_urn"`
}

type updateCapabilityBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (c *Controller) handleCreateCapability(w http.ResponseWriter, r *http.Request) {
	var b createCapabilityBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.CreateCapability(r.Context(), gov.CreateCapabilityInput{
		ShortID: b.ShortID, Name: b.Name, Description: b.Description,
		ParentDomainURN: b.ParentDomainURN,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, capabilityView(x))
}

func (c *Controller) handlePatchCapability(w http.ResponseWriter, r *http.Request, urn node.URN) {
	var b updateCapabilityBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.UpdateCapability(r.Context(), urn, gov.UpdateCapabilityInput{
		Name: b.Name, Description: b.Description,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, capabilityView(x))
}

func (c *Controller) handleDeleteCapability(w http.ResponseWriter, r *http.Request, urn node.URN) {
	if err := c.svc.DeleteCapability(r.Context(), urn); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Feature ----------

type createFeatureBody struct {
	ShortID          string   `json:"short_id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	ParentCapURN     node.URN `json:"parent_capability_urn"`
	ParentFeatureURN node.URN `json:"parent_feature_urn"`
	Status           string   `json:"status"`
	SLA              string   `json:"sla"`
	Priority         string   `json:"priority"`
}

type updateFeatureBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	SLA         *string `json:"sla"`
	Priority    *string `json:"priority"`
}

func (c *Controller) handleCreateFeature(w http.ResponseWriter, r *http.Request) {
	var b createFeatureBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.CreateFeature(r.Context(), gov.CreateFeatureInput{
		ShortID: b.ShortID, Name: b.Name, Description: b.Description,
		ParentCapURN: b.ParentCapURN, ParentFeatureURN: b.ParentFeatureURN,
		Status: b.Status, SLA: b.SLA, Priority: b.Priority,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, featureView(x))
}

func (c *Controller) handlePatchFeature(w http.ResponseWriter, r *http.Request, urn node.URN) {
	var b updateFeatureBody
	if err := decodeBody(r, &b); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.UpdateFeature(r.Context(), urn, gov.UpdateFeatureInput{
		Name: b.Name, Description: b.Description,
		Status: b.Status, SLA: b.SLA, Priority: b.Priority,
	})
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, featureView(x))
}

func (c *Controller) handleDeleteFeature(w http.ResponseWriter, r *http.Request, urn node.URN) {
	if err := c.svc.DeleteFeature(r.Context(), urn); err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- dispatchers ----------
//
// PATCH/DELETE compartilham o pattern `{rest...}` com GET. Para Feature,
// `/children` continua suportado só em GET.

func (c *Controller) dispatchCompany(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		c.handlePatchCompany(w, r, urn)
	case http.MethodDelete:
		c.handleDeleteCompany(w, r, urn)
	}
}

func (c *Controller) dispatchBusinessArea(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		c.handlePatchBusinessArea(w, r, urn)
	case http.MethodDelete:
		c.handleDeleteBusinessArea(w, r, urn)
	}
}

func (c *Controller) dispatchDomain(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		c.handlePatchDomain(w, r, urn)
	case http.MethodDelete:
		c.handleDeleteDomain(w, r, urn)
	}
}

func (c *Controller) dispatchCapability(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		c.handlePatchCapability(w, r, urn)
	case http.MethodDelete:
		c.handleDeleteCapability(w, r, urn)
	}
}

// dispatchFeatureWrite cobre PATCH/DELETE em `/v1/gov/features/{urn}`.
// GET (incluindo /children) continua em dispatchFeature.
func (c *Controller) dispatchFeatureWrite(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		c.handlePatchFeature(w, r, urn)
	case http.MethodDelete:
		c.handleDeleteFeature(w, r, urn)
	}
}
