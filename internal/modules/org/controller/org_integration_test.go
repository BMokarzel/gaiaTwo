package controller_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestOrgHierarchy_EndToEnd percorre a hierarquia completa
// (teams → squads → members → reports) numa única corrida,
// validando determinismo, PII mascarada e consistência de URNs.
func TestOrgHierarchy_EndToEnd(t *testing.T) {
	h, r := fixture(t)
	f := newOrgFixture(t, r)
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	// Cadeia de reports: Bob → Alice.
	addReportsTo(t, r, f.personBob, f.personAlice, now)

	// 1) Lista times → encontra Payments.
	rr := doReq(t, h, "GET", "/v1/teams", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("teams list status=%d", rr.Code)
	}
	var teamsList map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&teamsList)
	teams := teamsList["results"].([]any)
	var paymentsURN string
	for _, raw := range teams {
		m := raw.(map[string]any)
		if m["urn"] == string(f.teamPayments) {
			paymentsURN = m["urn"].(string)
			if int(m["squad_count"].(float64)) != 2 {
				t.Fatalf("payments squad_count=%v", m["squad_count"])
			}
		}
	}
	if paymentsURN == "" {
		t.Fatalf("payments missing from list")
	}

	// 2) Detalhe do team com squads[].
	rr = doReq(t, h, "GET", "/v1/teams/"+paymentsURN, nil)
	var teamDetail map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&teamDetail)
	squads := teamDetail["squads"].([]any)
	if len(squads) != 2 {
		t.Fatalf("squads in detail=%d", len(squads))
	}

	// 3) Squads do team via endpoint dedicado.
	rr = doReq(t, h, "GET", "/v1/teams/"+paymentsURN+"/squads", nil)
	var sqList map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&sqList)
	sqResults := sqList["results"].([]any)
	var checkoutURN string
	for _, raw := range sqResults {
		m := raw.(map[string]any)
		if m["urn"] == string(f.squadCheck) {
			checkoutURN = m["urn"].(string)
			if m["team_urn"] != paymentsURN {
				t.Fatalf("squad team_urn=%v want=%v", m["team_urn"], paymentsURN)
			}
			if int(m["member_count"].(float64)) != 2 {
				t.Fatalf("member_count=%v", m["member_count"])
			}
		}
	}
	if checkoutURN == "" {
		t.Fatalf("checkout missing")
	}

	// 4) Members do squad — PII mascarada.
	rr = doReq(t, h, "GET", "/v1/squads/"+checkoutURN+"/members", nil)
	var mbList map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&mbList)
	mbResults := mbList["results"].([]any)
	if len(mbResults) != 2 {
		t.Fatalf("members=%d", len(mbResults))
	}
	for _, raw := range mbResults {
		m := raw.(map[string]any)
		if email := m["email"].(string); !strings.Contains(email, "***@") {
			t.Fatalf("PII vazada: %q", email)
		}
		if strings.Contains(m["email"].(string), "alice@") ||
			strings.Contains(m["email"].(string), "bob@") {
			t.Fatalf("email pleno: %v", m["email"])
		}
	}

	// 5) Person detail com team_urn transitivo.
	rr = doReq(t, h, "GET", "/v1/people/"+string(f.personAlice), nil)
	var aliceDetail map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&aliceDetail)
	if aliceDetail["team_urn"] != paymentsURN {
		t.Fatalf("alice team_urn=%v", aliceDetail["team_urn"])
	}
	if aliceDetail["active"] != true {
		t.Fatalf("alice active=%v", aliceDetail["active"])
	}

	// 6) Reports tree — Alice tem Bob como subordinado.
	rr = doReq(t, h, "GET", "/v1/people/"+string(f.personAlice)+"/reports", nil)
	var reportsTree map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&reportsTree)
	rep := reportsTree["reports"].([]any)
	if len(rep) != 1 {
		t.Fatalf("reports=%d", len(rep))
	}
	bob := rep[0].(map[string]any)["person"].(map[string]any)
	if bob["urn"] != string(f.personBob) {
		t.Fatalf("bob urn mismatch")
	}
	if reportsTree["truncated"].(bool) {
		t.Fatalf("unexpected truncated")
	}
}

// TestOrgHierarchy_CursorPagination_Stable verifica que cursores
// permanecem estáveis entre páginas e que mudar `as_of` invalida.
func TestOrgHierarchy_CursorPagination_Stable(t *testing.T) {
	h, r := fixture(t)
	newOrgFixture(t, r)

	rr := doReq(t, h, "GET", "/v1/teams?limit=1", nil)
	var p1 map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&p1)
	next := p1["next_cursor"].(string)
	if next == "" {
		t.Fatalf("p1 missing cursor")
	}

	// Mudar `as_of` entre páginas com o mesmo cursor deve invalidar.
	rr2 := doReq(t, h, "GET",
		"/v1/teams?limit=1&as_of=2026-01-01T00:00:00Z&cursor="+next, nil)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("cross-as_of cursor status=%d want=400", rr2.Code)
	}
}
