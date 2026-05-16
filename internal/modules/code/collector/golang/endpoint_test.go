package golang

import (
	"path/filepath"
	"sort"
	"testing"

	"costEngine/internal/entity/node"
)

func TestExtractEndpoints_NetHttp(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "api", "routes.go"), `package api
import "net/http"
func register() {
	http.HandleFunc("/v1/users", listUsers)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/users/{id}", getUser)
	mux.Handle("/static/", staticHandler())
}
func listUsers(w http.ResponseWriter, r *http.Request) {}
func getUser(w http.ResponseWriter, r *http.Request) {}
func staticHandler() http.Handler { return nil }
`)

	got, err := ExtractEndpoints(root, root, ".",
		node.NewServiceURN("ex", "."), "ex", nil, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	pairs := []string{}
	for _, e := range got {
		pairs = append(pairs, e.Method+" "+e.Route)
	}
	sort.Strings(pairs)
	want := []string{"ANY /static/", "ANY /v1/users", "ANY /v1/users/{id}"}
	if len(pairs) != len(want) {
		t.Fatalf("got=%v want=%v", pairs, want)
	}
	for i, w := range want {
		if pairs[i] != w {
			t.Errorf("[%d] %q want %q", i, pairs[i], w)
		}
	}

	for _, e := range got {
		if e.Framework != "net/http" {
			t.Errorf("Framework=%q for %s", e.Framework, e.Route)
		}
		if e.ServiceURN == "" {
			t.Error("ServiceURN empty")
		}
	}
}

func TestExtractEndpoints_Chi(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "api", "chi.go"), `package api
func register(r interface{}) {
	r.Get("/users", listUsers)
	r.Post("/users", createUser)
	r.Put("/users/{id}", updateUser)
	r.Delete("/users/{id}", deleteUser)
	r.Patch("/users/{id}", patchUser)
	r.Method("PROPFIND", "/dav", davHandler)
	r.MethodFunc("REPORT", "/v1/r", reportHandler)
}
func listUsers() {}
func createUser() {}
func updateUser() {}
func deleteUser() {}
func patchUser() {}
func davHandler() {}
func reportHandler() {}
`)

	got, err := ExtractEndpoints(root, root, ".",
		node.NewServiceURN("ex", "."), "ex", nil, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	pairs := []string{}
	for _, e := range got {
		pairs = append(pairs, e.Method+" "+e.Route+" ["+e.Framework+"]")
	}
	sort.Strings(pairs)
	want := []string{
		"DELETE /users/{id} [chi]",
		"GET /users [chi]",
		"PATCH /users/{id} [chi]",
		"POST /users [chi]",
		"PROPFIND /dav [chi]",
		"PUT /users/{id} [chi]",
		"REPORT /v1/r [chi]",
	}
	if len(pairs) != len(want) {
		t.Fatalf("got=%v want=%v", pairs, want)
	}
	for i, w := range want {
		if pairs[i] != w {
			t.Errorf("[%d] %q want %q", i, pairs[i], w)
		}
	}
}

func TestExtractEndpoints_IgnoresDynamicRoute(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "api", "dyn.go"), `package api
import "net/http"
func register() {
	for _, p := range []string{"/a", "/b"} {
		http.HandleFunc(p, h)
	}
}
func h(w http.ResponseWriter, r *http.Request) {}
`)
	got, err := ExtractEndpoints(root, root, ".",
		node.NewServiceURN("ex", "."), "ex", nil, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %+v", got)
	}
}

func TestExtractEndpoints_URNDeterministic(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "api", "r.go"), `package api
func register(r interface{}) {
	r.Get("/x/{id}", h)
}
func h() {}
`)
	a, _ := ExtractEndpoints(root, root, ".",
		node.NewServiceURN("ex", "."), "ex", nil, EmitOptions{Repo: "ex"})
	b, _ := ExtractEndpoints(root, root, ".",
		node.NewServiceURN("ex", "."), "ex", nil, EmitOptions{Repo: "ex"})
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("a=%d b=%d", len(a), len(b))
	}
	if a[0].URN() != b[0].URN() {
		t.Errorf("URN diverge: %q vs %q", a[0].URN(), b[0].URN())
	}
	wantURN := node.URN("urn:ce:code:ex:endpoint/.!GET:/x/{id}")
	if a[0].URN() != wantURN {
		t.Errorf("URN=%q want %q", a[0].URN(), wantURN)
	}
}
