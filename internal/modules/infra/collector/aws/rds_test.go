package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// fakeRDSInstances simula DescribeDBInstances paginado por Marker.
type fakeRDSInstances struct {
	pages [][]rdstypes.DBInstance
	err   error
	calls int
}

func (f *fakeRDSInstances) DescribeDBInstances(ctx context.Context, in *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.pages) {
		return &rds.DescribeDBInstancesOutput{}, nil
	}
	idx := f.calls
	f.calls++
	out := &rds.DescribeDBInstancesOutput{DBInstances: f.pages[idx]}
	if idx < len(f.pages)-1 {
		m := "marker"
		out.Marker = &m
	}
	return out, nil
}

func mkDBInstance(id, az, engine, version string, size, iops int32, encrypted bool, tags map[string]string) rdstypes.DBInstance {
	idCopy := id
	azCopy := az
	eng := engine
	ver := version
	sz := size
	io := iops
	enc := encrypted
	d := rdstypes.DBInstance{
		DBInstanceIdentifier: &idCopy,
		AvailabilityZone:     &azCopy,
		Engine:               &eng,
		EngineVersion:        &ver,
		AllocatedStorage:     &sz,
		Iops:                 &io,
		StorageEncrypted:     &enc,
	}
	for k, val := range tags {
		kk, vv := k, val
		d.TagList = append(d.TagList, rdstypes.Tag{Key: &kk, Value: &vv})
	}
	return d
}

func TestRDS_DiscoverPersistence_HappyPath(t *testing.T) {
	clock := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	topo := mkTopo("123456789012", "us-east-1", []string{"us-east-1a", "us-east-1b"}, clock)

	insts := []rdstypes.DBInstance{
		mkDBInstance("prod-pg", "us-east-1a", "postgres", "15.3", 100, 3000, true, map[string]string{"env": "prod"}),
		mkDBInstance("legacy-mysql", "us-east-1b", "mysql", "8.0.36", 50, 0, false, nil),
	}
	c := NewRDSWithClient(&fakeRDSInstances{pages: [][]rdstypes.DBInstance{insts}}).WithClock(fixedClock(clock))

	got, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got.Persistences) != 2 {
		t.Fatalf("expected 2 persistences, got %d", len(got.Persistences))
	}
	if len(got.Edges) != 2 {
		t.Fatalf("expected 2 Contains edges, got %d", len(got.Edges))
	}

	p0 := got.Persistences[0]
	if p0.URN() != "urn:ce:aws:123456789012:persistence/prod-pg" {
		t.Fatalf("p0 URN = %q", p0.URN())
	}
	if p0.Flavor != node.PersistenceRelDB {
		t.Fatalf("p0 flavor = %q", p0.Flavor)
	}
	if p0.Engine != "postgres-15.3" {
		t.Fatalf("p0 engine = %q (want postgres-15.3)", p0.Engine)
	}
	if p0.SizeGiB != 100 || p0.IOPS != 3000 || !p0.Encrypted {
		t.Fatalf("p0 fields wrong: %+v", p0)
	}
	if p0.Tags["env"] != "prod" {
		t.Fatalf("p0 tags = %+v", p0.Tags)
	}

	// Both edges must be Zone→Persistence Contains
	zoneA := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1a")
	zoneB := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1b")
	seen := map[string]bool{}
	for _, e := range got.Edges {
		if e.Type() != edge.TypeContains {
			t.Fatalf("expected Contains edge, got %q", e.Type())
		}
		seen[string(e.From())+"→"+string(e.To())] = true
	}
	if !seen[string(zoneA)+"→urn:ce:aws:123456789012:persistence/prod-pg"] {
		t.Fatal("missing edge zone-a → prod-pg")
	}
	if !seen[string(zoneB)+"→urn:ce:aws:123456789012:persistence/legacy-mysql"] {
		t.Fatal("missing edge zone-b → legacy-mysql")
	}
}

func TestRDS_DiscoverPersistence_Pagination(t *testing.T) {
	clock := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	page1 := []rdstypes.DBInstance{mkDBInstance("db-1", "us-east-1a", "postgres", "15", 10, 0, false, nil)}
	page2 := []rdstypes.DBInstance{mkDBInstance("db-2", "us-east-1a", "postgres", "15", 10, 0, false, nil)}
	fake := &fakeRDSInstances{pages: [][]rdstypes.DBInstance{page1, page2}}
	c := NewRDSWithClient(fake).WithClock(fixedClock(clock))
	got, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Persistences) != 2 || fake.calls != 2 {
		t.Fatalf("expected 2 persistences across 2 calls, got %d/%d", len(got.Persistences), fake.calls)
	}
}

func TestRDS_DiscoverPersistence_SkipsBlankID(t *testing.T) {
	clock := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	blank := ""
	az := "us-east-1a"
	insts := []rdstypes.DBInstance{
		{DBInstanceIdentifier: &blank, AvailabilityZone: &az},
		mkDBInstance("db-ok", "us-east-1a", "postgres", "15", 10, 0, false, nil),
	}
	c := NewRDSWithClient(&fakeRDSInstances{pages: [][]rdstypes.DBInstance{insts}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Persistences) != 1 {
		t.Fatalf("expected 1 persistence (blank skipped), got %d", len(got.Persistences))
	}
}

func TestRDS_DiscoverPersistence_UnknownAZ_NoContainsEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock) // só 1a
	insts := []rdstypes.DBInstance{
		mkDBInstance("db-z", "us-east-1z", "postgres", "15", 10, 0, false, nil),
	}
	c := NewRDSWithClient(&fakeRDSInstances{pages: [][]rdstypes.DBInstance{insts}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Persistences) != 1 {
		t.Fatalf("expected 1 persistence, got %d", len(got.Persistences))
	}
	if len(got.Edges) != 0 {
		t.Fatalf("expected 0 edges (unknown AZ), got %d", len(got.Edges))
	}
}

func TestRDS_DiscoverPersistence_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	mk := func() *RDS {
		return NewRDSWithClient(&fakeRDSInstances{pages: [][]rdstypes.DBInstance{{
			mkDBInstance("db-1", "us-east-1a", "postgres", "15", 10, 0, false, nil),
		}}}).WithClock(fixedClock(clock))
	}
	a, _ := mk().DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	b, _ := mk().DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(a.Edges) != len(b.Edges) {
		t.Fatalf("edge count differs: %d vs %d", len(a.Edges), len(b.Edges))
	}
	for i := range a.Edges {
		if a.Edges[i].ID() != b.Edges[i].ID() {
			t.Fatalf("edge[%d] ID differs", i)
		}
	}
}

func TestRDS_DiscoverPersistence_EngineLabelFallback(t *testing.T) {
	clock := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	// Engine sem versão → rótulo é só "postgres".
	id := "db-no-ver"
	az := "us-east-1a"
	eng := "postgres"
	d := rdstypes.DBInstance{DBInstanceIdentifier: &id, AvailabilityZone: &az, Engine: &eng}
	c := NewRDSWithClient(&fakeRDSInstances{pages: [][]rdstypes.DBInstance{{d}}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Persistences) != 1 || got.Persistences[0].Engine != "postgres" {
		t.Fatalf("expected engine 'postgres', got %+v", got.Persistences)
	}
}

func TestRDS_DiscoverPersistence_APIError(t *testing.T) {
	boom := errors.New("AccessDenied")
	c := NewRDSWithClient(&fakeRDSInstances{err: boom})
	_, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestRDS_DiscoverPersistence_EmptyScope(t *testing.T) {
	c := NewRDSWithClient(&fakeRDSInstances{})
	if _, err := c.DiscoverPersistence(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}
