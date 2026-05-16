package controller_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestListTeams_ReturnsTeamsWithCounts(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/teams", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	results, _ := body["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("len(results)=%d want=2", len(results))
	}
	byURN := map[string]map[string]any{}
	for _, raw := range results {
		m := raw.(map[string]any)
		byURN[m["urn"].(string)] = m
	}
	pay := byURN[string(f.teamPayments)]
	if pay == nil {
		t.Fatalf("payments missing in results")
	}
	if int(pay["squad_count"].(float64)) != 2 {
		t.Fatalf("payments squad_count=%v want=2", pay["squad_count"])
	}
	if int(pay["person_count"].(float64)) != 2 {
		t.Fatalf("payments person_count=%v want=2", pay["person_count"])
	}
	data := byURN[string(f.teamData)]
	if int(data["squad_count"].(float64)) != 0 || int(data["person_count"].(float64)) != 0 {
		t.Fatalf("data counts=%v %v", data["squad_count"], data["person_count"])
	}
}

func TestListTeams_Pagination_Cursor(t *testing.T) {
	h, r := fixture(t)
	newOrgFixture(t, r)

	// limit=1 → 2 páginas (2 teams).
	rr := doReq(t, h, "GET", "/v1/teams?limit=1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("p1 status=%d", rr.Code)
	}
	var p1 map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&p1)
	if len(p1["results"].([]any)) != 1 {
		t.Fatalf("p1 results=%d want=1", len(p1["results"].([]any)))
	}
	next, _ := p1["next_cursor"].(string)
	if next == "" {
		t.Fatalf("p1 missing next_cursor")
	}

	rr2 := doReq(t, h, "GET", "/v1/teams?limit=1&cursor="+next, nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("p2 status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var p2 map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&p2)
	if len(p2["results"].([]any)) != 1 {
		t.Fatalf("p2 results=%d want=1", len(p2["results"].([]any)))
	}
	if p2["next_cursor"].(string) != "" {
		t.Fatalf("p2 should not have next_cursor (got %q)", p2["next_cursor"])
	}
}

func TestListTeams_InvalidCursor_400(t *testing.T) {
	h, r := fixture(t)
	newOrgFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/teams?cursor=garbage", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetTeam_ReturnsDetailWithSquads(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/teams/"+string(f.teamPayments), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["urn"] != string(f.teamPayments) {
		t.Fatalf("urn=%v", body["urn"])
	}
	if int(body["squad_count"].(float64)) != 2 || int(body["person_count"].(float64)) != 2 {
		t.Fatalf("counts=%v %v", body["squad_count"], body["person_count"])
	}
	squads, _ := body["squads"].([]any)
	if len(squads) != 2 {
		t.Fatalf("squads=%v", squads)
	}
	// Determinismo: ordenadas por URN.
	if !(squads[0].(string) < squads[1].(string)) {
		t.Fatalf("squads not sorted: %v", squads)
	}
}

func TestGetTeam_NotFound_404(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/teams/urn:ce:org:acme:team/ghost", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetTeam_WrongKind_400(t *testing.T) {
	// Endereçar uma URN que existe mas é de outro kind (squad) sob a
	// rota /v1/teams/ — esperamos 400 com mensagem clara.
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/teams/"+string(f.squadCheck), nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Team") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}
