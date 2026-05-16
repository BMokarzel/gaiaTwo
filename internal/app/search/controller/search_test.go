package controller_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"costEngine/internal/app/search"
	searchctrl "costEngine/internal/app/search/controller"
	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// repoSearchAdapter espelha o adapter de cmd/api/main.go — duplicado
// no test porque a regra "search não importa repository" só vale em
// build de produção. Em teste, conveniencia.
type repoSearchAdapter struct{ r repository.NodeRepository }

func (a repoSearchAdapter) Search(ctx context.Context, q search.SearchQuery) ([]node.Node, error) {
	return a.r.Search(ctx, repository.SearchQuery{
		Q: q.Q, Kind: q.Kind, Limit: q.Limit, Offset: q.Offset,
	})
}

const defaultLimitForTest = 50

func fixture(t *testing.T) (http.Handler, *memory.Repo) {
	t.Helper()
	r := memory.New().WithClock(func() time.Time {
		return time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	})
	ctrl := searchctrl.New(repoSearchAdapter{r: r})
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

func mkService(urn, repo, modPath string, vfrom time.Time) node.Service {
	return node.Service{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vfrom, ObservedAt: vfrom, Confidence: 1},
		},
		Repo:       repo,
		ModulePath: modPath,
		Language:   "go",
	}
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

func TestSearch_RequiresQ(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET", "/v1/architecture/search", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestSearch_MatchesByQ_AndKindFilter(t *testing.T) {
	h, r := fixture(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = r.Upsert(t.Context(), mkService("urn:ce:code:payments:service/.", "payments", ".", now))
	_ = r.Upsert(t.Context(), mkService("urn:ce:code:other:service/billing", "other", "billing", now))
	_ = r.Upsert(t.Context(), mkCompute("urn:ce:aws:111:compute/i-payments", "i-payments", now))

	rr := doReq(t, h, "GET", "/v1/architecture/search?q=payments", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Results []map[string]any `json:"results"`
		Limit   int              `json:"limit"`
		Offset  int              `json:"offset"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(body.Results))
	}
	if body.Limit != defaultLimitForTest || body.Offset != 0 {
		t.Fatalf("paging echo wrong: %+v", body)
	}

	rr = doReq(t, h, "GET", "/v1/architecture/search?q=payments&kind=service", nil)
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Results) != 1 || body.Results[0]["kind"] != "service" {
		t.Fatalf("kind filter: %+v", body)
	}
}

func TestSearch_PaginatesViaCursor(t *testing.T) {
	h, r := fixture(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, slug := range []string{"a", "b", "c", "d", "e"} {
		_ = r.Upsert(t.Context(),
			mkService("urn:ce:code:"+slug+":service/.", "shared", ".", now))
	}

	rr := doReq(t, h, "GET",
		"/v1/architecture/search?q=shared&limit=2", nil)
	var p1 struct {
		Results    []map[string]any `json:"results"`
		NextCursor string           `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&p1)
	if len(p1.Results) != 2 || p1.NextCursor == "" {
		t.Fatalf("p1: %+v", p1)
	}
	if p1.Results[0]["urn"] != "urn:ce:code:a:service/." {
		t.Fatalf("p1 order: %v", p1.Results[0])
	}

	rr = doReq(t, h, "GET",
		"/v1/architecture/search?q=shared&limit=2&cursor="+p1.NextCursor, nil)
	var p2 struct {
		Results    []map[string]any `json:"results"`
		NextCursor string           `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&p2)
	if len(p2.Results) != 2 {
		t.Fatalf("p2 size: %d", len(p2.Results))
	}
	if p2.Results[0]["urn"] != "urn:ce:code:c:service/." {
		t.Fatalf("p2 start: %v", p2.Results[0])
	}

	rr = doReq(t, h, "GET",
		"/v1/architecture/search?q=shared&limit=2&cursor="+p2.NextCursor, nil)
	var p3 struct {
		Results    []map[string]any `json:"results"`
		NextCursor string           `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&p3)
	if len(p3.Results) != 1 || p3.NextCursor != "" {
		t.Fatalf("p3: %+v", p3)
	}
}

func TestSearch_CursorFingerprint_RejectsQueryChange(t *testing.T) {
	h, r := fixture(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, slug := range []string{"a", "b", "c"} {
		_ = r.Upsert(t.Context(),
			mkService("urn:ce:code:"+slug+":service/.", "shared", ".", now))
	}
	rr := doReq(t, h, "GET",
		"/v1/architecture/search?q=shared&limit=2", nil)
	var p1 struct {
		NextCursor string `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&p1)

	rr = doReq(t, h, "GET",
		"/v1/architecture/search?q=different&limit=2&cursor="+p1.NextCursor, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSearch_BadCursor_Returns400(t *testing.T) {
	h, _ := fixture(t)
	rr := doReq(t, h, "GET",
		"/v1/architecture/search?q=x&cursor=not-base64!!", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
	_ = decodeProblem(t, rr.Body)
}

func TestNew_NilSearcherPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for nil searcher")
		}
	}()
	searchctrl.New(nil)
}
