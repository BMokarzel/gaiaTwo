package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// mkService cria um Service mínimo para alimentar Search.
func mkService(urn, repo, modulePath string, vfrom time.Time) node.Service {
	return node.Service{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vfrom, ObservedAt: vfrom, Confidence: 1},
		},
		Repo:       repo,
		ModulePath: modulePath,
		Language:   "go",
	}
}

func mkComputeNamed(urn, ext, nameTag string, vfrom time.Time) node.Compute {
	c := mkCompute(urn, ext, vfrom)
	c.Tags = map[string]string{"Name": nameTag}
	return c
}

func mkDeployed(from, to node.URN, vfrom time.Time) edge.DeployedOn {
	id := edge.DeterministicID(from, edge.TypeDeployedOn, to, vfrom)
	return edge.DeployedOn{
		Base: edge.Base{
			EdgeID:   id,
			EdgeType: edge.TypeDeployedOn,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{ValidFrom: vfrom, Directional: true},
		},
	}
}

// ----------------------------------------------------------------------------
// Search
// ----------------------------------------------------------------------------

func TestSearch_EmptyQ_ReturnsInvalidArgument(t *testing.T) {
	r := newRepo()
	_, err := r.Search(context.Background(), repository.SearchQuery{Q: "  "})
	if !errors.Is(err, repository.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestSearch_MatchesURN_ServiceFields_AndResourceTagName(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	now := time.Now()

	_ = r.Upsert(ctx, mkService("urn:ce:code:my-repo:service/.", "my-repo", ".", now))
	_ = r.Upsert(ctx, mkService("urn:ce:code:another:service/billing", "another", "billing", now))
	_ = r.Upsert(ctx, mkComputeNamed("urn:ce:aws:111:compute/i-1", "i-1", "svc-payments-prod", now))
	_ = r.Upsert(ctx, mkCompute("urn:ce:aws:111:compute/i-2", "vol-secret", now))

	// URN substring
	out, err := r.Search(ctx, repository.SearchQuery{Q: "billing"})
	if err != nil || len(out) != 1 || string(out[0].URN()) != "urn:ce:code:another:service/billing" {
		t.Fatalf("billing search: err=%v out=%v", err, out)
	}

	// Repo field
	out, _ = r.Search(ctx, repository.SearchQuery{Q: "my-repo"})
	if len(out) != 1 || string(out[0].URN()) != "urn:ce:code:my-repo:service/." {
		t.Fatalf("my-repo search: %v", out)
	}

	// Resource Name tag
	out, _ = r.Search(ctx, repository.SearchQuery{Q: "payments"})
	if len(out) != 1 || string(out[0].URN()) != "urn:ce:aws:111:compute/i-1" {
		t.Fatalf("payments search: %v", out)
	}

	// ExternalID match
	out, _ = r.Search(ctx, repository.SearchQuery{Q: "vol-secret"})
	if len(out) != 1 || string(out[0].URN()) != "urn:ce:aws:111:compute/i-2" {
		t.Fatalf("ext search: %v", out)
	}
}

func TestSearch_CaseInsensitive_AndKindFilter(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	now := time.Now()
	_ = r.Upsert(ctx, mkService("urn:ce:code:Apex:service/.", "Apex", ".", now))
	_ = r.Upsert(ctx, mkComputeNamed("urn:ce:aws:111:compute/i-apex", "i-apex", "Apex", now))

	out, _ := r.Search(ctx, repository.SearchQuery{Q: "APEX"})
	if len(out) != 2 {
		t.Fatalf("case-insensitive total: %d", len(out))
	}
	out, _ = r.Search(ctx, repository.SearchQuery{Q: "apex", Kind: node.KindService})
	if len(out) != 1 || out[0].Kind() != node.KindService {
		t.Fatalf("kind filter: %v", out)
	}
}

func TestSearch_DeterministicOrder_AndLimitOffset(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	now := time.Now()
	for _, urn := range []string{
		"urn:ce:code:c:service/.",
		"urn:ce:code:a:service/.",
		"urn:ce:code:b:service/.",
	} {
		_ = r.Upsert(ctx, mkService(urn, "shared-name", ".", now))
	}
	out, _ := r.Search(ctx, repository.SearchQuery{Q: "shared-name"})
	if len(out) != 3 ||
		string(out[0].URN()) != "urn:ce:code:a:service/." ||
		string(out[1].URN()) != "urn:ce:code:b:service/." ||
		string(out[2].URN()) != "urn:ce:code:c:service/." {
		t.Fatalf("order: %v", out)
	}
	page2, _ := r.Search(ctx, repository.SearchQuery{Q: "shared-name", Limit: 1, Offset: 1})
	if len(page2) != 1 || string(page2[0].URN()) != "urn:ce:code:b:service/." {
		t.Fatalf("paging: %v", page2)
	}
}

func TestSearch_SkipsClosedVersions(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	urn := "urn:ce:code:gone:service/."
	v1 := mkService(urn, "gone", ".", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	_ = r.Upsert(ctx, v1)
	_ = r.Delete(ctx, v1.URN())
	out, _ := r.Search(ctx, repository.SearchQuery{Q: "gone"})
	if len(out) != 0 {
		t.Fatalf("expected no current version, got %v", out)
	}
}

// ----------------------------------------------------------------------------
// Paths
// ----------------------------------------------------------------------------

func TestPaths_RejectsBadArgs(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	cases := []struct {
		name  string
		from  node.URN
		to    node.URN
		hops  int
		wants error
	}{
		{"empty from", "", "x", 1, repository.ErrInvalidArgument},
		{"empty to", "x", "", 1, repository.ErrInvalidArgument},
		{"zero hops", "x", "y", 0, repository.ErrInvalidArgument},
		{"too many hops", "x", "y", 6, repository.ErrInvalidArgument},
	}
	for _, c := range cases {
		_, err := er.Paths(context.Background(), c.from, c.to, c.hops, repository.EdgeFilter{})
		if !errors.Is(err, c.wants) {
			t.Fatalf("%s: got %v", c.name, err)
		}
	}
}

func TestPaths_FromEqualsTo_ReturnsTrivial(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	out, err := er.Paths(context.Background(), "x", "x", 2, repository.EdgeFilter{})
	if err != nil || len(out) != 1 || out[0][0] != "x" {
		t.Fatalf("trivial: %v %v", out, err)
	}
}

func TestPaths_SimpleChain_AndDirectionRespected(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Now()
	a := node.URN("urn:ce:aws:111:compute/a")
	b := node.URN("urn:ce:aws:111:compute/b")
	c := node.URN("urn:ce:aws:111:compute/c")

	// a → b → c
	_ = er.Upsert(ctx, mkDeployed(a, b, now), node.KindCompute, node.KindCompute)
	_ = er.Upsert(ctx, mkDeployed(b, c, now), node.KindCompute, node.KindCompute)

	out, err := er.Paths(ctx, a, c, 3, repository.EdgeFilter{})
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	if len(out) != 1 || len(out[0]) != 3 || out[0][0] != a || out[0][1] != b || out[0][2] != c {
		t.Fatalf("expected single chain a→b→c, got %v", out)
	}

	// Reverso (c→a) não existe via DirOut.
	rev, _ := er.Paths(ctx, c, a, 3, repository.EdgeFilter{})
	if len(rev) != 0 {
		t.Fatalf("reverse should be empty, got %v", rev)
	}

	// maxHops=1 não consegue cobrir a→c (2 hops necessários).
	short, _ := er.Paths(ctx, a, c, 1, repository.EdgeFilter{})
	if len(short) != 0 {
		t.Fatalf("hops=1 should miss, got %v", short)
	}
}

func TestPaths_MultiplePaths_Deterministic(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Now()
	a := node.URN("urn:ce:aws:111:compute/a")
	b := node.URN("urn:ce:aws:111:compute/b")
	c := node.URN("urn:ce:aws:111:compute/c")
	d := node.URN("urn:ce:aws:111:compute/d")

	// a→b→d e a→c→d
	for _, e := range []edge.Edge{
		mkDeployed(a, b, now),
		mkDeployed(a, c, now),
		mkDeployed(b, d, now),
		mkDeployed(c, d, now),
	} {
		_ = er.Upsert(ctx, e, node.KindCompute, node.KindCompute)
	}

	out, err := er.Paths(ctx, a, d, 3, repository.EdgeFilter{})
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 paths, got %d: %v", len(out), out)
	}
	// Determinismo: a→b→d antes de a→c→d (URN b < c).
	if out[0][1] != b || out[1][1] != c {
		t.Fatalf("non-deterministic order: %v", out)
	}
}
