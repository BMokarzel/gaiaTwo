package github_webhook

import (
	"context"
	"sort"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// fakeLookup é um ServiceLookup minimal que devolve um slice fixo.
type fakeLookup struct{ nodes []node.Node }

func (f fakeLookup) List(ctx context.Context, _ repository.NodeFilter) ([]node.Node, error) {
	return f.nodes, nil
}

func svc(repo, mp string) node.Service {
	return node.Service{
		Base: node.Base{
			NodeURN:  node.NewServiceURN(repo, mp),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{ValidFrom: time.Now(), Confidence: 1},
		},
		Repo:       repo,
		ModulePath: mp,
	}
}

func TestResolver_LongestPrefixWins(t *testing.T) {
	lookup := fakeLookup{nodes: []node.Node{
		svc("acme/api", "."),
		svc("acme/api", "services/billing"),
		svc("acme/api", "services/billing/refund"),
		svc("acme/web", "."), // mesmo "."  em outro repo — não deve interferir
	}}
	r, err := newPathResolver(context.Background(), lookup, "acme/api")
	if err != nil {
		t.Fatalf("newPathResolver: %v", err)
	}

	tests := []struct {
		path string
		want node.URN
	}{
		{"services/billing/refund/main.go", node.NewServiceURN("acme/api", "services/billing/refund")},
		{"services/billing/charge.go", node.NewServiceURN("acme/api", "services/billing")},
		{"cmd/api/main.go", node.NewServiceURN("acme/api", ".")},
		{"README.md", node.NewServiceURN("acme/api", ".")},
	}
	for _, tc := range tests {
		got := r.Resolve([]string{tc.path})
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("path %s: got %v, want %s", tc.path, got, tc.want)
		}
	}
}

func TestResolver_NoRootFallback(t *testing.T) {
	lookup := fakeLookup{nodes: []node.Node{
		svc("acme/api", "services/billing"),
	}}
	r, _ := newPathResolver(context.Background(), lookup, "acme/api")
	got := r.Resolve([]string{"cmd/api/main.go", "docs/X.md"})
	if len(got) != 0 {
		t.Errorf("expected nothing resolved, got %v", got)
	}
}

func TestResolver_DistinctServices(t *testing.T) {
	lookup := fakeLookup{nodes: []node.Node{
		svc("acme/api", "services/billing"),
		svc("acme/api", "services/orders"),
	}}
	r, _ := newPathResolver(context.Background(), lookup, "acme/api")
	got := r.Resolve([]string{
		"services/billing/a.go",
		"services/billing/b.go",
		"services/orders/c.go",
	})
	if len(got) != 2 {
		t.Errorf("expected 2 distinct services, got %d (%v)", len(got), got)
	}
	strs := make([]string, len(got))
	for i, u := range got {
		strs[i] = string(u)
	}
	sort.Strings(strs)
	if strs[0] != string(node.NewServiceURN("acme/api", "services/billing")) ||
		strs[1] != string(node.NewServiceURN("acme/api", "services/orders")) {
		t.Errorf("unexpected URNs: %v", strs)
	}
}
