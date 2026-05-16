package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository/memory"
)

func TestGetNode_ReturnsCurrentVersion(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = r.Upsert(ctx, mkCompute("urn:ce:aws:111:compute/i-1", "i-1", now))

	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/urn:ce:aws:111:compute/i-1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["urn"] != "urn:ce:aws:111:compute/i-1" {
		t.Fatalf("urn = %v", body["urn"])
	}
	if body["kind"] != "compute" {
		t.Fatalf("kind = %v", body["kind"])
	}
}

func TestGetNode_NotFound_Returns404JSON(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/urn:ce:aws:111:compute/missing", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
	body := decodeProblem(t, rr.Body)
	if body["code"] != "not_found" {
		t.Fatalf("code = %v", body["code"])
	}
}

func TestGetNode_AsOf_BadFormat_Returns400(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/urn:ce:aws:111:compute/i?as_of=not-a-time", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestGetNode_AsOf_ReturnsHistoricalVersion(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()

	v1 := mkCompute("urn:ce:aws:111:compute/i-1", "ext-A", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	_ = r.Upsert(ctx, v1)
	// Repo dummy só para confirmar API; fixture clock avança a cada call.
	r2 := memory.New().WithClock(func() time.Time {
		return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	})
	v2 := mkCompute("urn:ce:aws:111:compute/i-1", "ext-B", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	_ = r.Upsert(ctx, v2)
	_ = r2

	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/urn:ce:aws:111:compute/i-1?as_of=2026-01-01T12:00:00Z", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	data := body["data"].(map[string]any)
	if data["external_id"] != "ext-A" {
		t.Fatalf("expected v1 (ext-A), got %v", data["external_id"])
	}
}

func TestHistory_ReturnsDescOrder(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()
	urn := "urn:ce:aws:111:compute/i-1"
	for i := 1; i <= 3; i++ {
		_ = r.Upsert(ctx, mkCompute(urn, "v", time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC)))
	}
	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/"+urn+"/history", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Versions []map[string]any `json:"versions"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Versions) != 3 {
		t.Fatalf("len = %d", len(body.Versions))
	}
	if body.Versions[0]["valid_from"].(string) < body.Versions[2]["valid_from"].(string) {
		t.Fatalf("not desc: %v", body.Versions)
	}
}

func TestHistory_LimitClamped(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()
	urn := "urn:ce:aws:111:compute/i-1"
	for i := 1; i <= 5; i++ {
		_ = r.Upsert(ctx, mkCompute(urn, "v", time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC)))
	}
	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/"+urn+"/history?limit=2", nil)
	var body struct {
		Versions []any `json:"versions"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Versions) != 2 {
		t.Fatalf("limit not applied: %d", len(body.Versions))
	}
}

func TestHistory_BadLimit_Returns400(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/urn:ce:aws:111:compute/i-1/history?limit=abc", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNeighbors_DepthOne(t *testing.T) {
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
	_ = er.Upsert(ctx, mkDeployed(a, c, now), node.KindCompute, node.KindCompute)

	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/"+string(a)+"/neighbors", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Edges []map[string]any `json:"edges"`
		Nodes []map[string]any `json:"nodes"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Edges) != 2 || len(body.Nodes) != 2 {
		t.Fatalf("edges=%d nodes=%d", len(body.Edges), len(body.Nodes))
	}
}

func TestNeighbors_DepthCapEnforced(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/urn:ce:aws:111:compute/x/neighbors?depth=99", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNeighbors_EdgeTypesFilter(t *testing.T) {
	h, r := fixture(t)
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := node.URN("urn:ce:aws:111:compute/a")
	b := node.URN("urn:ce:aws:111:compute/b")
	for _, u := range []node.URN{a, b} {
		_ = r.Upsert(ctx, mkCompute(string(u), string(u), now))
	}
	_ = er.Upsert(ctx, mkDeployed(a, b, now), node.KindCompute, node.KindCompute)

	rr := doReq(t, h, "GET",
		"/v1/architecture/nodes/"+string(a)+"/neighbors?edge_types=CONTAINS", nil)
	var body struct {
		Edges []any `json:"edges"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Edges) != 0 {
		t.Fatalf("expected 0 edges with type filter mismatch, got %d", len(body.Edges))
	}

	rr = doReq(t, h, "GET",
		"/v1/architecture/nodes/"+string(a)+"/neighbors?edge_types=UNKNOWN_TYPE", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("unknown type should not 400, got %d", rr.Code)
	}
}

func TestNeighbors_DepthTwo(t *testing.T) {
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
		"/v1/architecture/nodes/"+string(a)+"/neighbors?depth=2", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Edges []any `json:"edges"`
		Nodes []any `json:"nodes"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Edges) != 2 || len(body.Nodes) != 2 {
		t.Fatalf("depth=2: edges=%d nodes=%d", len(body.Edges), len(body.Nodes))
	}
}
