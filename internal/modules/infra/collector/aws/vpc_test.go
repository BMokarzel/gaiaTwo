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

// fakeEC2VPC simula DescribeVpcs + DescribeSubnets paginados.
type fakeEC2VPC struct {
	vpcPages    [][]ec2types.Vpc
	subnetPages [][]ec2types.Subnet
	vpcErr      error
	subnetErr   error
	vpcCalls    int
	subnetCalls int
}

func (f *fakeEC2VPC) DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if f.vpcErr != nil {
		return nil, f.vpcErr
	}
	if f.vpcCalls >= len(f.vpcPages) {
		return &ec2.DescribeVpcsOutput{}, nil
	}
	idx := f.vpcCalls
	f.vpcCalls++
	out := &ec2.DescribeVpcsOutput{Vpcs: f.vpcPages[idx]}
	if idx < len(f.vpcPages)-1 {
		t := "tok"
		out.NextToken = &t
	}
	return out, nil
}

func (f *fakeEC2VPC) DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if f.subnetErr != nil {
		return nil, f.subnetErr
	}
	if f.subnetCalls >= len(f.subnetPages) {
		return &ec2.DescribeSubnetsOutput{}, nil
	}
	idx := f.subnetCalls
	f.subnetCalls++
	out := &ec2.DescribeSubnetsOutput{Subnets: f.subnetPages[idx]}
	if idx < len(f.subnetPages)-1 {
		t := "tok"
		out.NextToken = &t
	}
	return out, nil
}

func mkVpcRaw(id, cidr string, tags map[string]string) ec2types.Vpc {
	idCopy := id
	cidrCopy := cidr
	v := ec2types.Vpc{VpcId: &idCopy, CidrBlock: &cidrCopy}
	for k, val := range tags {
		kk, vv := k, val
		v.Tags = append(v.Tags, ec2types.Tag{Key: &kk, Value: &vv})
	}
	return v
}

func mkSubnetRaw(id, cidr, az, vpcID string, tags map[string]string) ec2types.Subnet {
	idCopy := id
	cidrCopy := cidr
	azCopy := az
	vpcIDCopy := vpcID
	s := ec2types.Subnet{
		SubnetId:         &idCopy,
		CidrBlock:        &cidrCopy,
		AvailabilityZone: &azCopy,
		VpcId:            &vpcIDCopy,
	}
	for k, val := range tags {
		kk, vv := k, val
		s.Tags = append(s.Tags, ec2types.Tag{Key: &kk, Value: &vv})
	}
	return s
}

func TestVPC_DiscoverNetwork_HappyPath(t *testing.T) {
	clock := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	topo := mkTopo("123456789012", "us-east-1", []string{"us-east-1a", "us-east-1b"}, clock)

	vpcs := []ec2types.Vpc{
		mkVpcRaw("vpc-aaa", "10.0.0.0/16", map[string]string{"env": "prod"}),
	}
	subnets := []ec2types.Subnet{
		mkSubnetRaw("subnet-1a", "10.0.1.0/24", "us-east-1a", "vpc-aaa", nil),
		mkSubnetRaw("subnet-1b", "10.0.2.0/24", "us-east-1b", "vpc-aaa", nil),
	}

	c := NewVPCWithClient(&fakeEC2VPC{
		vpcPages:    [][]ec2types.Vpc{vpcs},
		subnetPages: [][]ec2types.Subnet{subnets},
	}).WithClock(fixedClock(clock))

	got, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// 1 VPC + 2 Subnets
	if len(got.Networks) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(got.Networks))
	}

	// Edges esperadas:
	//   Region → VPC                            (1)
	//   Zone-a → subnet-1a + VPC → subnet-1a    (2)
	//   Zone-b → subnet-1b + VPC → subnet-1b    (2)
	// = 5 total
	if len(got.Edges) != 5 {
		t.Fatalf("expected 5 edges, got %d", len(got.Edges))
	}

	vpcURN := node.URN("urn:ce:aws:123456789012:network/vpc-aaa")
	regionURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindRegion, "us-east-1")
	zoneA := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1a")
	zoneB := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1b")
	sub1A := node.URN("urn:ce:aws:123456789012:network/subnet-1a")
	sub1B := node.URN("urn:ce:aws:123456789012:network/subnet-1b")

	// Verifica VPC fields
	var vpc node.Network
	for _, n := range got.Networks {
		if n.URN() == vpcURN {
			vpc = n
		}
	}
	if vpc.URN() == "" {
		t.Fatal("VPC node not found")
	}
	if vpc.Flavor != node.NetworkVPC {
		t.Fatalf("vpc flavor = %q", vpc.Flavor)
	}
	if vpc.CIDR != "10.0.0.0/16" {
		t.Fatalf("vpc CIDR = %q", vpc.CIDR)
	}
	if vpc.Tags["env"] != "prod" {
		t.Fatalf("vpc tags = %+v", vpc.Tags)
	}

	seen := map[string]bool{}
	for _, e := range got.Edges {
		if e.Type() != edge.TypeContains {
			t.Fatalf("expected Contains, got %q", e.Type())
		}
		seen[string(e.From())+"→"+string(e.To())] = true
	}

	for _, key := range []string{
		string(regionURN) + "→" + string(vpcURN),
		string(zoneA) + "→" + string(sub1A),
		string(vpcURN) + "→" + string(sub1A),
		string(zoneB) + "→" + string(sub1B),
		string(vpcURN) + "→" + string(sub1B),
	} {
		if !seen[key] {
			t.Fatalf("missing edge: %s", key)
		}
	}
}

