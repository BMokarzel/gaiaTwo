package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// fakeEC2Volumes simula DescribeVolumes paginado.
type fakeEC2Volumes struct {
	pages [][]ec2types.Volume
	err   error
	calls int
}

func (f *fakeEC2Volumes) DescribeVolumes(ctx context.Context, in *ec2.DescribeVolumesInput, opts ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.pages) {
		return &ec2.DescribeVolumesOutput{}, nil
	}
	idx := f.calls
	f.calls++
	out := &ec2.DescribeVolumesOutput{Volumes: f.pages[idx]}
	if idx < len(f.pages)-1 {
		tok := "tok"
		out.NextToken = &tok
	}
	return out, nil
}

func mkVolume(id, az, volType string, size int32, iops int32, encrypted bool, tags map[string]string, attached []string) ec2types.Volume {
	idCopy := id
	azCopy := az
	enc := encrypted
	sz := size
	io := iops
	v := ec2types.Volume{
		VolumeId:         &idCopy,
		AvailabilityZone: &azCopy,
		VolumeType:       ec2types.VolumeType(volType),
		Size:             &sz,
		Iops:             &io,
		Encrypted:        &enc,
	}
	for k, val := range tags {
		kk, vv := k, val
		v.Tags = append(v.Tags, ec2types.Tag{Key: &kk, Value: &vv})
	}
	for _, instID := range attached {
		idc := instID
		dev := "/dev/sdf"
		v.Attachments = append(v.Attachments, ec2types.VolumeAttachment{
			InstanceId: &idc,
			Device:     &dev,
		})
	}
	return v
}

func TestEBS_DiscoverPersistence_AttachedAndDetached(t *testing.T) {
	clock := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	topo := mkTopo("123456789012", "us-east-1", []string{"us-east-1a", "us-east-1b"}, clock)

	vols := []ec2types.Volume{
		mkVolume("vol-aaa", "us-east-1a", "gp3", 100, 3000, true, map[string]string{"env": "prod"}, []string{"i-aaa"}),
		mkVolume("vol-bbb", "us-east-1b", "io2", 200, 10000, false, nil, nil), // detached
	}
	c := NewEBSWithClient(&fakeEC2Volumes{pages: [][]ec2types.Volume{vols}}).WithClock(fixedClock(clock))

	got, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got.Persistences) != 2 {
		t.Fatalf("expected 2 persistences, got %d", len(got.Persistences))
	}

	// vol-aaa attached: edges = Contains + AttachedTo = 2
	// vol-bbb detached: edges = Contains = 1
	// total = 3
	if len(got.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(got.Edges))
	}

	// vol-aaa fields
	p0 := got.Persistences[0]
	if p0.URN() != "urn:ce:aws:123456789012:persistence/vol-aaa" {
		t.Fatalf("p0 URN = %q", p0.URN())
	}
	if p0.Flavor != node.PersistenceBlock {
		t.Fatalf("p0 flavor = %q", p0.Flavor)
	}
	if p0.Engine != "gp3" || p0.SizeGiB != 100 || p0.IOPS != 3000 || !p0.Encrypted {
		t.Fatalf("p0 fields wrong: %+v", p0)
	}
	if p0.Tags["env"] != "prod" {
		t.Fatalf("p0 tags wrong: %+v", p0.Tags)
	}

	// Verifica edges presentes.
	zoneA := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1a")
	zoneB := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1b")
	persA := node.URN("urn:ce:aws:123456789012:persistence/vol-aaa")
	persB := node.URN("urn:ce:aws:123456789012:persistence/vol-bbb")
	computeA := node.NewURN(node.ProviderAWS, "123456789012", node.KindCompute, "i-aaa")

	seen := map[string]bool{}
	for _, e := range got.Edges {
		key := string(e.Type()) + ":" + string(e.From()) + "→" + string(e.To())
		seen[key] = true
	}
	if !seen["CONTAINS:"+string(zoneA)+"→"+string(persA)] {
		t.Fatal("missing edge Zone→Persistence for vol-aaa")
	}
	if !seen["CONTAINS:"+string(zoneB)+"→"+string(persB)] {
		t.Fatal("missing edge Zone→Persistence for vol-bbb")
	}
	if !seen["ATTACHED_TO:"+string(persA)+"→"+string(computeA)] {
		t.Fatalf("missing edge Persistence→Compute for vol-aaa; got %+v", seen)
	}
}

