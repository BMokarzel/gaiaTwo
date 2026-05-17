package github_webhook

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

func seed(t *testing.T) (*memory.Repo, repository.EdgeRepository) {
	t.Helper()
	repo := memory.New()
	ctx := context.Background()

	feat := node.Feature{
		Base: node.Base{
			NodeURN:  node.NewFeatureURN("acme", "checkout-redesign"),
			NodeKind: node.KindFeature,
			NodeMeta: node.Meta{ValidFrom: time.Now().Add(-time.Hour), Confidence: 1},
		},
		Tenant:  "acme",
		ShortID: "checkout-redesign",
		Name:    "Checkout Redesign",
	}
	if err := repo.Upsert(ctx, feat); err != nil {
		t.Fatalf("seed feature: %v", err)
	}

	for _, mp := range []string{".", "services/billing"} {
		s := node.Service{
			Base: node.Base{
				NodeURN:  node.NewServiceURN("acme/api", mp),
				NodeKind: node.KindService,
				NodeMeta: node.Meta{ValidFrom: time.Now().Add(-time.Hour), Confidence: 1},
			},
			Repo:       "acme/api",
			ModulePath: mp,
		}
		if err := repo.Upsert(ctx, s); err != nil {
			t.Fatalf("seed service %s: %v", mp, err)
		}
	}
	return repo, repo.AsEdgeRepo()
}

func TestService_Ingest_HappyAndIdempotent(t *testing.T) {
	nodes, edges := seed(t)
	svc := New(nodes, edges)
	ctx := context.Background()

	payload := &Payload{
		Action:      "closed",
		Merged:      true,
		MergedAt:    time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		HTMLURL:     "https://github.com/acme/api/pull/42",
		FeatureURNs: []node.URN{node.NewFeatureURN("acme", "checkout-redesign")},
		Files:       []string{"services/billing/charge.go", "cmd/api/main.go"},
		Repo:        "acme/api",
	}

	stats, err := svc.Ingest(ctx, payload)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	// 2 services touched (billing + root), 1 feature → 2 edges.
	if stats.EdgesUpserted != 2 || stats.ResolvedServices != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// Same payload again — idempotent: nothing new, edges count via Neighbors.
	if _, err := svc.Ingest(ctx, payload); err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	es, err := edges.Neighbors(ctx, payload.FeatureURNs[0], repository.DirOut, repository.EdgeFilter{
		Types: []edge.Type{edge.TypeRealizes},
	})
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if len(es) != 2 {
		t.Fatalf("expected 2 Realizes edges after duplicate ingest, got %d", len(es))
	}
	// Audit fields present.
	for _, e := range es {
		props := e.Meta().Properties
		if props["pr_url"] != payload.HTMLURL {
			t.Errorf("missing pr_url in edge %s", e.ID())
		}
		if props["merged_at"] == nil {
			t.Errorf("missing merged_at in edge %s", e.ID())
		}
	}
}

func TestService_Ingest_FeatureUnknown(t *testing.T) {
	nodes, edges := seed(t)
	svc := New(nodes, edges)
	payload := &Payload{
		Action:      "closed",
		Merged:      true,
		MergedAt:    time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		HTMLURL:     "https://github.com/acme/api/pull/42",
		FeatureURNs: []node.URN{node.NewFeatureURN("acme", "ghost-feature")},
		Files:       []string{"services/billing/x.go"},
		Repo:        "acme/api",
	}
	_, err := svc.Ingest(context.Background(), payload)
	if !errors.Is(err, ErrFeatureNotFound) {
		t.Fatalf("expected ErrFeatureNotFound, got %v", err)
	}
}

func TestService_Ingest_NoServicesTouched(t *testing.T) {
	nodes, edges := seed(t)
	// Drop root service so no fallback exists; only billing remains.
	if err := nodes.Delete(context.Background(), node.NewServiceURN("acme/api", ".")); err != nil {
		t.Fatalf("delete root service: %v", err)
	}
	svc := New(nodes, edges)
	payload := &Payload{
		Action:      "closed",
		Merged:      true,
		MergedAt:    time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		FeatureURNs: []node.URN{node.NewFeatureURN("acme", "checkout-redesign")},
		Files:       []string{"docs/README.md"},
		Repo:        "acme/api",
	}
	stats, err := svc.Ingest(context.Background(), payload)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if stats.EdgesUpserted != 0 || stats.Reason == "" {
		t.Fatalf("expected no-op stats, got %+v", stats)
	}
}

func TestController_Handle_BadHMAC(t *testing.T) {
	nodes, edges := seed(t)
	c := NewController(New(nodes, edges), "topsecret")

	body := []byte(`{"action":"closed"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	req.Header.Set("X-GitHub-Event", "pull_request")
	rec := httptest.NewRecorder()
	c.handle(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestController_Handle_HappyEnd2End(t *testing.T) {
	nodes, edges := seed(t)
	secret := "topsecret"
	c := NewController(New(nodes, edges), secret)

	body := []byte(samplePR)
	// samplePR references feature checkout-redesign + pricing-v2 in tenant acme.
	// Seed only checkout-redesign — pricing-v2 must fail with ErrFeatureNotFound,
	// which surfaces as 400. To make the happy path clean, seed pricing-v2 too.
	pv2 := node.Feature{
		Base: node.Base{
			NodeURN:  node.NewFeatureURN("acme", "pricing-v2"),
			NodeKind: node.KindFeature,
			NodeMeta: node.Meta{ValidFrom: time.Now().Add(-time.Hour), Confidence: 1},
		},
		Tenant: "acme", ShortID: "pricing-v2", Name: "Pricing v2",
	}
	if err := nodes.Upsert(context.Background(), pv2); err != nil {
		t.Fatalf("seed pv2: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, secret))
	req.Header.Set("X-GitHub-Event", "pull_request")
	rec := httptest.NewRecorder()
	c.handle(rec, req)
	if rec.Code != http.StatusOK {
		out, _ := io.ReadAll(rec.Body)
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, out)
	}
}

func TestController_Handle_IgnoredEvent(t *testing.T) {
	nodes, edges := seed(t)
	secret := "s"
	c := NewController(New(nodes, edges), secret)
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, secret))
	req.Header.Set("X-GitHub-Event", "ping")
	rec := httptest.NewRecorder()
	c.handle(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
