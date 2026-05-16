package controller_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"costEngine/internal/app/graph/controller"
	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository/memory"
)

// fixture monta um Controller graph com memory.Repo determinístico
// (clock fixo em 2026-05-14 12:00 UTC). Devolve o handler completo
// (com middlewares) e o Repo para os testes injetarem dados via
// Upsert direto.
func fixture(t *testing.T) (http.Handler, *memory.Repo) {
	t.Helper()
	r := memory.New().WithClock(func() time.Time {
		return time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	})
	ctrl := controller.New(r, r.AsEdgeRepo())
	hs := httpserver.New(httpserver.Config{}, ctrl)
	return hs.Routes(), r
}

func doReq(t *testing.T, h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeProblem(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

func mkCompute(urn, ext string, vfrom time.Time) node.Compute {
	return node.Compute{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vfrom, ObservedAt: vfrom, Confidence: 1},
		},
		ProviderID: node.ProviderAWS,
		Account:    "urn:ce:aws:111:account/111",
		Region:     "urn:ce:aws:111:region/us-east-1",
		ExtID:      ext,
		Flavor:     node.ComputeVM,
	}
}

func mkDeployed(from, to node.URN, vfrom time.Time) edge.DeployedOn {
	return edge.DeployedOn{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeDeployedOn, to, vfrom),
			EdgeType: edge.TypeDeployedOn,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{ValidFrom: vfrom, Directional: true},
		},
	}
}
