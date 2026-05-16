package golang

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

func TestExtractFunctions_PicksExportedInRelevantPackages(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/test\n")

	// pkg `userservice` → relevante (contém "service")
	mustWrite(t, filepath.Join(root, "userservice", "svc.go"), `package userservice

type Svc struct{}

func (s *Svc) CreateUser(name string, age int) error { return nil }

func PublicHelper(in string) (string, error) { return in, nil }

func privateThing() {}
`)
	// pkg `repo` → irrelevante (não bate substring)
	mustWrite(t, filepath.Join(root, "repo", "r.go"), `package repo

func ShouldBeIgnored() {}
`)
	// _test.go ignorado
	mustWrite(t, filepath.Join(root, "userservice", "svc_test.go"), `package userservice

func TestX() {}
`)

	urn := node.NewServiceURN("ex", ".")
	got, err := ExtractFunctions(root, root, ".", urn, "ex",
		FuncFilterOptions{}, EmitOptions{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("ExtractFunctions: %v", err)
	}

	syms := []string{}
	for _, f := range got {
		syms = append(syms, f.Symbol)
	}
	sort.Strings(syms)

	want := []string{"(*Svc).CreateUser", "PublicHelper"}
	if len(syms) != len(want) {
		t.Fatalf("got symbols %v want %v", syms, want)
	}
	for i, w := range want {
		if syms[i] != w {
			t.Errorf("syms[%d]=%q want %q", i, syms[i], w)
		}
	}

	// URN/Service link
	for _, f := range got {
		if f.ServiceURN != urn {
			t.Errorf("ServiceURN=%q want %q", f.ServiceURN, urn)
		}
		if f.Kind() != node.KindFunction {
			t.Errorf("Kind=%q", f.Kind())
		}
		if f.SignatureHash == "" {
			t.Errorf("empty SignatureHash for %q", f.Symbol)
		}
	}
}

func TestExtractFunctions_SkipsSubmodule(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "handlers", "h.go"), `package handlers
func TopFn() {}
`)
	// Submódulo deve ser pulado (tem seu próprio go.mod).
	mustWrite(t, filepath.Join(root, "cmd", "cli", "go.mod"), "module ex/cmd/cli\n")
	mustWrite(t, filepath.Join(root, "cmd", "cli", "handlers", "h.go"), `package handlers
func SubmoduleFn() {}
`)

	got, err := ExtractFunctions(root, root, ".",
		node.NewServiceURN("ex", "."), "ex",
		FuncFilterOptions{}, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("ExtractFunctions: %v", err)
	}
	if len(got) != 1 || got[0].Symbol != "TopFn" {
		t.Fatalf("got=%+v", got)
	}
}

func TestExtractFunctions_Idempotent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service
func DoThing(a, b int) (int, error) { return a + b, nil }
`)
	a, err := ExtractFunctions(root, root, ".",
		node.NewServiceURN("ex", "."), "ex",
		FuncFilterOptions{}, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := ExtractFunctions(root, root, ".",
		node.NewServiceURN("ex", "."), "ex",
		FuncFilterOptions{}, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("a=%d b=%d", len(a), len(b))
	}
	if a[0].URN() != b[0].URN() {
		t.Errorf("URN diverge")
	}
	if a[0].SignatureHash != b[0].SignatureHash {
		t.Errorf("SignatureHash diverge")
	}
}

func TestRenderSignature_Variants(t *testing.T) {
	// Sanity: pelo menos round-trip pelo extractor cobre o caso comum.
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "handlers", "h.go"), `package handlers
import "context"
func A(ctx context.Context, ids ...string) (map[string]int, error) { return nil, nil }
`)
	got, err := ExtractFunctions(root, root, ".",
		node.NewServiceURN("ex", "."), "ex",
		FuncFilterOptions{}, EmitOptions{Repo: "ex"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	want := "(context.Context,...string)(map[string]int,error)"
	if got[0].Signature != want {
		t.Errorf("Signature=%q want %q", got[0].Signature, want)
	}
}
