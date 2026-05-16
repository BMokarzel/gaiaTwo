package controller_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSquadMembers_ReturnsMaskedEmails(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET",
		"/v1/squads/"+string(f.squadCheck)+"/members", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	results := body["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("len=%d want=2", len(results))
	}
	for _, raw := range results {
		m := raw.(map[string]any)
		email := m["email"].(string)
		if !strings.Contains(email, "***@") {
			t.Fatalf("email não-mascarado: %q", email)
		}
		if strings.Contains(email, "alice@") || strings.Contains(email, "bob@") {
			t.Fatalf("email pleno vazou: %q", email)
		}
	}
}

func TestSquadMembers_EmptySquad(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET",
		"/v1/squads/"+string(f.squadGrowth)+"/members", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body["results"].([]any)) != 0 {
		t.Fatalf("expected 0 members for empty squad")
	}
}

func TestSquadMembers_SquadNotFound_404(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/squads/urn:ce:org:acme:squad/ghost/members", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetPerson_ReturnsMaskedEmailAndTeamURN(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/people/"+string(f.personAlice), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["urn"] != string(f.personAlice) {
		t.Fatalf("urn=%v", body["urn"])
	}
	email := body["email"].(string)
	if !strings.Contains(email, "***@") {
		t.Fatalf("email not masked: %q", email)
	}
	if strings.Contains(email, "alice@") {
		t.Fatalf("email pleno vazou: %q", email)
	}
	if body["squad_urn"] != string(f.squadCheck) {
		t.Fatalf("squad_urn=%v", body["squad_urn"])
	}
	if body["team_urn"] != string(f.teamPayments) {
		t.Fatalf("team_urn=%v want=%v", body["team_urn"], f.teamPayments)
	}
	if body["active"] != true {
		t.Fatalf("active=%v want=true", body["active"])
	}
}

func TestGetPerson_NotFound_404(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/people/urn:ce:org:acme:person/deadbeef", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetPerson_WrongKind_400(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	rr := doReq(t, h, "GET", "/v1/people/"+string(f.teamPayments), nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}
