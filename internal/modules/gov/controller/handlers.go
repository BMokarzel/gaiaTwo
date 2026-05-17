package controller

import (
	"net/http"

	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
)

// Handlers seguem o mesmo padrão dos do org: parse → svc → view. Cada
// kind tem List + Get; Feature tem o extra `/children`.

// ---------- Company ----------

func (c *Controller) handleListCompanies(w http.ResponseWriter, r *http.Request) {
	q, fp, err := c.parseListQuery(r, "companies", "")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	page, err := c.svc.ListCompanies(r.Context(), q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, x := range page.Items {
		out = append(out, companyView(x))
	}
	writeList(w, out, page.Limit, page.NextOffset, fp)
}

func (c *Controller) handleGetCompany(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.GetCompany(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, companyView(x))
}

// ---------- BusinessArea ----------

func (c *Controller) handleListBusinessAreas(w http.ResponseWriter, r *http.Request) {
	q, fp, err := c.parseListQuery(r, "business_areas", "")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	page, err := c.svc.ListBusinessAreas(r.Context(), q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, x := range page.Items {
		out = append(out, businessAreaView(x))
	}
	writeList(w, out, page.Limit, page.NextOffset, fp)
}

func (c *Controller) handleGetBusinessArea(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.GetBusinessArea(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, businessAreaView(x))
}

// ---------- Domain ----------

func (c *Controller) handleListDomains(w http.ResponseWriter, r *http.Request) {
	q, fp, err := c.parseListQuery(r, "domains", "")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	page, err := c.svc.ListDomains(r.Context(), q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, x := range page.Items {
		out = append(out, domainView(x))
	}
	writeList(w, out, page.Limit, page.NextOffset, fp)
}

func (c *Controller) handleGetDomain(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.GetDomain(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, domainView(x))
}

// ---------- Capability ----------

func (c *Controller) handleListCapabilities(w http.ResponseWriter, r *http.Request) {
	q, fp, err := c.parseListQuery(r, "capabilities", "")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	page, err := c.svc.ListCapabilities(r.Context(), q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, x := range page.Items {
		out = append(out, capabilityView(x))
	}
	writeList(w, out, page.Limit, page.NextOffset, fp)
}

func (c *Controller) handleGetCapability(w http.ResponseWriter, r *http.Request) {
	urn, _, ok := splitGovPath(r, nil)
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.GetCapability(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, capabilityView(x))
}

// ---------- Feature ----------

func (c *Controller) handleListFeatures(w http.ResponseWriter, r *http.Request) {
	q, fp, err := c.parseListQuery(r, "features", "")
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	page, err := c.svc.ListFeatures(r.Context(), q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, x := range page.Items {
		out = append(out, featureView(x))
	}
	writeList(w, out, page.Limit, page.NextOffset, fp)
}

// dispatchFeature reparte `/v1/gov/features/{urn}` e
// `/v1/gov/features/{urn}/children`.
func (c *Controller) dispatchFeature(w http.ResponseWriter, r *http.Request) {
	urn, suffix, ok := splitGovPath(r, []string{"/children"})
	if !ok {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_request", "missing URN"))
		return
	}
	switch suffix {
	case "/children":
		c.handleFeatureChildren(w, r, urn)
	default:
		c.handleGetFeature(w, r, urn)
	}
}

func (c *Controller) handleGetFeature(w http.ResponseWriter, r *http.Request, urn node.URN) {
	opts, err := parseAsOfOptions(r.URL.Query())
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	x, err := c.svc.GetFeature(r.Context(), urn, opts)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, featureView(x))
}

func (c *Controller) handleFeatureChildren(w http.ResponseWriter, r *http.Request, parent node.URN) {
	q, fp, err := c.parseListQuery(r, "feature-children", string(parent))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	page, err := c.svc.ListFeatureChildren(r.Context(), parent, q)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	out := make([]any, 0, len(page.Items))
	for _, x := range page.Items {
		out = append(out, featureView(x))
	}
	writeList(w, out, page.Limit, page.NextOffset, fp)
}

// writeList centraliza o envelope de listagem.
func writeList(w http.ResponseWriter, out []any, limit, nextOffset int, fp string) {
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"results":     out,
		"limit":       limit,
		"next_cursor": httpserver.EncodeCursor(httpserver.Cursor{Offset: nextOffset, Fingerprint: fp}),
	})
}
