package golang

import (
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func TestExtractModules_NestingAndIndex(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "internal", "api", "h.go"), `package api
func register() {}
`)
	mustWrite(t, filepath.Join(root, "internal", "api", "users", "u.go"), `package users
func List() {}
`)
	mustWrite(t, filepath.Join(root, "internal", "userservice", "svc.go"), `package userservice
func Create() {}
`)

	svcURN := node.NewServiceURN("ex", ".")
	mods, idx, err := ExtractModules(root, root, ".", "ex/repo", svcURN, "ex", nil, EmitOptions{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("ExtractModules: %v", err)
	}

	// Esperamos ao menos: ex/repo (raiz), ex/repo/internal, ex/repo/internal/api,
	// ex/repo/internal/api/users, ex/repo/internal/userservice.
	want := []string{
		"ex/repo",
		"ex/repo/internal",
		"ex/repo/internal/api",
		"ex/repo/internal/api/users",
		"ex/repo/internal/userservice",
	}
	for _, ns := range want {
		if _, ok := idx[ns]; !ok {
			t.Errorf("namespace %q ausente no idx", ns)
		}
	}

	// ParentURN correto para o filho aninhado.
	for _, m := range mods {
		if m.Namespace == "ex/repo/internal/api/users" {
			parentWant := node.NewModuleURN("ex", ".", "ex/repo/internal/api")
			if m.ParentURN != parentWant {
				t.Errorf("ParentURN=%q want %q", m.ParentURN, parentWant)
			}
		}
		if m.Namespace == "ex/repo" {
			if m.ParentURN != "" {
				t.Errorf("raiz não deveria ter ParentURN; got %q", m.ParentURN)
			}
		}
	}
}

func TestCollect_EmitsContainsEdges(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "userservice", "svc.go"), `package userservice
type Svc struct{}
func (s *Svc) Create() error { return nil }
`)
	mustWrite(t, filepath.Join(root, "api", "routes.go"), `package api
import "net/http"
func register() { http.HandleFunc("/v1/users", h) }
func h(w http.ResponseWriter, r *http.Request) {}
`)

	cfg := Config{Repo: "exrepo", RunID: "run-1", ObservedAt: time.Unix(1700000000, 0).UTC()}
	res, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(res.Modules) == 0 {
		t.Fatal("Modules vazio")
	}
	if len(res.Contains) == 0 {
		t.Fatal("Contains vazio")
	}

	// Todas as arestas Contains devem ser tipo CONTAINS, determinísticas, From/To não vazios.
	fromKinds := map[node.URN]node.Kind{}
	for _, s := range res.Services {
		fromKinds[s.URN()] = node.KindService
	}
	for _, m := range res.Modules {
		fromKinds[m.URN()] = node.KindModule
	}
	toKinds := map[node.URN]node.Kind{}
	for _, m := range res.Modules {
		toKinds[m.URN()] = node.KindModule
	}
	for _, f := range res.Functions {
		toKinds[f.URN()] = node.KindFunction
	}
	for _, e := range res.Endpoints {
		toKinds[e.URN()] = node.KindEndpoint
	}

	for _, c := range res.Contains {
		if c.Type() != edge.TypeContains {
			t.Errorf("Type=%q", c.Type())
		}
		if c.ID() == "" || c.From() == "" || c.To() == "" {
			t.Errorf("aresta incompleta: %+v", c)
		}
		fk, ok1 := fromKinds[c.From()]
		tk, ok2 := toKinds[c.To()]
		if !ok1 || !ok2 {
			continue // pode ser raiz que não está indexada
		}
		if err := edge.Validate(c, fk, tk); err != nil {
			t.Errorf("Validate(CONTAINS %s→%s): %v", fk, tk, err)
		}
	}

	// Function/Endpoint devem ter ModuleURN populado.
	for _, f := range res.Functions {
		if f.ModuleURN == "" {
			t.Errorf("Function %q sem ModuleURN", f.Symbol)
		}
	}
	for _, e := range res.Endpoints {
		if e.ModuleURN == "" {
			t.Errorf("Endpoint %q sem ModuleURN", e.Route)
		}
	}
}

func TestExtractModules_Idempotent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service
func DoIt() {}
`)
	svcURN := node.NewServiceURN("ex", ".")
	a, _, err := ExtractModules(root, root, ".", "ex", svcURN, "ex", nil, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, _, err := ExtractModules(root, root, ".", "ex", svcURN, "ex", nil, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("len diverge: a=%d b=%d", len(a), len(b))
	}
	urns := map[node.URN]bool{}
	for _, m := range a {
		urns[m.URN()] = true
	}
	for _, m := range b {
		if !urns[m.URN()] {
			t.Errorf("URN não bate: %q", m.URN())
		}
	}
}
