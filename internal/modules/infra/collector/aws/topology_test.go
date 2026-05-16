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

// fakeEC2 retorna AZs predefinidas (ou erro) no DescribeAvailabilityZones.
type fakeEC2 struct {
	zones []string
	err   error
}

func (f fakeEC2) DescribeAvailabilityZones(ctx context.Context, in *ec2.DescribeAvailabilityZonesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	azs := make([]ec2types.AvailabilityZone, 0, len(f.zones))
	for _, z := range f.zones {
		name := z
		azs = append(azs, ec2types.AvailabilityZone{ZoneName: &name})
	}
	return &ec2.DescribeAvailabilityZonesOutput{AvailabilityZones: azs}, nil
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestTopology_DiscoverScope_HappyPath(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := NewTopologyWithEC2(fakeEC2{zones: []string{"us-east-1a", "us-east-1b", "us-east-1c"}}).WithClock(fixedClock(clock))

	got, err := topo.DiscoverScope(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Account
	if got.Account.URN() != "urn:ce:aws:123456789012:account/123456789012" {
		t.Fatalf("account URN = %q", got.Account.URN())
	}
	if got.Account.ExternalID != "123456789012" || got.Account.ProviderID != node.ProviderAWS {
		t.Fatalf("account fields wrong: %+v", got.Account)
	}
	if got.Account.Meta().Version != 1 || !got.Account.Meta().IsCurrent() {
		t.Fatal("account meta wrong")
	}

	// Region
	if got.Region.URN() != "urn:ce:aws:123456789012:region/us-east-1" {
		t.Fatalf("region URN = %q", got.Region.URN())
	}
	if got.Region.Code != "us-east-1" {
		t.Fatalf("region code = %q", got.Region.Code)
	}

	// Zones
	if len(got.Zones) != 3 {
		t.Fatalf("expected 3 zones, got %d", len(got.Zones))
	}
	wantZones := map[string]bool{"us-east-1a": false, "us-east-1b": false, "us-east-1c": false}
	for _, z := range got.Zones {
		if _, ok := wantZones[z.Code]; !ok {
			t.Fatalf("unexpected zone %q", z.Code)
		}
		wantZones[z.Code] = true
		if z.RegionURN != got.Region.URN() {
			t.Fatalf("zone %q points to wrong region URN: %q", z.Code, z.RegionURN)
		}
		if z.URN() != node.URN("urn:ce:aws:123456789012:zone/"+z.Code) {
			t.Fatalf("zone %q URN wrong: %q", z.Code, z.URN())
		}
	}
	for code, seen := range wantZones {
		if !seen {
			t.Fatalf("zone %q missing", code)
		}
	}

	// Edges: 1 Account→Region + 3 Region→Zone = 4
	if len(got.Edges) != 4 {
		t.Fatalf("expected 4 edges, got %d", len(got.Edges))
	}

	// Verifica primeira edge: Account → Region
	first := got.Edges[0]
	if first.Type() != edge.TypeContains {
		t.Fatalf("first edge type = %q", first.Type())
	}
	if first.From() != got.Account.URN() || first.To() != got.Region.URN() {
		t.Fatalf("first edge endpoints: %q → %q", first.From(), first.To())
	}
	if first.ID() == "" {
		t.Fatal("first edge ID empty")
	}
	if !first.Meta().Directional {
		t.Fatal("Contains must be directional")
	}

	// Edges Region→Zone: cada zona deve ter exatamente uma edge entrando do Region.
	regionToZoneCount := map[node.URN]int{}
	for _, e := range got.Edges[1:] {
		if e.From() != got.Region.URN() {
			t.Fatalf("expected From=Region, got %q", e.From())
		}
		regionToZoneCount[e.To()]++
	}
	for _, z := range got.Zones {
		if regionToZoneCount[z.URN()] != 1 {
			t.Fatalf("zone %q has %d incoming edges, want 1", z.URN(), regionToZoneCount[z.URN()])
		}
	}
}

func TestTopology_DiscoverScope_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	mk := func() *Topology {
		return NewTopologyWithEC2(fakeEC2{zones: []string{"us-east-1a"}}).WithClock(fixedClock(clock))
	}

	a, err := mk().DiscoverScope(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := mk().DiscoverScope(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}

	if len(a.Edges) != len(b.Edges) {
		t.Fatalf("edge count differs: %d vs %d", len(a.Edges), len(b.Edges))
	}
	for i := range a.Edges {
		if a.Edges[i].ID() != b.Edges[i].ID() {
			t.Fatalf("edge[%d] ID differs: %q vs %q", i, a.Edges[i].ID(), b.Edges[i].ID())
		}
	}
}

func TestTopology_DiscoverScope_EmptyScope(t *testing.T) {
	topo := NewTopologyWithEC2(fakeEC2{})
	if _, err := topo.DiscoverScope(context.Background(), collector.Scope{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
	if _, err := topo.DiscoverScope(context.Background(), collector.Scope{Account: "1"}); err == nil {
		t.Fatal("expected error when Region missing")
	}
	if _, err := topo.DiscoverScope(context.Background(), collector.Scope{Region: "us-east-1"}); err == nil {
		t.Fatal("expected error when Account missing")
	}
}

func TestTopology_DiscoverScope_EC2Error(t *testing.T) {
	boom := errors.New("UnauthorizedOperation")
	topo := NewTopologyWithEC2(fakeEC2{err: boom})
	_, err := topo.DiscoverScope(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestTopology_DiscoverScope_SkipsBlankZoneNames(t *testing.T) {
	topo := NewTopologyWithEC2(fakeEC2{zones: []string{"us-east-1a", ""}})
	got, err := topo.DiscoverScope(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Zones) != 1 {
		t.Fatalf("expected 1 zone (blank skipped), got %d", len(got.Zones))
	}
	// 1 Account→Region + 1 Region→Zone = 2 edges
	if len(got.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(got.Edges))
	}
}
