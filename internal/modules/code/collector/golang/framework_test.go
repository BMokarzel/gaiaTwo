package golang

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func TestExtractFrameworks_ParsesBothSingleAndBlock(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), `module ex/repo

go 1.22

require github.com/go-chi/chi/v5 v5.0.10

require (
	github.com/jackc/pgx/v5 v5.5.0
	golang.org/x/sync v0.5.0 // indirect
)
`)
	fws, deps, err := ExtractFrameworks(root, root, EmitOptions{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("ExtractFrameworks: %v", err)
	}
	if len(fws) != 3 || len(deps) != 3 {
		t.Fatalf("len fws=%d deps=%d want 3", len(fws), len(deps))
	}

	got := map[string]node.Framework{}
	for _, f := range fws {
		got[f.Name] = f
	}
	if chi, ok := got["github.com/go-chi/chi/v5"]; !ok || chi.LatestVersion != "v5.0.10" || chi.IsDevOnly {
		t.Errorf("chi inesperado: %+v", chi)
	}
	if pgx, ok := got["github.com/jackc/pgx/v5"]; !ok || pgx.LatestVersion != "v5.5.0" || pgx.IsDevOnly {
		t.Errorf("pgx inesperado: %+v", pgx)
	}
	if sync, ok := got["golang.org/x/sync"]; !ok || !sync.IsDevOnly {
		t.Errorf("indirect deveria marcar IsDevOnly: %+v", sync)
	}

	// URN é global.
	for _, f := range fws {
		want := node.NewFrameworkURN("go", f.Name)
		if f.URN() != want {
			t.Errorf("URN %q want %q", f.URN(), want)
		}
	}
}

func TestExtractFrameworks_Empty(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n\ngo 1.22\n")
	fws, deps, err := ExtractFrameworks(root, root, EmitOptions{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(fws) != 0 || len(deps) != 0 {
		t.Errorf("esperado vazio, got fws=%d deps=%d", len(fws), len(deps))
	}
}

func TestCollect_EmitsFrameworksAndDependsOn(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), `module ex/repo
go 1.22
require github.com/go-chi/chi/v5 v5.0.10
`)
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service
func DoIt() {}
`)
	res, err := Collect(root, Config{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(res.Frameworks) != 1 {
		t.Fatalf("Frameworks=%d want 1", len(res.Frameworks))
	}
	if len(res.DependsOn) != 1 {
		t.Fatalf("DependsOn=%d want 1", len(res.DependsOn))
	}

	d := res.DependsOn[0]
	if d.Type() != edge.TypeDependsOn {
		t.Errorf("Type=%q", d.Type())
	}
	if d.From() != res.Services[0].URN() || d.To() != res.Frameworks[0].URN() {
		t.Errorf("From/To incorretos: %q→%q", d.From(), d.To())
	}
	if !d.Declared {
		t.Error("Declared deveria ser true para require direto")
	}
	if !d.Hard {
		t.Error("Hard deveria ser true para require direto (não indirect)")
	}

	// Validate via registry.
	if err := edge.Validate(d, node.KindService, node.KindFramework); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestCollect_Frameworks_Idempotent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), `module ex
go 1.22
require (
	github.com/x/y v1.0.0
	github.com/a/b v2.0.0
)
`)
	mustWrite(t, filepath.Join(root, "service", "s.go"), "package service\nfunc Do(){}\n")

	a, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if len(a.Frameworks) != len(b.Frameworks) {
		t.Fatalf("len diverge a=%d b=%d", len(a.Frameworks), len(b.Frameworks))
	}
	au, bu := []string{}, []string{}
	for _, f := range a.Frameworks {
		au = append(au, string(f.URN()))
	}
	for _, f := range b.Frameworks {
		bu = append(bu, string(f.URN()))
	}
	sort.Strings(au)
	sort.Strings(bu)
	for i := range au {
		if au[i] != bu[i] {
			t.Errorf("URN[%d] diverge: %q vs %q", i, au[i], bu[i])
		}
	}
}