func TestEBS_DiscoverPersistence_MultipleAttachments(t *testing.T) {
	clock := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)

	vols := []ec2types.Volume{
		mkVolume("vol-multi", "us-east-1a", "gp3", 50, 0, true, nil, []string{"i-1", "i-2"}),
	}
	c := NewEBSWithClient(&fakeEC2Volumes{pages: [][]ec2types.Volume{vols}}).WithClock(fixedClock(clock))
	got, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatal(err)
	}
	// 1 Contains + 2 AttachedTo = 3
	if len(got.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(got.Edges))
	}
	attachedCount := 0
	for _, e := range got.Edges {
		if e.Type() == edge.TypeAttachedTo {
			attachedCount++
		}
	}
	if attachedCount != 2 {
		t.Fatalf("expected 2 AttachedTo, got %d", attachedCount)
	}
}

func TestEBS_DiscoverPersistence_Pagination(t *testing.T) {
	clock := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	page1 := []ec2types.Volume{mkVolume("vol-1", "us-east-1a", "gp3", 10, 0, false, nil, nil)}
	page2 := []ec2types.Volume{mkVolume("vol-2", "us-east-1a", "gp3", 10, 0, false, nil, nil)}
	fake := &fakeEC2Volumes{pages: [][]ec2types.Volume{page1, page2}}
	c := NewEBSWithClient(fake).WithClock(fixedClock(clock))
	got, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Persistences) != 2 || fake.calls != 2 {
		t.Fatalf("expected 2 persistences across 2 calls, got %d/%d", len(got.Persistences), fake.calls)
	}
}

func TestEBS_DiscoverPersistence_SkipsBlankVolumeID(t *testing.T) {
	clock := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	blank := ""
	az := "us-east-1a"
	vols := []ec2types.Volume{
		{VolumeId: &blank, AvailabilityZone: &az},
		mkVolume("vol-1", "us-east-1a", "gp3", 10, 0, false, nil, nil),
	}
	c := NewEBSWithClient(&fakeEC2Volumes{pages: [][]ec2types.Volume{vols}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Persistences) != 1 {
		t.Fatalf("expected 1 persistence (blank skipped), got %d", len(got.Persistences))
	}
}

func TestEBS_DiscoverPersistence_UnknownAZ_NoContainsEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock) // só 1a
	vols := []ec2types.Volume{
		mkVolume("vol-1", "us-east-1z", "gp3", 10, 0, false, nil, []string{"i-1"}),
	}
	c := NewEBSWithClient(&fakeEC2Volumes{pages: [][]ec2types.Volume{vols}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Persistences) != 1 {
		t.Fatalf("expected 1 persistence, got %d", len(got.Persistences))
	}
	// AZ desconhecida → sem Contains; mas attachment ainda gera AttachedTo.
	containsCount := 0
	attachedCount := 0
	for _, e := range got.Edges {
		switch e.Type() {
		case edge.TypeContains:
			containsCount++
		case edge.TypeAttachedTo:
			attachedCount++
		}
	}
	if containsCount != 0 {
		t.Fatalf("expected 0 Contains for unknown AZ, got %d", containsCount)
	}
	if attachedCount != 1 {
		t.Fatalf("expected 1 AttachedTo, got %d", attachedCount)
	}
}

func TestEBS_DiscoverPersistence_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	mk := func() *EBS {
		return NewEBSWithClient(&fakeEC2Volumes{pages: [][]ec2types.Volume{{
			mkVolume("vol-1", "us-east-1a", "gp3", 10, 0, false, nil, []string{"i-1"}),
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

func TestEBS_DiscoverPersistence_APIError(t *testing.T) {
	boom := errors.New("AccessDenied")
	c := NewEBSWithClient(&fakeEC2Volumes{err: boom})
	_, err := c.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestEBS_DiscoverPersistence_EmptyScope(t *testing.T) {
	c := NewEBSWithClient(&fakeEC2Volumes{})
	if _, err := c.DiscoverPersistence(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}
