package openapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

const collectorSpec = `openapi: 3.0.0
info:
  title: T
  version: 1.0.0
paths:
  /a:
    get:
      operationId: getA
      responses:
        "200": { description: ok }
  /b:
    post:
      operationId: createB
      requestBody:
        content:
          application/json:
            schema: { type: object }
      responses:
        "201": { description: created }
`

const collectorSpecPlusPath = collectorSpec + `  /c:
    delete:
      operationId: dropC
      responses:
        "204": { description: gone }
`

func TestCollect_ProducesEndpointsAndEdges(t *testing.T) {
	svc := node.NewServiceURN("acme/payments", ".")
	res, err := CollectFromReader(strings.NewReader(collectorSpec), Config{
		ServiceURN: svc, RunID: "test", ObservedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(res.Endpoints) != 2 {
		t.Fatalf("eps=%d want=2", len(res.Endpoints))
	}
	if len(res.Edges) != 2 {
		t.Fatalf("edges=%d want=2", len(res.Edges))
	}
	if res.Endpoints[0].Framework != "openapi" {
		t.Fatalf("framework=%q", res.Endpoints[0].Framework)
	}
	if res.Endpoints[0].ServiceURN != svc {
		t.Fatalf("svc urn=%q", res.Endpoints[0].ServiceURN)
	}
	if res.Endpoints[0].Handler != "getA" {
		t.Fatalf("handler=%q", res.Endpoints[0].Handler)
	}
	// Edge ID determinístico.
	if res.Edges[0].FromURN != res.Endpoints[0].URN() || res.Edges[0].ToURN != svc {
		t.Fatalf("edge=%+v", res.Edges[0])
	}
}

func TestCollect_FallbackHandlerWhenNoOperationID(t *testing.T) {
	spec := `openapi: 3.0.0
info: { title: T, version: 1 }
paths:
  /noop:
    get:
      responses:
        "200": { description: ok }
`
	res, err := CollectFromReader(strings.NewReader(spec), Config{
		ServiceURN: node.NewServiceURN("r", "."),
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !strings.HasPrefix(res.Endpoints[0].Handler, "openapi:") {
		t.Fatalf("expected fallback handler, got %q", res.Endpoints[0].Handler)
	}
}

func TestCollect_RejectsNonServiceURN(t *testing.T) {
	_, err := CollectFromReader(strings.NewReader(collectorSpec), Config{
		ServiceURN: node.NewURN(node.ProviderCode, "r", node.KindFunction, "foo"),
	})
	if err == nil || !strings.Contains(err.Error(), "kind must be") {
		t.Fatalf("err=%v", err)
	}
}

func TestApply_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()

	// Pré-cadastra Service.
	svc := node.Service{
		Base: node.Base{
			NodeURN:  node.NewServiceURN("acme/payments", "."),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: time.Now().UTC(), ObservedAt: time.Now().UTC(), Confidence: 1},
		},
		Repo: "acme/payments", ModulePath: ".", Language: "go",
	}
	if err := repo.Upsert(ctx, svc); err != nil {
		t.Fatalf("seed service: %v", err)
	}

	cfg := Config{ServiceURN: svc.URN(), RunID: "r1", ObservedAt: time.Unix(1700000000, 0).UTC()}
	res, err := CollectFromReader(strings.NewReader(collectorSpec), cfg)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	st, err := w.Apply(ctx, res, svc.URN())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Endpoints != 2 || st.Edges != 2 {
		t.Fatalf("stats=%+v", st)
	}

	// Re-apply: mesmas URNs, mesmo ContentHash → repositório bitemporal
	// não cria versão nova (idempotência).
	res2, _ := CollectFromReader(strings.NewReader(collectorSpec), cfg)
	if _, err := w.Apply(ctx, res2, svc.URN()); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	// Endpoint corrente ainda existe.
	got, err := repo.GetByURN(ctx, res.Endpoints[0].URN(), repository.AsOf{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.URN() != res.Endpoints[0].URN() {
		t.Fatalf("urn=%v", got.URN())
	}
}

func TestApply_AddPathCreatesNew(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	svc := node.Service{
		Base: node.Base{
			NodeURN:  node.NewServiceURN("acme/payments", "."),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: time.Now().UTC(), ObservedAt: time.Now().UTC(), Confidence: 1},
		},
		Repo: "acme/payments", ModulePath: ".", Language: "go",
	}
	if err := repo.Upsert(ctx, svc); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}

	cfg := Config{ServiceURN: svc.URN(), ObservedAt: time.Unix(1700000000, 0).UTC()}
	r1, _ := CollectFromReader(strings.NewReader(collectorSpec), cfg)
	if _, err := w.Apply(ctx, r1, svc.URN()); err != nil {
		t.Fatalf("apply r1: %v", err)
	}

	cfg.ObservedAt = time.Unix(1700001000, 0).UTC()
	r2, _ := CollectFromReader(strings.NewReader(collectorSpecPlusPath), cfg)
	if len(r2.Endpoints) != 3 {
		t.Fatalf("r2 endpoints=%d", len(r2.Endpoints))
	}
	if _, err := w.Apply(ctx, r2, svc.URN()); err != nil {
		t.Fatalf("apply r2: %v", err)
	}
}

func TestApply_ServiceMissing(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	svc := node.NewServiceURN("ghost", ".")
	res, err := CollectFromReader(strings.NewReader(collectorSpec), Config{ServiceURN: svc})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	_, err = w.Apply(ctx, res, svc)
	if err == nil {
		t.Fatalf("expected error for missing service")
	}
}
