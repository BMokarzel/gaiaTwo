package codeowners

import (
	"context"
	"errors"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository/memory"
)

func mkService(repo, modPath string, vfrom time.Time) node.Service {
	return node.Service{
		Base: node.Base{
			NodeURN:  node.NewServiceURN(repo, modPath),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vfrom, ObservedAt: vfrom, Confidence: 1},
		},
		Repo: repo, ModulePath: modPath, Language: "go", GoModule: "example.com/" + repo,
	}
}

func TestServiceLookup_RootModule(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_ = r.Upsert(context.Background(), mkService("payments", ".", now))

	l := &ServiceLookup{Nodes: r}
	res, err := l.ResolveForRepo(context.Background(), "/home/u/repos/payments")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Found != node.NewServiceURN("payments", ".") {
		t.Fatalf("found=%q", res.Found)
	}
	if len(res.Ambiguous) != 0 {
		t.Fatalf("ambig=%v", res.Ambiguous)
	}
}

func TestServiceLookup_SubmoduleOnly(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	// Só existe um Service no submódulo cmd/api — sem raiz.
	_ = r.Upsert(context.Background(), mkService("payments", "cmd/api", now))

	l := &ServiceLookup{Nodes: r}
	res, err := l.ResolveForRepo(context.Background(), "payments")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Found != node.NewServiceURN("payments", "cmd/api") {
		t.Fatalf("found=%q", res.Found)
	}
}

func TestServiceLookup_MonoRepoAmbiguous(t *testing.T) {
	r := memory.New()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	_ = r.Upsert(context.Background(), mkService("platform", "cmd/api", now))
	_ = r.Upsert(context.Background(), mkService("platform", "cmd/worker", now))

	l := &ServiceLookup{Nodes: r}
	res, err := l.ResolveForRepo(context.Background(), "platform")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Found != "" {
		t.Fatalf("expected ambiguous, got Found=%q", res.Found)
	}
	if len(res.Ambiguous) != 2 {
		t.Fatalf("ambig=%v want 2", res.Ambiguous)
	}
}

func TestServiceLookup_NotFound(t *testing.T) {
	r := memory.New()
	l := &ServiceLookup{Nodes: r}
	_, err := l.ResolveForRepo(context.Background(), "missing")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("err=%v want ErrServiceNotFound", err)
	}
}

func TestRepoBasename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"payments", "payments"},
		{"/home/u/repos/payments", "payments"},
		{"/home/u/repos/payments/", "payments"},
		{`C:\dev\payments`, "payments"},
		{`C:\dev\payments\`, "payments"},
		{"", ""},
	}
	for _, c := range cases {
		if got := repoBasename(c.in); got != c.want {
			t.Errorf("repoBasename(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
