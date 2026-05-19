package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

// TestListNodes_FilterByKind cria 2 services + 1 module e valida que
// GET /v1/architecture/nodes?kind=service retorna apenas os services.
func TestListNodes_FilterByKind(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	mk := func(repo string) node.Service {
		return node.Service{
			Base: node.Base{
				NodeURN:  node.NewServiceURN(repo, "."),
				NodeKind: node.KindService,
				NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
			},
			Repo: repo, ModulePath: ".",
		}
	}
	svc1, svc2 := mk("acme"), mk("foo")
	mod := node.Module{
		Base: node.Base{
			NodeURN:  node.NewModuleURN("acme", ".", "src"),
			NodeKind: node.KindModule,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		ServiceURN: svc1.URN(), Namespace: "src", Path: "src",
	}
	for _, n := range []node.Node{svc1, svc2, mod} {
		if err := r.Upsert(ctx, n); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	rr := doReq(t, h, "GET", "/v1/architecture/nodes?kind=service", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Results []map[string]any `json:"results"`
		Limit   int              `json:"limit"`
		Offset  int              `json:"offset"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != 2 {
		t.Fatalf("count = %d want 2 (services only); results=%v", body.Count, body.Results)
	}
	if body.Limit != 100 || body.Offset != 0 {
		t.Fatalf("limit/offset = %d/%d want 100/0", body.Limit, body.Offset)
	}
}

// TestListNodes_LimitClamps confirma que limit > 500 silenciosamente
// é reduzido para 500 (comportamento de parseLimit).
func TestListNodes_LimitClamps(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/architecture/nodes?limit=99999", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Limit int `json:"limit"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Limit != 500 {
		t.Fatalf("limit = %d want 500 (cap)", body.Limit)
	}
}

func TestListNodes_InvalidLimit(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/architecture/nodes?limit=-1", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
}
