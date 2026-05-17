package controller_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	govctrl "costEngine/internal/modules/gov/controller"
	govservice "costEngine/internal/modules/gov/service"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository/memory"
)

// fixture monta o handler completo do módulo gov sobre um memory.Repo.
// O Repo é exposto para que os testes adicionem nós governance.
func fixture(t *testing.T) (http.Handler, *memory.Repo) {
	t.Helper()
	r := memory.New().WithClock(func() time.Time {
		return time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	})
	svc := govservice.New(r, r.AsEdgeRepo())
	ctrl := govctrl.New(svc)
	hs := httpserver.New(httpserver.Config{}, ctrl)
	return hs.Routes(), r
}

func doReq(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

// govFixture é o grafo determinístico usado nos testes:
// 1 Company → 1 BusinessArea → 1 Domain → 1 Capability → 2 Features
// (uma é filha da outra para testar /children).
type govFixture struct {
	company    node.URN
	area       node.URN
	domain     node.URN
	capability node.URN
	featRoot   node.URN
	featChild  node.URN
}

func newGovFixture(t *testing.T, r *memory.Repo) govFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)

	base := func(urn node.URN, kind node.Kind) node.Base {
		return node.Base{
			NodeURN: urn, NodeKind: kind,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		}
	}

	company := node.Company{
		Base:    base(node.NewCompanyURN("acme", "acme"), node.KindCompany),
		Tenant:  "acme", ShortID: "acme", Name: "Acme Inc.",
		Domain:  "acme.com",
	}
	area := node.BusinessArea{
		Base:       base(node.NewBusinessAreaURN("acme", "marketplace"), node.KindBusinessArea),
		Tenant:     "acme", ShortID: "marketplace", Name: "Marketplace",
		CompanyURN: company.URN(),
	}
	domain := node.Domain{
		Base:          base(node.NewDomainURN("acme", "checkout"), node.KindDomain),
		Tenant:        "acme", ShortID: "checkout", Name: "Checkout",
		ParentAreaURN: area.URN(),
	}
	capability := node.Capability{
		Base:            base(node.NewCapabilityURN("acme", "payments"), node.KindCapability),
		Tenant:          "acme", ShortID: "payments", Name: "Payments",
		ParentDomainURN: domain.URN(),
	}
	featRoot := node.Feature{
		Base:         base(node.NewFeatureURN("acme", "credit-card"), node.KindFeature),
		Tenant:       "acme", ShortID: "credit-card", Name: "Credit Card",
		ParentCapURN: capability.URN(), Status: "active",
	}
	featChild := node.Feature{
		Base:             base(node.NewFeatureURN("acme", "3ds"), node.KindFeature),
		Tenant:           "acme", ShortID: "3ds", Name: "3DS",
		ParentCapURN:     capability.URN(),
		ParentFeatureURN: featRoot.URN(),
		Status:           "active",
	}
	for _, n := range []node.Node{company, area, domain, capability, featRoot, featChild} {
		if err := r.Upsert(ctx, n); err != nil {
			t.Fatalf("upsert %s: %v", n.URN(), err)
		}
	}
	return govFixture{
		company: company.URN(), area: area.URN(), domain: domain.URN(),
		capability: capability.URN(), featRoot: featRoot.URN(), featChild: featChild.URN(),
	}
}

// ---------- Company ----------

func TestListCompanies_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/gov/companies")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	results, _ := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results=%d want=1", len(results))
	}
	m := results[0].(map[string]any)
	if m["urn"] != string(f.company) {
		t.Fatalf("urn=%v", m["urn"])
	}
	if m["domain"] != "acme.com" {
		t.Fatalf("domain=%v", m["domain"])
	}
}

func TestGetCompany_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/companies/"+string(f.company))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["name"] != "Acme Inc." {
		t.Fatalf("name=%v", body["name"])
	}
}

