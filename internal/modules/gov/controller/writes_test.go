package controller_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"costEngine/internal/platform/httpserver"
)

// doWrite envia método/path com body JSON + X-Tenant-ID. Reuso do
// fixture/newGovFixture do handlers_test.go.
func doWrite(t *testing.T, h http.Handler, method, path, tenant string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if tenant != "" {
		req.Header.Set(httpserver.HeaderTenantID, tenant)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// ---------- Company ----------

func TestCreateCompany_OK(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "POST", "/v1/gov/companies", "acme", map[string]any{
		"short_id": "newco", "name": "NewCo", "domain": "newco.io",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["urn"] == nil || !strings.Contains(body["urn"].(string), "company/newco") {
		t.Fatalf("urn=%v", body["urn"])
	}
	if body["name"] != "NewCo" {
		t.Fatalf("name=%v", body["name"])
	}
}

func TestCreateCompany_ValidationMissingName(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "POST", "/v1/gov/companies", "acme", map[string]any{
		"short_id": "newco",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateCompany_InvalidShortID(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "POST", "/v1/gov/companies", "acme", map[string]any{
		"short_id": "Bad Name!", "name": "X",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateCompany_Conflict(t *testing.T) {
	h, r := fixture(t)
	newGovFixture(t, r) // já cria company "acme"
	rr := doWrite(t, h, "POST", "/v1/gov/companies", "acme", map[string]any{
		"short_id": "acme", "name": "Dup",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateCompany_UnknownField(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "POST", "/v1/gov/companies", "acme", map[string]any{
		"short_id": "co", "name": "N", "unknown": "x",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPatchCompany_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	newName := "Acme Renamed"
	rr := doWrite(t, h, "PATCH", "/v1/gov/companies/"+string(f.company), "acme", map[string]any{
		"name": newName,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["name"] != newName {
		t.Fatalf("name=%v", body["name"])
	}

	// GET reflete a mudança.
	rr2 := doReq(t, h, "GET", "/v1/gov/companies/"+string(f.company))
	if rr2.Code != http.StatusOK {
		t.Fatalf("get status=%d", rr2.Code)
	}
	got := decodeJSON(t, rr2.Body)
	if got["name"] != newName {
		t.Fatalf("post-patch name=%v", got["name"])
	}
}

func TestPatchCompany_NotFound(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "PATCH", "/v1/gov/companies/urn:ce:gov:acme:company/ghost", "acme", map[string]any{
		"name": "X",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDeleteCompany_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "DELETE", "/v1/gov/companies/"+string(f.company), "acme", nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// GET pós-delete → 404 (sem as_of).
	rr2 := doReq(t, h, "GET", "/v1/gov/companies/"+string(f.company))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("post-delete status=%d", rr2.Code)
	}
}

func TestDeleteCompany_WrongKind(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "DELETE", "/v1/gov/companies/"+string(f.domain), "acme", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// ---------- Hierarquia: Create children referencia parents existentes ----------

func TestCreateBusinessArea_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "POST", "/v1/gov/business_areas", "acme", map[string]any{
		"short_id": "logistics", "name": "Logistics",
		"company_urn": string(f.company),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["company_urn"] != string(f.company) {
		t.Fatalf("company_urn=%v", body["company_urn"])
	}
}

func TestCreateBusinessArea_ParentMissing(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "POST", "/v1/gov/business_areas", "acme", map[string]any{
		"short_id": "ops", "name": "Ops",
		"company_urn": "urn:ce:gov:acme:company/ghost",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateBusinessArea_ParentWrongKind(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	// Aponta domain como parent → 400 (kind errado).
	rr := doWrite(t, h, "POST", "/v1/gov/business_areas", "acme", map[string]any{
		"short_id": "ops", "name": "Ops",
		"company_urn": string(f.domain),
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateDomain_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "POST", "/v1/gov/domains", "acme", map[string]any{
		"short_id": "fulfilment", "name": "Fulfilment",
		"description":     "post-checkout",
		"parent_area_urn": string(f.area),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateCapability_OK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "POST", "/v1/gov/capabilities", "acme", map[string]any{
		"short_id": "billing", "name": "Billing",
		"parent_domain_urn": string(f.domain),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateFeature_RootOK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "POST", "/v1/gov/features", "acme", map[string]any{
		"short_id": "pix", "name": "PIX Payment",
		"parent_capability_urn": string(f.capability),
		"status":                "active",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["status"] != "active" {
		t.Fatalf("status=%v", body["status"])
	}
}

func TestCreateFeature_SubFeatureOK(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "POST", "/v1/gov/features", "acme", map[string]any{
		"short_id": "frictionless",
		"name":     "Frictionless 3DS",
		"parent_capability_urn": string(f.capability),
		"parent_feature_urn":    string(f.featRoot),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["parent_feature_urn"] != string(f.featRoot) {
		t.Fatalf("parent=%v", body["parent_feature_urn"])
	}

	// /children deve listar a nova feature.
	rr2 := doReq(t, h, "GET", "/v1/gov/features/"+string(f.featRoot)+"/children")
	if rr2.Code != http.StatusOK {
		t.Fatalf("children status=%d", rr2.Code)
	}
	body2 := decodeJSON(t, rr2.Body)
	results := body2["results"].([]any)
	if len(results) != 2 { // featChild original + frictionless novo
		t.Fatalf("children=%d want=2", len(results))
	}
}

func TestPatchFeature_StatusChange(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	deprecated := "deprecated"
	rr := doWrite(t, h, "PATCH", "/v1/gov/features/"+string(f.featRoot), "acme", map[string]any{
		"status": deprecated,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := decodeJSON(t, rr.Body)
	if body["status"] != deprecated {
		t.Fatalf("status=%v", body["status"])
	}
}

func TestDeleteFeature_SoftClose(t *testing.T) {
	h, r := fixture(t)
	f := newGovFixture(t, r)
	rr := doWrite(t, h, "DELETE", "/v1/gov/features/"+string(f.featChild), "acme", nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
	// Sumiu de /children.
	rr2 := doReq(t, h, "GET", "/v1/gov/features/"+string(f.featRoot)+"/children")
	body := decodeJSON(t, rr2.Body)
	if len(body["results"].([]any)) != 0 {
		t.Fatalf("post-delete children=%d", len(body["results"].([]any)))
	}
}

// ---------- Body errors ----------

func TestCreate_EmptyBody(t *testing.T) {
	h, _ := fixture(t)
	rr := doWrite(t, h, "POST", "/v1/gov/companies", "acme", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
