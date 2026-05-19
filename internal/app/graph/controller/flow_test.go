package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// TestFlow_EndpointReturnsConnectedSubgraph cria um pequeno grafo de
// código (Service → Module → Endpoint + Function) e valida que /flow
// retorna todos os nós alcançáveis agrupados por kind, com as
// arestas que conectam o sub-grafo.
func TestFlow_EndpointReturnsConnectedSubgraph(t *testing.T) {
	h, r := fixture(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	svc := node.Service{
		Base: node.Base{
			NodeURN:  node.NewServiceURN("acme", "."),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Repo: "acme", ModulePath: ".", Language: "typescript",
	}
	mod := node.Module{
		Base: node.Base{
			NodeURN:  node.NewModuleURN("acme", ".", "src"),
			NodeKind: node.KindModule,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		ServiceURN: svc.URN(), Namespace: "src", Path: "src", Language: "typescript",
	}
	ep := node.Endpoint{
		Base: node.Base{
			NodeURN:  node.NewEndpointURN("acme", ".", "GET", "/users/:id"),
			NodeKind: node.KindEndpoint,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		ServiceURN: svc.URN(), ModuleURN: mod.URN(),
		Method: "GET", Route: "/users/:id", Handler: "findById", Framework: "express",
	}
	fn := node.Function{
		Base: node.Base{
			NodeURN:  node.NewFunctionURN("acme", ".", "src", "findById"),
			NodeKind: node.KindFunction,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		ServiceURN: svc.URN(), ModuleURN: mod.URN(),
		Namespace: "src", Symbol: "findById",
	}

	for _, n := range []node.Node{svc, mod, ep, fn} {
		if err := r.Upsert(ctx, n); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	mkContains := func(from, to node.URN) edge.Contains {
		return edge.Contains{Base: edge.Base{
			EdgeID: edge.DeterministicID(from, edge.TypeContains, to, now),
			EdgeType: edge.TypeContains, FromURN: from, ToURN: to,
			EdgeMeta: edge.Meta{ValidFrom: now, ObservedAt: now, Directional: true, Confidence: 1},
		}}
	}
	edges := []struct {
		e        edge.Edge
		from, to node.Kind
	}{
		{mkContains(svc.URN(), mod.URN()), node.KindService, node.KindModule},
		{mkContains(mod.URN(), ep.URN()), node.KindModule, node.KindEndpoint},
		{mkContains(mod.URN(), fn.URN()), node.KindModule, node.KindFunction},
	}
	for _, x := range edges {
		if err := r.AsEdgeRepo().Upsert(ctx, x.e, x.from, x.to); err != nil {
			t.Fatalf("edge upsert: %v", err)
		}
	}

	// /flow a partir do Endpoint deve subir até Service e descer até Function.
	rr := doReq(t, h, "GET", "/v1/architecture/nodes/"+string(ep.URN())+"/flow", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Root  string              `json:"root"`
		Nodes map[string][]any    `json:"nodes"`
		Edges []map[string]any    `json:"edges"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Root != string(ep.URN()) {
		t.Fatalf("root = %q want %q", body.Root, ep.URN())
	}
	if len(body.Nodes["service"]) != 1 {
		t.Fatalf("expected 1 service, got %d", len(body.Nodes["service"]))
	}
	if len(body.Nodes["module"]) != 1 {
		t.Fatalf("expected 1 module, got %d", len(body.Nodes["module"]))
	}
	if len(body.Nodes["endpoint"]) != 1 {
		t.Fatalf("expected 1 endpoint (root), got %d", len(body.Nodes["endpoint"]))
	}
	if len(body.Nodes["function"]) != 1 {
		t.Fatalf("expected 1 function, got %d", len(body.Nodes["function"]))
	}
	if len(body.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(body.Edges))
	}
}

func TestFlow_NotFound(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/architecture/nodes/urn:ce:code:nope:endpoint/x!GET:/y/flow", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
}

func TestFlow_DepthCap(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/architecture/nodes/urn:ce:code:r:service/api/flow?depth=99", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}