func TestGetCompany_NotFound(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/gov/companies/urn:ce:gov:acme:company/ghost")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetCompany_WrongKind_400(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/companies/"+string(f.domain))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// ---------- BusinessArea ----------

func TestGetBusinessArea_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/business_areas/"+string(f.area))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["company_urn"] != string(f.company) {
		t.Fatalf("company_urn=%v", body["company_urn"])
	}
}

// ---------- Domain ----------

func TestListDomains_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/domains")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := decodeJSON(t, rr.Body)
	results := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results=%d", len(results))
	}
	m := results[0].(map[string]any)
	if m["urn"] != string(f.domain) {
		t.Fatalf("urn=%v", m["urn"])
	}
	if m["parent_area_urn"] != string(f.area) {
		t.Fatalf("parent_area_urn=%v", m["parent_area_urn"])
	}
}

// ---------- Capability ----------

func TestGetCapability_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/capabilities/"+string(f.capability))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := decodeJSON(t, rr.Body)
	if body["parent_domain_urn"] != string(f.domain) {
		t.Fatalf("parent_domain_urn=%v", body["parent_domain_urn"])
	}
}

// ---------- Feature ----------

func TestListFeatures_OK(t *testing.T) {
	h, r := fixture(t)
	newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/features")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := decodeJSON(t, rr.Body)
	if len(body["results"].([]any)) != 2 {
		t.Fatalf("results=%d want=2", len(body["results"].([]any)))
	}
}

func TestGetFeature_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/features/"+string(f.featRoot))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := decodeJSON(t, rr.Body)
	if body["name"] != "Credit Card" {
		t.Fatalf("name=%v", body["name"])
	}
	if body["status"] != "active" {
		t.Fatalf("status=%v", body["status"])
	}
}

func TestGetFeature_NotFound(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/gov/features/urn:ce:gov:acme:feature/ghost")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestFeatureChildren_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/gov/features/"+string(f.featRoot)+"/children")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	results := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("children=%d want=1", len(results))
	}
	child := results[0].(map[string]any)
	if child["urn"] != string(f.featChild) {
		t.Fatalf("child urn=%v", child["urn"])
	}
	if child["parent_feature_urn"] != string(f.featRoot) {
		t.Fatalf("parent_feature_urn=%v", child["parent_feature_urn"])
	}
}

func TestFeatureChildren_ParentNotFound(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/gov/features/urn:ce:gov:acme:feature/ghost/children")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFeatureChildren_ParentWrongKind(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	// Capability na rota /features/.../children → 400 (wrong kind para parent).
	rr := doReq(t, h, "GET", "/v1/gov/features/"+string(f.capability)+"/children")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// ---------- Listing meta ----------

func TestListFeatures_Pagination(t *testing.T) {
	h, r := fixture(t)
	newGovFixture(t, r)

	// limit=1 → 2 páginas (2 features).
	rr := doReq(t, h, "GET", "/v1/gov/features?limit=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("p1 status=%d", rr.Code)
	}
	var p1 map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&p1)
	if len(p1["results"].([]any)) != 1 {
		t.Fatalf("p1 results=%d", len(p1["results"].([]any)))
	}
	next, _ := p1["next_cursor"].(string)
	if next == "" {
		t.Fatalf("missing next_cursor")
	}

	rr2 := doReq(t, h, "GET", "/v1/gov/features?limit=1&cursor="+next)
	if rr2.Code != http.StatusOK {
		t.Fatalf("p2 status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var p2 map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&p2)
	if len(p2["results"].([]any)) != 1 {
		t.Fatalf("p2 results=%d", len(p2["results"].([]any)))
	}
	if p2["next_cursor"].(string) != "" {
		t.Fatalf("p2 should have no next_cursor")
	}
}

func TestListFeatures_InvalidLimit(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/gov/features?limit=0")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestList_InvalidAsOf(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/gov/companies?as_of=not-a-date")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "as_of") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestGet_MissingURN(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/gov/companies/")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
