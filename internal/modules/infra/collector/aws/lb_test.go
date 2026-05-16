package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

type fakeELBv2 struct {
	pages [][]elbtypes.LoadBalancer
	err   error
	calls int
}

func (f *fakeELBv2) DescribeLoadBalancers(ctx context.Context, in *elbv2.DescribeLoadBalancersInput, opts ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.pages) {
		return &elbv2.DescribeLoadBalancersOutput{}, nil
	}
	idx := f.calls
	f.calls++
	out := &elbv2.DescribeLoadBalancersOutput{LoadBalancers: f.pages[idx]}
	if idx < len(f.pages)-1 {
		m := "marker"
		out.NextMarker = &m
	}
	return out, nil
}

func mkLBRaw(arn, name, vpcID string, lbType elbtypes.LoadBalancerTypeEnum, scheme elbtypes.LoadBalancerSchemeEnum, azNames []string) elbtypes.LoadBalancer {
	arnCopy := arn
	nameCopy := name
	vpc := vpcID
	lb := elbtypes.LoadBalancer{
		LoadBalancerArn:  &arnCopy,
		LoadBalancerName: &nameCopy,
		Type:             lbType,
		Scheme:           scheme,
	}
	if vpcID != "" {
		lb.VpcId = &vpc
	}
	for _, az := range azNames {
		zn := az
		lb.AvailabilityZones = append(lb.AvailabilityZones, elbtypes.AvailabilityZone{ZoneName: &zn})
	}
	return lb
}

func TestLB_DiscoverNetwork_HappyPath(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	topo := mkTopo("123456789012", "us-east-1", []string{"us-east-1a", "us-east-1b"}, clock)

	lbs := []elbtypes.LoadBalancer{
		mkLBRaw(
			"arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/web/abc",
			"web",
			"vpc-1",
			elbtypes.LoadBalancerTypeEnumApplication,
			elbtypes.LoadBalancerSchemeEnumInternetFacing,
			[]string{"us-east-1a", "us-east-1b"},
		),
	}
	c := NewLBWithClient(&fakeELBv2{pages: [][]elbtypes.LoadBalancer{lbs}}).WithClock(fixedClock(clock))
	got, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(got.Networks))
	}
	n := got.Networks[0]
	if n.Flavor != node.NetworkLB {
		t.Fatalf("flavor = %q", n.Flavor)
	}
	if n.Tags["lb:type"] != "application" || n.Tags["lb:scheme"] != "internet-facing" {
		t.Fatalf("tags = %+v", n.Tags)
	}
	// Edges esperadas: Region→LB, VPC→LB, Zone-a→LB, Zone-b→LB = 4
	if len(got.Edges) != 4 {
		t.Fatalf("expected 4 edges, got %d", len(got.Edges))
	}
	seenKinds := map[node.Kind]int{}
	for _, e := range got.Edges {
		if e.Type() != edge.TypeContains {
			t.Fatalf("expected Contains, got %q", e.Type())
		}
		from, _ := node.ParseURN(e.From())
		seenKinds[from.Kind]++
	}
	if seenKinds[node.KindRegion] != 1 || seenKinds[node.KindNetwork] != 1 || seenKinds[node.KindZone] != 2 {
		t.Fatalf("edge kinds = %+v, want Region:1 Network(VPC):1 Zone:2", seenKinds)
	}
}

func TestLB_DiscoverNetwork_NoVpcID_NoVpcEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	lbs := []elbtypes.LoadBalancer{
		mkLBRaw("arn:lb", "lb", "", elbtypes.LoadBalancerTypeEnumNetwork, "", []string{"us-east-1a"}),
	}
	c := NewLBWithClient(&fakeELBv2{pages: [][]elbtypes.LoadBalancer{lbs}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	// Edges: Region→LB + Zone→LB = 2 (sem VPC)
	if len(got.Edges) != 2 {
		t.Fatalf("expected 2 edges (no VPC), got %d", len(got.Edges))
	}
}

func TestLB_DiscoverNetwork_UnknownAZ_SkipsZoneEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock) // só 1a
	lbs := []elbtypes.LoadBalancer{
		mkLBRaw("arn:lb", "lb", "vpc-1", elbtypes.LoadBalancerTypeEnumApplication, "", []string{"us-east-1a", "us-east-1z"}),
	}
	c := NewLBWithClient(&fakeELBv2{pages: [][]elbtypes.LoadBalancer{lbs}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	// Edges: Region + VPC + Zone-a (us-east-1z é pulada) = 3
	if len(got.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(got.Edges))
	}
}

func TestLB_DiscoverNetwork_Pagination(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	p1 := []elbtypes.LoadBalancer{mkLBRaw("arn:1", "n1", "vpc-1", elbtypes.LoadBalancerTypeEnumApplication, "", nil)}
	p2 := []elbtypes.LoadBalancer{mkLBRaw("arn:2", "n2", "vpc-1", elbtypes.LoadBalancerTypeEnumApplication, "", nil)}
	fake := &fakeELBv2{pages: [][]elbtypes.LoadBalancer{p1, p2}}
	c := NewLBWithClient(fake).WithClock(fixedClock(clock))
	got, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Networks) != 2 || fake.calls != 2 {
		t.Fatalf("expected 2 networks across 2 calls, got %d/%d", len(got.Networks), fake.calls)
	}
}

func TestLB_DiscoverNetwork_ExtIDFallbackToName(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	name := "named-lb"
	lb := elbtypes.LoadBalancer{LoadBalancerName: &name, Type: elbtypes.LoadBalancerTypeEnumApplication}
	c := NewLBWithClient(&fakeELBv2{pages: [][]elbtypes.LoadBalancer{{lb}}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if len(got.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(got.Networks))
	}
	if got.Networks[0].ExtID != "named-lb" {
		t.Fatalf("expected ExtID=named-lb, got %q", got.Networks[0].ExtID)
	}
}

func TestLB_DiscoverNetwork_SkipsBlank(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	lbs := []elbtypes.LoadBalancer{
		{}, // sem ARN nem Name
		mkLBRaw("arn:ok", "ok", "vpc-1", elbtypes.LoadBalancerTypeEnumApplication, "", nil),
	}
	c := NewLBWithClient(&fakeELBv2{pages: [][]elbtypes.LoadBalancer{lbs}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if len(got.Networks) != 1 {
		t.Fatalf("expected 1 network (blank skipped), got %d", len(got.Networks))
	}
}

func TestLB_DiscoverNetwork_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 16, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	mk := func() *LB {
		return NewLBWithClient(&fakeELBv2{pages: [][]elbtypes.LoadBalancer{{
			mkLBRaw("arn:1", "n", "vpc-1", elbtypes.LoadBalancerTypeEnumApplication, "", []string{"us-east-1a"}),
		}}}).WithClock(fixedClock(clock))
	}
	a, _ := mk().DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	b, _ := mk().DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(a.Edges) != len(b.Edges) {
		t.Fatalf("edge count differs: %d vs %d", len(a.Edges), len(b.Edges))
	}
	for i := range a.Edges {
		if a.Edges[i].ID() != b.Edges[i].ID() {
			t.Fatalf("edge[%d] ID differs", i)
		}
	}
}

func TestLB_DiscoverNetwork_APIError(t *testing.T) {
	boom := errors.New("AccessDenied")
	c := NewLBWithClient(&fakeELBv2{err: boom})
	_, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestLB_DiscoverNetwork_EmptyScope(t *testing.T) {
	c := NewLBWithClient(&fakeELBv2{})
	if _, err := c.DiscoverNetwork(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}