func TestVPC_DiscoverNetwork_Pagination(t *testing.T) {
	clock := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)

	vpcPage1 := []ec2types.Vpc{mkVpcRaw("vpc-1", "10.0.0.0/16", nil)}
	vpcPage2 := []ec2types.Vpc{mkVpcRaw("vpc-2", "10.1.0.0/16", nil)}
	subPage1 := []ec2types.Subnet{mkSubnetRaw("subnet-1", "10.0.1.0/24", "us-east-1a", "vpc-1", nil)}
	subPage2 := []ec2types.Subnet{mkSubnetRaw("subnet-2", "10.1.1.0/24", "us-east-1a", "vpc-2", nil)}

	fake := &fakeEC2VPC{
		vpcPages:    [][]ec2types.Vpc{vpcPage1, vpcPage2},
		subnetPages: [][]ec2types.Subnet{subPage1, subPage2},
	}
	c := NewVPCWithClient(fake).WithClock(fixedClock(clock))

	got, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Networks) != 4 || fake.vpcCalls != 2 || fake.subnetCalls != 2 {
		t.Fatalf("expected 4 networks across 2+2 calls, got %d networks, %d/%d calls",
			len(got.Networks), fake.vpcCalls, fake.subnetCalls)
	}
}

func TestVPC_DiscoverNetwork_SubnetUnknownAZ_NoZoneEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock) // só 1a
	vpcs := []ec2types.Vpc{mkVpcRaw("vpc-1", "10.0.0.0/16", nil)}
	subnets := []ec2types.Subnet{
		mkSubnetRaw("subnet-z", "10.0.9.0/24", "us-east-1z", "vpc-1", nil),
	}
	c := NewVPCWithClient(&fakeEC2VPC{
		vpcPages:    [][]ec2types.Vpc{vpcs},
		subnetPages: [][]ec2types.Subnet{subnets},
	}).WithClock(fixedClock(clock))

	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)

	// Edges: Region→VPC + VPC→Subnet = 2 (sem Zone→Subnet)
	if len(got.Edges) != 2 {
		t.Fatalf("expected 2 edges (no Zone edge for unknown AZ), got %d", len(got.Edges))
	}
	for _, e := range got.Edges {
		fromKind, _ := node.ParseURN(e.From())
		if fromKind.Kind == node.KindZone {
			t.Fatalf("unexpected Zone→ edge: %s", e.ID())
		}
	}
}

func TestVPC_DiscoverNetwork_SubnetUnknownVPC_NoVPCEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	// Sem VPCs no batch — subnet referencia vpc-x que não existe.
	subnets := []ec2types.Subnet{
		mkSubnetRaw("subnet-orphan", "10.0.1.0/24", "us-east-1a", "vpc-x", nil),
	}
	c := NewVPCWithClient(&fakeEC2VPC{
		vpcPages:    [][]ec2types.Vpc{nil},
		subnetPages: [][]ec2types.Subnet{subnets},
	}).WithClock(fixedClock(clock))

	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	// Apenas Zone→Subnet (1 edge), sem VPC→Subnet (vpc-x não está no batch).
	if len(got.Networks) != 1 {
		t.Fatalf("expected 1 network (subnet only), got %d", len(got.Networks))
	}
	if len(got.Edges) != 1 {
		t.Fatalf("expected 1 edge (zone→subnet only), got %d", len(got.Edges))
	}
}

func TestVPC_DiscoverNetwork_SkipsBlankIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	blank := ""
	cidr := "10.0.0.0/16"
	vpcs := []ec2types.Vpc{
		{VpcId: &blank, CidrBlock: &cidr},
		mkVpcRaw("vpc-ok", "10.1.0.0/16", nil),
	}
	subnets := []ec2types.Subnet{
		{SubnetId: &blank, AvailabilityZone: stringPtr("us-east-1a"), VpcId: stringPtr("vpc-ok")},
		mkSubnetRaw("subnet-ok", "10.1.1.0/24", "us-east-1a", "vpc-ok", nil),
	}
	c := NewVPCWithClient(&fakeEC2VPC{
		vpcPages:    [][]ec2types.Vpc{vpcs},
		subnetPages: [][]ec2types.Subnet{subnets},
	}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Networks) != 2 {
		t.Fatalf("expected 2 networks (blanks skipped), got %d", len(got.Networks))
	}
}

func TestVPC_DiscoverNetwork_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	mk := func() *VPC {
		return NewVPCWithClient(&fakeEC2VPC{
			vpcPages:    [][]ec2types.Vpc{{mkVpcRaw("vpc-1", "10.0.0.0/16", nil)}},
			subnetPages: [][]ec2types.Subnet{{mkSubnetRaw("subnet-1", "10.0.1.0/24", "us-east-1a", "vpc-1", nil)}},
		}).WithClock(fixedClock(clock))
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

func TestVPC_DiscoverNetwork_VPCsAPIError(t *testing.T) {
	boom := errors.New("AccessDenied")
	c := NewVPCWithClient(&fakeEC2VPC{vpcErr: boom})
	_, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestVPC_DiscoverNetwork_SubnetsAPIError(t *testing.T) {
	boom := errors.New("AccessDenied")
	c := NewVPCWithClient(&fakeEC2VPC{
		vpcPages:  [][]ec2types.Vpc{nil},
		subnetErr: boom,
	})
	_, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestVPC_DiscoverNetwork_EmptyScope(t *testing.T) {
	c := NewVPCWithClient(&fakeEC2VPC{})
	if _, err := c.DiscoverNetwork(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}

func stringPtr(s string) *string { return &s }
