package controller_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTeamSquads_ReturnsSquadsWithCounts(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/teams/"+string(f.teamPayments)+"/squads", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	results := body["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("len=%d want=2", len(results))
	}
	byURN := map[string]map[string]any{}
	for _, raw := range results {
		m := raw.(map[string]any)
		byURN[m["urn"].(string)] = m
	}
	check := byURN[string(f.squadCheck)]
	if check == nil || int(check["member_count"].(float64)) != 2 {
		t.Fatalf("checkout member_count=%v", check["member_count"])
	}
	growth := byURN[string(f.squadGrowth)]
	if growth == nil || int(growth["member_count"].(float64)) != 0 {
		t.Fatalf("growth member_count=%v", growth["member_count"])
	}
	if check["team_urn"] != string(f.teamPayments) {
		t.Fatalf("team_urn=%v", check["team_urn"])
	}
}

func TestTeamSquads_TeamNotFound_404(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/teams/urn:ce:org:acme:team/ghost/squads", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestTeamSquads_Pagination_Cursor(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET",
		"/v1/teams/"+string(f.teamPayments)+"/squads?limit=1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("p1 status=%d", rr.Code)
	}
	var p1 map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&p1)
	if len(p1["results"].([]any)) != 1 {
		t.Fatalf("p1 len=%d", len(p1["results"].([]any)))
	}
	next := p1["next_cursor"].(string)
	if next == "" {
		t.Fatalf("p1 missing cursor")
	}
	rr2 := doReq(t, h, "GET",
		"/v1/teams/"+string(f.teamPayments)+"/squads?limit=1&cursor="+next, nil)
	var p2 map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&p2)
	if len(p2["results"].([]any)) != 1 {
		t.Fatalf("p2 len=%d", len(p2["results"].([]any)))
	}
	if p2["next_cursor"].(string) != "" {
		t.Fatalf("p2 next=%q", p2["next_cursor"])
	}
}

func TestGetSquad_Detail(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/squads/"+string(f.squadCheck), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["urn"] != string(f.squadCheck) {
		t.Fatalf("urn=%v", body["urn"])
	}
	if int(body["member_count"].(float64)) != 2 {
		t.Fatalf("member_count=%v", body["member_count"])
	}
	if body["team_urn"] != string(f.teamPayments) {
		t.Fatalf("team_urn=%v", body["team_urn"])
	}
}

func TestGetSquad_WrongKind_400(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/squads/"+string(f.teamPayments), nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Squad") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestGetSquad_NotFound_404(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/squads/urn:ce:org:acme:squad/ghost", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}
