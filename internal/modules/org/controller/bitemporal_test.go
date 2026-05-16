package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAsOf_SnapshotsHistoricalState garante que `?as_of=t` recupera
// estado vigente naquele instante. O fixture cria a hierarquia em
// 2026-05-14; antes dessa data o time não existia ainda, então a
// listagem deve voltar vazia.
func TestAsOf_SnapshotsHistoricalState(t *testing.T) {
	h, r := fixture(t)
	newOrgFixture(t, r)

	rr := doReq(t, h, "GET",
		"/v1/teams?as_of=2026-01-01T00:00:00Z", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body["results"].([]any)) != 0 {
		t.Fatalf("as_of antes do fixture: results=%v want=empty",
			body["results"])
	}

	rr2 := doReq(t, h, "GET",
		"/v1/teams?as_of=2026-06-01T00:00:00Z", nil)
	var body2 map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&body2)
	if len(body2["results"].([]any)) != 2 {
		t.Fatalf("as_of depois do fixture: results=%d want=2",
			len(body2["results"].([]any)))
	}
}

func TestAsOf_InvalidValue_400(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/teams?as_of=garbage", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestIncludeInactive_HidesByDefault(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	if err := r.Delete(context.Background(), f.personAlice); err != nil {
		t.Fatalf("close alice: %v", err)
	}

	rr := doReq(t, h, "GET",
		"/v1/squads/"+string(f.squadCheck)+"/members", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	results := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("len=%d want=1 (only Bob)", len(results))
	}
	if first, _ := results[0].(map[string]any); first["urn"] != string(f.personBob) {
		t.Fatalf("expected only Bob, got urn=%v", first["urn"])
	}
}

func TestIncludeInactive_RevealsTerminated(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	if err := r.Delete(context.Background(), f.personAlice); err != nil {
		t.Fatalf("close alice: %v", err)
	}

	rr := doReq(t, h, "GET",
		"/v1/squads/"+string(f.squadCheck)+"/members?include_inactive=true", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	results := body["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("len=%d want=2 (Bob + Alice inativa)", len(results))
	}
	urns := map[string]map[string]any{}
	for _, raw := range results {
		m := raw.(map[string]any)
		urns[m["urn"].(string)] = m
	}
	alice := urns[string(f.personAlice)]
	if alice == nil {
		t.Fatalf("Alice (inativa) ausente")
	}
	if email := alice["email"].(string); !strings.Contains(email, "***@") {
		t.Fatalf("Alice email não mascarada: %q", email)
	}
	if alice["active"].(bool) {
		t.Fatalf("Alice deveria ter active=false")
	}
}

func TestIncludeInactive_PersonDetail(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	if err := r.Delete(context.Background(), f.personAlice); err != nil {
		t.Fatalf("close alice: %v", err)
	}

	rr := doReq(t, h, "GET", "/v1/people/"+string(f.personAlice), nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("default status=%d want=404", rr.Code)
	}

	rr2 := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"?include_inactive=true", nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("include_inactive status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&body)
	if body["active"].(bool) {
		t.Fatalf("active=true want=false")
	}
	if body["urn"] != string(f.personAlice) {
		t.Fatalf("urn=%v", body["urn"])
	}
}

func TestIncludeInactive_InvalidValue_400(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET",
		"/v1/squads/"+string(f.squadCheck)+"/members?include_inactive=maybe", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestIncludeInactive_ReportsTreeShowsClosed(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	addReportsTo(t, r, f.personBob, f.personAlice, now)
	if err := r.Delete(context.Background(), f.personBob); err != nil {
		t.Fatalf("close bob: %v", err)
	}

	rr := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"/reports", nil)
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if reports, _ := body["reports"].([]any); len(reports) != 0 {
		t.Fatalf("default reports=%v want=empty", reports)
	}

	rr2 := doReq(t, h, "GET",
		"/v1/people/"+string(f.personAlice)+"/reports?include_inactive=true", nil)
	var body2 map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&body2)
	reports2 := body2["reports"].([]any)
	if len(reports2) != 1 {
		t.Fatalf("include_inactive reports=%d want=1", len(reports2))
	}
	bobNode := reports2[0].(map[string]any)["person"].(map[string]any)
	if bobNode["urn"] != string(f.personBob) || bobNode["active"].(bool) {
		t.Fatalf("Bob node inesperado: %+v", bobNode)
	}
}
