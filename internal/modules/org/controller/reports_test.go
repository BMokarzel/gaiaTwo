package controller_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestPersonReports_DirectReports(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	addReportsTo(t, r, f.personBob, f.personAlice, now)

	rr := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"/reports", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if int(body["depth"].(float64)) != 1 {
		t.Fatalf("depth=%v", body["depth"])
	}
	if body["truncated"].(bool) {
		t.Fatal("truncated true unexpectedly")
	}
	reports := body["reports"].([]any)
	if len(reports) != 1 {
		t.Fatalf("len(reports)=%d", len(reports))
	}
	first := reports[0].(map[string]any)
	person := first["person"].(map[string]any)
	if person["urn"] != string(f.personBob) {
		t.Fatalf("subordinate urn=%v want=%v", person["urn"], f.personBob)
	}
	if len(first["reports"].([]any)) != 0 {
		t.Fatalf("depth=1 should not include grandchildren")
	}
}

func TestPersonReports_DepthTwo_BuildsTree(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	carol := mkPerson(t, r, "carol@a.com", "Carol", f.squadCheck, now)
	addReportsTo(t, r, f.personBob, f.personAlice, now)
	addReportsTo(t, r, carol, f.personBob, now)

	rr := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"/reports?depth=2", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if int(body["depth"].(float64)) != 2 {
		t.Fatalf("depth=%v", body["depth"])
	}
	reports := body["reports"].([]any)
	if len(reports) != 1 {
		t.Fatalf("len=%d", len(reports))
	}
	bobNode := reports[0].(map[string]any)
	bobReports := bobNode["reports"].([]any)
	if len(bobReports) != 1 {
		t.Fatalf("bob reports=%d want=1", len(bobReports))
	}
	carolNode := bobReports[0].(map[string]any)
	carolPerson := carolNode["person"].(map[string]any)
	if carolPerson["urn"] != string(carol) {
		t.Fatalf("carol urn=%v", carolPerson["urn"])
	}
}

func TestPersonReports_DepthCap_400(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"/reports?depth=99", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

// TestPersonReports_Cycle_Truncated cria Alice → Bob → Alice
// (ciclo). O endpoint deve interromper e marcar truncated=true.
func TestPersonReports_Cycle_Truncated(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	addReportsTo(t, r, f.personBob, f.personAlice, now)
	addReportsTo(t, r, f.personAlice, f.personBob, now)

	rr := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"/reports?depth=3", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if !body["truncated"].(bool) {
		t.Fatal("expected truncated=true on cycle")
	}
}

func TestPersonReports_NoSubordinates_EmptyTree(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET",
		"/v1/people/"+string(f.personBob)+"/reports", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if rep, _ := body["reports"].([]any); rep == nil || len(rep) != 0 {
		t.Fatalf("reports=%v want=empty", body["reports"])
	}
}

func TestPersonReports_PersonNotFound_404(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/people/urn:ce:org:acme:person/missing/reports", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}
