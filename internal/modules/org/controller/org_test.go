package controller_test

import (
	"net/http"
	"testing"
)

// TestOrgRoutes_MissingURN exercita o branch de URN ausente em
// dispatchTeam/dispatchSquad/dispatchPerson (rest == "").
func TestOrgRoutes_MissingURN(t *testing.T) {
	h, _ := fixture(t)
	for _, path := range []string{"/v1/teams/", "/v1/squads/", "/v1/people/"} {
		rr := doReq(t, h, "GET", path, nil)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d want=400", path, rr.Code)
		}
	}
}
