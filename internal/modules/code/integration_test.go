package code_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/code/collector/golang"
	codesvc "costEngine/internal/modules/code/service"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// fixture monta um pseudo-repo realista para o teste end-to-end de
// F-007. Contém:
//   - Raiz com go.mod
//   - Submódulo `cmd/cli` (pulado por ter seu próprio go.mod no escopo
//     do Service raiz — mas EmitServices cria 2 Services)
//   - Pacote `userservice` (relevante) com método em receiver
//   - Pacote `api/handlers` (relevante) com handlers net/http e chi
//   - Pacote `repo` (NÃO relevante) que deve ser ignorado
//   - Pacote `vendor/...` ignorado
//   - Arquivos `_test.go` e `mocks/` ignorados
func buildFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(p, body string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/app\n")
	write("cmd/cli/go.mod", "module example.com/app/cmd/cli\n")
	write("cmd/cli/main.go", `package main
func main() {}
`)

	write("userservice/svc.go", `package userservice
type Svc struct{}
func (s *Svc) Create(name string) error { return nil }
func (s *Svc) List() ([]string, error) { return nil, nil }
func internalThing() {}
`)
	write("userservice/svc_test.go", `package userservice
func TestCreate() {}
`)

	write("api/handlers/http.go", `package handlers
import "net/http"
func register() {
	http.HandleFunc("/v1/users", listUsers)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/users/{id}", getUser)
}
func listUsers(w http.ResponseWriter, r *http.Request) {}
func getUser(w http.ResponseWriter, r *http.Request) {}
`)
	write("api/handlers/chi.go", `package handlers
func registerChi(r interface{}) {
	r.Get("/v1/items", listItems)
	r.Post("/v1/items", createItem)
	r.Method("PROPFIND", "/dav", davHandler)
}
func listItems() {}
func createItem() {}
func davHandler() {}
`)

	write("repo/postgres.go", `package repo
func Query() {}
`)
	write("mocks/m.go", `package mocks
func Stub() {}
`)
	write("vendor/x/v.go", `package x
func ShouldBeIgnored() {}
`)
	return dir
}

func TestIntegration_FullCollect_EndToEnd(t *testing.T) {
	dir := buildFixture(t)

	cfg := golang.Config{
		Repo:       "myrepo",
		RunID:      "run-1",
		ObservedAt: time.Unix(1700000000, 0).UTC(),
	}
	res, err := golang.Collect(dir, cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// 2 services: ".", "cmd/cli"
	if len(res.Services) != 2 {
		t.Errorf("Services=%d want 2", len(res.Services))
	}
	// Functions: apenas exportadas em pacotes que batem o filtro. Os
	// handlers do fixture são unexported (listUsers etc.), então só
	// (*Svc).Create e (*Svc).List entram.
	if len(res.Functions) != 2 {
		t.Errorf("Functions=%d want 2 — got %+v", len(res.Functions), funcSymbols(res.Functions))
	}
	// 5 endpoints (3 net/http + 3 chi - wait, chi tem 3: Get, Post, Method).
	// net/http: HandleFunc("/v1/users"), HandleFunc("/v1/users/{id}") = 2.
	// chi: Get("/v1/items"), Post("/v1/items"), Method("PROPFIND","/dav") = 3.
	// Total: 5.
	if len(res.Endpoints) != 5 {
		t.Errorf("Endpoints=%d want 5", len(res.Endpoints))
	}
	// Edges = Functions + Endpoints (cada um aponta para seu Service).
	if len(res.Edges) != len(res.Functions)+len(res.Endpoints) {
		t.Errorf("Edges=%d want %d", len(res.Edges), len(res.Functions)+len(res.Endpoints))
	}

	// Toda aresta deve ser válida pela matriz de adjacência.
	for _, e := range res.Edges {
		var fromKind node.Kind = node.KindFunction
		// Heurística simples: se a URN começa com endpoint/ é Endpoint.
		if isEndpointURN(e.From()) {
			fromKind = node.KindEndpoint
		}
		if err := edge.Validate(e, fromKind, node.KindService); err != nil {
			t.Errorf("Validate(%s→%s): %v", e.From(), e.To(), err)
		}
	}

	// Repo writer
	repo := memory.New()
	w := codesvc.Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	st, err := w.Apply(context.Background(), res)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if st.Services+st.Endpoints+st.Functions+st.Edges == 0 {
		t.Fatal("nothing was written")
	}

	// Random spot check: Endpoint GET /v1/items pertence ao Service raiz.
	rootSvc := node.NewServiceURN("myrepo", ".")
	epURN := node.NewEndpointURN("myrepo", ".", "GET", "/v1/items")
	if got, err := repo.GetByURN(context.Background(), epURN, repository.AsOf{}); err != nil {
		t.Errorf("GET /v1/items not in repo: %v", err)
	} else if got.URN() != epURN {
		t.Errorf("URN mismatch: %s", got.URN())
	}
	edges, _ := repo.AsEdgeRepo().Neighbors(context.Background(), epURN, repository.DirOut,
		repository.EdgeFilter{Types: []edge.Type{edge.TypeDefinedIn}})
	if len(edges) != 1 || edges[0].To() != rootSvc {
		t.Errorf("DEFINED_IN edge from endpoint missing: %+v", edges)
	}
}

func TestIntegration_Idempotent(t *testing.T) {
	dir := buildFixture(t)
	cfg := golang.Config{Repo: "myrepo", RunID: "r", ObservedAt: time.Unix(0, 0).UTC()}

	a, err := golang.Collect(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := golang.Collect(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Mesmas URNs/IDs em duas execuções.
	if setOfNodeURNs(a) != setOfNodeURNs(b) {
		t.Errorf("URN set diverged between runs")
	}
	if setOfEdgeIDs(a) != setOfEdgeIDs(b) {
		t.Errorf("Edge ID set diverged between runs")
	}
}

func funcSymbols(fs []node.Function) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Package+"."+f.Symbol)
	}
	return out
}

func isEndpointURN(u node.URN) bool {
	parts, err := node.ParseURN(u)
	if err != nil {
		return false
	}
	return parts.Kind == node.KindEndpoint
}

// setOfNodeURNs/setOfEdgeIDs reduzem o Result a uma string canônica
// (URNs/IDs ordenados) — comparação de igualdade simples basta.
func setOfNodeURNs(r golang.Result) string {
	urns := []string{}
	for _, s := range r.Services {
		urns = append(urns, string(s.URN()))
	}
	for _, e := range r.Endpoints {
		urns = append(urns, string(e.URN()))
	}
	for _, f := range r.Functions {
		urns = append(urns, string(f.URN()))
	}
	return joinSorted(urns)
}
func setOfEdgeIDs(r golang.Result) string {
	ids := []string{}
	for _, e := range r.Edges {
		ids = append(ids, e.ID())
	}
	return joinSorted(ids)
}
