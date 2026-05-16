package golang

import (
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func TestCollect_EndToEnd(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "cmd", "cli", "go.mod"), "module ex/repo/cmd/cli\n")

	mustWrite(t, filepath.Join(root, "userservice", "svc.go"), `package userservice
type Svc struct{}
func (s *Svc) Create() error { return nil }
`)
	mustWrite(t, filepath.Join(root, "api", "routes.go"), `package api
import "net/http"
func register() { http.HandleFunc("/v1/users", h) }
func h(w http.ResponseWriter, r *http.Request) {}
`)

	cfg := Config{
		Repo:       "exrepo",
		RunID:      "run-1",
		ObservedAt: time.Unix(1700000000, 0).UTC(),
	}
	res, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(res.Services) != 2 {
		t.Fatalf("Services=%d want 2", len(res.Services))
	}
	// Functions: (*Svc).Create. cmd/cli has no .go files → 0.
	if len(res.Functions) != 1 {
		t.Errorf("Functions=%d want 1: %+v", len(res.Functions), res.Functions)
	}
	if len(res.Endpoints) != 1 {
		t.Errorf("Endpoints=%d want 1", len(res.Endpoints))
	}
	if len(res.Edges) != 2 {
		t.Errorf("Edges=%d want 2", len(res.Edges))
	}

	// Every edge is DefinedIn, From→To valid, deterministic ID.
	for _, e := range res.Edges {
		if e.Type() != edge.TypeDefinedIn {
			t.Errorf("Type=%q", e.Type())
		}
		if e.ID() == "" {
			t.Error("empty ID")
		}
		if e.From() == "" || e.To() == "" {
			t.Error("empty URNs")
		}
	}
}

func TestCollect_Idempotent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service
func DoIt() {}
`)
	mustWrite(t, filepath.Join(root, "api", "r.go"), `package api
func reg(r interface{}) { r.Get("/x", h) }
func h() {}
`)
	cfg := Config{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()}

	a, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("b: %v", err)
	}

	if len(a.Services) != len(b.Services) || len(a.Functions) != len(b.Functions) ||
		len(a.Endpoints) != len(b.Endpoints) || len(a.Edges) != len(b.Edges) {
		t.Fatalf("count diverge")
	}
	// Compare URNs of nodes & edge IDs
	uset := map[node.URN]bool{}
	for _, s := range a.Services {
		uset[s.URN()] = true
	}
	for _, s := range b.Services {
		if !uset[s.URN()] {
			t.Errorf("Service URN mismatch: %q", s.URN())
		}
	}
	eset := map[string]bool{}
	for _, e := range a.Edges {
		eset[e.ID()] = true
	}
	for _, e := range b.Edges {
		if !eset[e.ID()] {
			t.Errorf("Edge ID mismatch: %q", e.ID())
		}
	}
}

func TestCollect_EdgesValidateAgainstRegistry(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service
func DoIt() {}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Function → Service is valid; verify via registry.
	for _, e := range res.Edges {
		if err := edge.Validate(e, node.KindFunction, node.KindService); err != nil {
			t.Errorf("Validate: %v", err)
		}
	}
}
