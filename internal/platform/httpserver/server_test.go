package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"costEngine/internal/core/errs"
	"costEngine/internal/platform/httpserver"
)

// noopRegistrar registra um endpoint de eco do tenant para validar a
// cadeia inteira (RequestID + Auth chegam até o handler).
type noopRegistrar struct{}

func (noopRegistrar) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /echo", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"tenant":     httpserver.TenantIDFrom(r.Context()),
			"request_id": httpserver.RequestIDFrom(r.Context()),
		})
	})

	mux.HandleFunc("GET /boom", func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	mux.HandleFunc("GET /typed-err", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteError(w, r, &fakeDomainErr{
			urn: "urn:ce:foo",
		})
	})
}

type fakeDomainErr struct{ urn string }

func (e *fakeDomainErr) Error() string           { return "team not found: " + e.urn }
func (e *fakeDomainErr) HTTPStatus() int         { return http.StatusNotFound }
func (e *fakeDomainErr) Code() string            { return "org.team.not_found" }
func (e *fakeDomainErr) Title() string           { return "Team not found" }
func (e *fakeDomainErr) Details() map[string]any { return map[string]any{"urn": e.urn} }

func newServer(t *testing.T, tenantRequired bool) *httpserver.Server {
	t.Helper()
	return httpserver.New(httpserver.Config{TenantRequired: tenantRequired}, noopRegistrar{})
}

func do(t *testing.T, s *httpserver.Server, method, path string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec.Result()
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %s\n%s", err, b)
	}
	return m
}

func TestServer_Healthz(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/healthz", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := decode(t, resp)
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

func TestServer_NotFound_ProblemJSON(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/nope", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != errs.ContentType {
		t.Errorf("Content-Type = %q, want %q", ct, errs.ContentType)
	}
	body := decode(t, resp)
	if body["code"] != "not_found" {
		t.Errorf("code = %v", body)
	}
	if body["status"] != float64(404) {
		t.Errorf("status field = %v", body)
	}
}

func TestServer_DefaultTenantInjected(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/echo", nil)
	body := decode(t, resp)
	if body["tenant"] != httpserver.DefaultTenant {
		t.Errorf("tenant = %v", body)
	}
	if body["request_id"] == "" {
		t.Errorf("request_id empty")
	}
}

func TestServer_TenantRequired_401(t *testing.T) {
	s := newServer(t, true)
	resp := do(t, s, "GET", "/echo", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := decode(t, resp)
	if body["code"] != "unauthorized" {
		t.Errorf("code = %v", body)
	}
}

func TestServer_TenantRequired_OKWhenHeaderPresent(t *testing.T) {
	s := newServer(t, true)
	resp := do(t, s, "GET", "/echo", map[string]string{httpserver.HeaderTenantID: "acme"})
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := decode(t, resp)
	if body["tenant"] != "acme" {
		t.Errorf("tenant = %v", body)
	}
}

func TestServer_RequestIDEchoed(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/echo", map[string]string{httpserver.HeaderRequestID: "abc123"})
	if got := resp.Header.Get(httpserver.HeaderRequestID); got != "abc123" {
		t.Errorf("X-Request-ID = %q", got)
	}
}

func TestServer_RequestIDRejectsInvalid(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/echo", map[string]string{httpserver.HeaderRequestID: "with spaces!"})
	got := resp.Header.Get(httpserver.HeaderRequestID)
	if got == "with spaces!" {
		t.Errorf("invalid X-Request-ID accepted: %q", got)
	}
	if got == "" {
		t.Errorf("X-Request-ID not set after fallback")
	}
}

func TestServer_PanicRecovered(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/boom", nil)
	if resp.StatusCode != 500 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := decode(t, resp)
	if body["code"] != "internal" {
		t.Errorf("code = %v", body)
	}
	// Stack jamais vai pro body.
	for k, v := range body {
		if s, ok := v.(string); ok && strings.Contains(s, "goroutine") {
			t.Errorf("stack leaked in body.%s: %s", k, s)
		}
	}
}

func TestServer_TypedErrorRenderedAsProblem(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/typed-err", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != errs.ContentType {
		t.Errorf("Content-Type = %q", ct)
	}
	body := decode(t, resp)
	if body["code"] != "org.team.not_found" {
		t.Errorf("code = %v", body)
	}
	if body["urn"] != "urn:ce:foo" {
		t.Errorf("urn extra not flattened: %v", body)
	}
	if body["title"] != "Team not found" {
		t.Errorf("title = %v", body)
	}
}

func TestServer_TraceIDInProblemBody(t *testing.T) {
	s := newServer(t, false)
	resp := do(t, s, "GET", "/nope", map[string]string{httpserver.HeaderRequestID: "abc-1234"})
	body := decode(t, resp)
	if body["trace_id"] != "abc-1234" {
		t.Errorf("trace_id = %v", body)
	}
}

func TestCursor_Roundtrip(t *testing.T) {
	fp := httpserver.FingerprintSearch("foo", "Service")
	encoded := httpserver.EncodeCursor(httpserver.Cursor{Offset: 50, Fingerprint: fp})
	if encoded == "" {
		t.Fatal("empty cursor")
	}
	c, err := httpserver.DecodeCursor(encoded, fp)
	if err != nil {
		t.Fatal(err)
	}
	if c.Offset != 50 {
		t.Errorf("offset = %d", c.Offset)
	}
}

func TestCursor_Offset0EmptyEncoding(t *testing.T) {
	if got := httpserver.EncodeCursor(httpserver.Cursor{Offset: 0, Fingerprint: "x"}); got != "" {
		t.Errorf("offset 0 should encode to empty, got %q", got)
	}
}

func TestCursor_RejectsFingerprintMismatch(t *testing.T) {
	fp1 := httpserver.FingerprintSearch("foo", "")
	fp2 := httpserver.FingerprintSearch("bar", "")
	encoded := httpserver.EncodeCursor(httpserver.Cursor{Offset: 10, Fingerprint: fp1})
	if _, err := httpserver.DecodeCursor(encoded, fp2); err == nil {
		t.Error("expected mismatch error")
	}
}

func TestCursor_EmptyDecodeOK(t *testing.T) {
	c, err := httpserver.DecodeCursor("", "fp")
	if err != nil {
		t.Fatal(err)
	}
	if c.Offset != 0 || c.Fingerprint != "fp" {
		t.Errorf("c = %+v", c)
	}
}

func TestCursor_GarbageRejected(t *testing.T) {
	if _, err := httpserver.DecodeCursor("not-base64!!", "x"); err == nil {
		t.Error("expected error for garbage")
	}
}

func TestErrorf_IsRendered(t *testing.T) {
	err := httpserver.Errorf(http.StatusTeapot, "im_a_teapot", "short and stout")
	status, p := errs.Render(err)
	if status != http.StatusTeapot {
		t.Errorf("status = %d", status)
	}
	if p.Code != "im_a_teapot" {
		t.Errorf("code = %q", p.Code)
	}
	if p.Detail != "short and stout" {
		t.Errorf("detail = %q", p.Detail)
	}
}
