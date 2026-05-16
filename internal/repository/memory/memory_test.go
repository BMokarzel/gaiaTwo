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

// fakeClock retorna um relógio incremental para testes determinísticos.
func fakeClock() func() time.Time {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var i int64
	return func() time.Time {
		i++
		return base.Add(time.Duration(i) * time.Hour)
	}
}

func newRepo() *Repo {
	return New().WithClock(fakeClock())
}

func mkCompute(urn, ext string, vfrom time.Time) node.Compute {
	return node.Compute{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  vfrom,
				ObservedAt: vfrom,
				Confidence: 1,
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    "urn:ce:aws:111:account/111",
		Region:     "urn:ce:aws:111:region/us-east-1",
		ExtID:      ext,
		Flavor:     node.ComputeVM,
	}
}

// ----------------------------------------------------------------------------
// Node tests
// ----------------------------------------------------------------------------

func TestUpsert_Insert(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	c := mkCompute("urn:ce:aws:111:compute/i-1", "i-1", time.Now())

	if err := r.Upsert(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := r.GetByURN(ctx, c.URN(), repository.AsOf{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.URN() != c.URN() {
		t.Fatalf("got URN %q, want %q", got.URN(), c.URN())
	}
}

func TestUpsert_Versions(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	urn := "urn:ce:aws:111:compute/i-1"

	v1 := mkCompute(urn, "i-1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	v2 := mkCompute(urn, "i-1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	_ = r.Upsert(ctx, v1)
	_ = r.Upsert(ctx, v2)

	hist, err := r.History(ctx, v1.URN())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	// v1 (índice 0) deve estar fechado.
	if hist[0].Meta().ValidTo == nil {
		t.Fatalf("expected v1 to be closed")
	}
	// v2 (índice 1) deve ser current.
	if !hist[1].Meta().IsCurrent() {
		t.Fatalf("expected v2 to be current")
	}
}

func TestGetByURN_NotFound(t *testing.T) {
	r := newRepo()
	_, err := r.GetByURN(context.Background(), "urn:ce:aws:111:compute/none", repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetByURN_AsOf(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	urn := "urn:ce:aws:111:compute/i-1"

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	v1 := mkCompute(urn, "i-1", t0)
	v2 := mkCompute(urn, "i-1", t1)
	_ = r.Upsert(ctx, v1)
	_ = r.Upsert(ctx, v2)

	// As-of antes do v1 → not found
	if _, err := r.GetByURN(ctx, node.URN(urn), repository.AsOf(t0.Add(-time.Hour))); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound before v1, got %v", err)
	}
	// As-of entre v1 e v2 → v1
	got, err := r.GetByURN(ctx, node.URN(urn), repository.AsOf(t0.Add(time.Hour)))
	if err != nil {
		t.Fatalf("as-of v1: %v", err)
	}
	if got.Meta().ValidFrom != t0 {
		t.Fatalf("as-of v1 returned wrong version")
	}
	// As-of depois de v2 → v2
	got, err = r.GetByURN(ctx, node.URN(urn), repository.AsOf(t2))
	if err != nil {
		t.Fatalf("as-of v2: %v", err)
	}
	if got.Meta().ValidFrom != t1 {
		t.Fatalf("as-of v2 returned wrong version")
	}
}

func TestGetByExternalID(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	c := mkCompute("urn:ce:aws:111:compute/i-1", "i-1", time.Now())
	_ = r.Upsert(ctx, c)

	urn, err := r.GetByExternalID(ctx, node.ProviderAWS, "111", "i-1", repository.AsOf{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if urn != c.URN() {
		t.Fatalf("got %q, want %q", urn, c.URN())
	}

	_, err = r.GetByExternalID(ctx, node.ProviderAWS, "111", "i-unknown", repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestList_FilterByKind(t *testing.T) {
	r := newRepo()
	ctx := context.Background()

	_ = r.Upsert(ctx, mkCompute("urn:ce:aws:111:compute/i-1", "i-1", time.Now()))
	_ = r.Upsert(ctx, mkCompute("urn:ce:aws:111:compute/i-2", "i-2", time.Now()))

	persistence := node.Persistence{
		Base: node.Base{
			NodeURN:  "urn:ce:aws:111:persistence/db-1",
			NodeKind: node.KindPersistence,
			NodeMeta: node.Meta{ValidFrom: time.Now()},
		},
		ProviderID: node.ProviderAWS,
		Account:    "urn:ce:aws:111:account/111",
		Region:     "urn:ce:aws:111:region/us-east-1",
		ExtID:      "db-1",
		Flavor:     node.PersistenceRelDB,
	}
	_ = r.Upsert(ctx, persistence)

	out, err := r.List(ctx, repository.NodeFilter{Kind: node.KindCompute})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("kind=compute returned %d, want 2", len(out))
	}
}

func TestList_AsOfHidesFuture(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = r.Upsert(ctx, mkCompute("urn:ce:aws:111:compute/i-1", "i-1", future))

	out, _ := r.List(ctx, repository.NodeFilter{AsOf: repository.AsOf(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})
	if len(out) != 0 {
		t.Fatalf("expected 0 nodes as-of past, got %d", len(out))
	}
}

func TestDelete(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	c := mkCompute("urn:ce:aws:111:compute/i-1", "i-1", time.Now())
	_ = r.Upsert(ctx, c)

	if err := r.Delete(ctx, c.URN()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := r.GetByURN(ctx, c.URN(), repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Edge tests
// ----------------------------------------------------------------------------

func mkAttached(from, to node.URN, vfrom time.Time) edge.AttachedTo {
	id := edge.DeterministicID(from, edge.TypeAttachedTo, to, vfrom)
	return edge.AttachedTo{
		Base: edge.Base{
			EdgeID:   id,
			EdgeType: edge.TypeAttachedTo,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{ValidFrom: vfrom, Directional: true},
		},
	}
}

func TestEdgeUpsert_Validates(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()

	e := mkAttached("urn:ce:aws:111:persistence/db", "urn:ce:aws:111:compute/i-1", time.Now())
	if err := er.Upsert(ctx, e, node.KindPersistence, node.KindCompute); err != nil {
		t.Fatalf("valid edge rejected: %v", err)
	}

	// Compute → Persistence é direção inválida para AttachedTo.
	bad := mkAttached("urn:ce:aws:111:compute/i-1", "urn:ce:aws:111:persistence/db", time.Now())
	err := er.Upsert(ctx, bad, node.KindCompute, node.KindPersistence)
	if !errors.Is(err, repository.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestEdgeBetweenAndNeighbors(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Now()

	a := node.URN("urn:ce:aws:111:persistence/db-a")
	b := node.URN("urn:ce:aws:111:compute/i-b")

	e := mkAttached(a, b, now)
	if err := er.Upsert(ctx, e, node.KindPersistence, node.KindCompute); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	betw, _ := er.Between(ctx, a, b, repository.EdgeFilter{})
	if len(betw) != 1 {
		t.Fatalf("between len = %d, want 1", len(betw))
	}

	out, _ := er.Neighbors(ctx, a, repository.DirOut, repository.EdgeFilter{})
	if len(out) != 1 {
		t.Fatalf("out neighbors = %d, want 1", len(out))
	}
	in, _ := er.Neighbors(ctx, b, repository.DirIn, repository.EdgeFilter{})
	if len(in) != 1 {
		t.Fatalf("in neighbors = %d, want 1", len(in))
	}
}

func TestEdgeTraverse(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Now()

	// db ──ATTACHED_TO──▶ vm ──DEPLOYED_ON──▶ cluster
	db := node.URN("urn:ce:aws:111:persistence/db")
	vm := node.URN("urn:ce:aws:111:compute/vm")
	cluster := node.URN("urn:ce:aws:111:compute/cluster")

	e1 := mkAttached(db, vm, now)
	_ = er.Upsert(ctx, e1, node.KindPersistence, node.KindCompute)

	e2 := edge.DeployedOn{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(vm, edge.TypeDeployedOn, cluster, now),
			EdgeType: edge.TypeDeployedOn,
			FromURN:  vm,
			ToURN:    cluster,
			EdgeMeta: edge.Meta{ValidFrom: now, Directional: true},
		},
	}
	_ = er.Upsert(ctx, e2, node.KindCompute, node.KindCompute)

	out, err := er.Traverse(ctx, db, repository.DirOut, 2, repository.EdgeFilter{})
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("traverse depth=2 got %d, want 2 (vm + cluster)", len(out))
	}

	// depth=1 deve trazer só vm
	out, _ = er.Traverse(ctx, db, repository.DirOut, 1, repository.EdgeFilter{})
	if len(out) != 1 || out[0] != vm {
		t.Fatalf("traverse depth=1 = %v, want [vm]", out)
	}
}

func TestEdgeFilterByType(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Now()

	db := node.URN("urn:ce:aws:111:persistence/db")
	vm := node.URN("urn:ce:aws:111:compute/vm")

	_ = er.Upsert(ctx, mkAttached(db, vm, now), node.KindPersistence, node.KindCompute)

	out, _ := er.Neighbors(ctx, db, repository.DirOut, repository.EdgeFilter{Types: []edge.Type{edge.TypeDependsOn}})
	if len(out) != 0 {
		t.Fatalf("type filter should exclude AttachedTo, got %d", len(out))
	}
	out, _ = er.Neighbors(ctx, db, repository.DirOut, repository.EdgeFilter{Types: []edge.Type{edge.TypeAttachedTo}})
	if len(out) != 1 {
		t.Fatalf("type filter should include AttachedTo, got %d", len(out))
	}
}

func TestEdgeDelete(t *testing.T) {
	r := newRepo()
	er := r.AsEdgeRepo()
	ctx := context.Background()
	now := time.Now()
	a := node.URN("urn:ce:aws:111:persistence/db")
	b := node.URN("urn:ce:aws:111:compute/vm")
	e := mkAttached(a, b, now)
	_ = er.Upsert(ctx, e, node.KindPersistence, node.KindCompute)

	if err := er.Delete(ctx, e.ID()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	out, _ := er.Neighbors(ctx, a, repository.DirOut, repository.EdgeFilter{})
	if len(out) != 0 {
		t.Fatalf("expected no current edges after delete, got %d", len(out))
	}
}

func TestUpsert_InvalidArgs(t *testing.T) {
	r := newRepo()
	ctx := context.Background()
	// URN vazia
	bad := node.Compute{Base: node.Base{NodeURN: "", NodeKind: node.KindCompute}}
	err := r.Upsert(ctx, bad)
	if !errors.Is(err, repository.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}
