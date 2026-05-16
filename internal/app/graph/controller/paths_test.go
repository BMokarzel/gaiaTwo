package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

func TestPaths_RequiresFromAndTo(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/architecture/paths?from=x", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestPaths_HopsCap(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/architecture/paths?from=a&to=b&max_hops=99", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestPaths_ReturnsChain(t *testing.T) {
	h, r := fixture(t)
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := node.URN("urn:ce:aws:111:compute/a")
	b := node.URN("urn:ce:aws:111:compute/b")
	c := node.URN("urn:ce:aws:111:compute/c")
	for _, u := range []node.URN{a, b, c} {
		_ = r.Upsert(ctx, mkCompute(string(u), string(u), now))
	}
	_ = er.Upsert(ctx, mkDeployed(a, b, now), node.KindCompute, node.KindCompute)
	_ = er.Upsert(ctx, mkDeployed(b, c, now), node.KindCompute, node.KindCompute)

	rr := doReq(t, h, "GET",
		"/v1/architecture/paths?from="+string(a)+"&to="+string(c)+"&max_hops=3", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Paths []struct {
			Hops int      `json:"hops"`
			URNs []string `json:"urns"`
		} `json:"paths"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Paths) != 1 || body.Paths[0].Hops != 2 || len(body.Paths[0].URNs) != 3 {
		t.Fatalf("path: %+v", body.Paths)
	}
	if body.Paths[0].URNs[0] != string(a) || body.Paths[0].URNs[2] != string(c) {
		t.Fatalf("wrong endpoints: %v", body.Paths[0].URNs)
	}
}

func TestPaths_NoPath_EmptyArray(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := node.URN("urn:ce:aws:111:compute/a")
	b := node.URN("urn:ce:aws:111:compute/b")
	for _, u := range []node.URN{a, b} {
		_ = r.Upsert(ctx, mkCompute(string(u), string(u), now))
	}
	rr := doReq(t, h, "GET",
		"/v1/architecture/paths?from="+string(a)+"&to="+string(b), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Paths []any `json:"paths"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Paths) != 0 {
		t.Fatalf("expected empty, got %v", body.Paths)
	}
}
