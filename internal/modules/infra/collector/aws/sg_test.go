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

type fakeEC2SG struct {
	pages [][]ec2types.SecurityGroup
	err   error
	calls int
}

func (f *fakeEC2SG) DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.pages) {
		return &ec2.DescribeSecurityGroupsOutput{}, nil
	}
	idx := f.calls
	f.calls++
	out := &ec2.DescribeSecurityGroupsOutput{SecurityGroups: f.pages[idx]}
	if idx < len(f.pages)-1 {
		t := "tok"
		out.NextToken = &t
	}
	return out, nil
}

func mkSGRaw(id, vpcID string, tags map[string]string) ec2types.SecurityGroup {
	idCopy := id
	g := ec2types.SecurityGroup{GroupId: &idCopy}
	if vpcID != "" {
		v := vpcID
		g.VpcId = &v
	}
	for k, val := range tags {
		kk, vv := k, val
		g.Tags = append(g.Tags, ec2types.Tag{Key: &kk, Value: &vv})
	}
	return g
}

func TestSG_DiscoverNetwork_HappyPath(t *testing.T) {
	clock := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	topo := mkTopo("123456789012", "us-east-1", []string{"us-east-1a"}, clock)

	sgs := []ec2types.SecurityGroup{
		mkSGRaw("sg-aaa", "vpc-1", map[string]string{"env": "prod"}),
		mkSGRaw("sg-bbb", "vpc-1", nil),
	}
	c := NewSGWithClient(&fakeEC2SG{pages: [][]ec2types.SecurityGroup{sgs}}).WithClock(fixedClock(clock))

	got, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got.Networks) != 2 || len(got.Edges) != 2 {
		t.Fatalf("expected 2 networks + 2 edges, got %d/%d", len(got.Networks), len(got.Edges))
	}
	if got.Networks[0].Flavor != node.NetworkSecurityGroup {
		t.Fatalf("flavor = %q", got.Networks[0].Flavor)
	}
	if got.Networks[0].Tags["env"] != "prod" {
		t.Fatalf("tags = %+v", got.Networks[0].Tags)
	}
	vpcURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindNetwork, "vpc-1")
	for _, e := range got.Edges {
		if e.Type() != edge.TypeContains {
			t.Fatalf("expected Contains, got %q", e.Type())
		}
		if e.From() != vpcURN {
			t.Fatalf("expected From=VPC URN, got %q", e.From())
		}
	}
}

func TestSG_DiscoverNetwork_NoVpcID_NoEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	sgs := []ec2types.SecurityGroup{mkSGRaw("sg-classic", "", nil)}
	c := NewSGWithClient(&fakeEC2SG{pages: [][]ec2types.SecurityGroup{sgs}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if len(got.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(got.Networks))
	}
	if len(got.Edges) != 0 {
		t.Fatalf("expected 0 edges (no VpcId), got %d", len(got.Edges))
	}
}

func TestSG_DiscoverNetwork_Pagination(t *testing.T) {
	clock := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	p1 := []ec2types.SecurityGroup{mkSGRaw("sg-1", "vpc-1", nil)}
	p2 := []ec2types.SecurityGroup{mkSGRaw("sg-2", "vpc-1", nil)}
	fake := &fakeEC2SG{pages: [][]ec2types.SecurityGroup{p1, p2}}
	c := NewSGWithClient(fake).WithClock(fixedClock(clock))
	got, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Networks) != 2 || fake.calls != 2 {
		t.Fatalf("expected 2 networks across 2 calls, got %d/%d", len(got.Networks), fake.calls)
	}
}

func TestSG_DiscoverNetwork_SkipsBlankID(t *testing.T) {
	clock := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	blank := ""
	sgs := []ec2types.SecurityGroup{
		{GroupId: &blank},
		mkSGRaw("sg-ok", "vpc-1", nil),
	}
	c := NewSGWithClient(&fakeEC2SG{pages: [][]ec2types.SecurityGroup{sgs}}).WithClock(fixedClock(clock))
	got, _ := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if len(got.Networks) != 1 {
		t.Fatalf("expected 1 network (blank skipped), got %d", len(got.Networks))
	}
}

func TestSG_DiscoverNetwork_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	mk := func() *SG {
		return NewSGWithClient(&fakeEC2SG{pages: [][]ec2types.SecurityGroup{{
			mkSGRaw("sg-1", "vpc-1", nil),
		}}}).WithClock(fixedClock(clock))
	}
	a, _ := mk().DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	b, _ := mk().DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if a.Edges[0].ID() != b.Edges[0].ID() {
		t.Fatalf("edge ID not deterministic: %q vs %q", a.Edges[0].ID(), b.Edges[0].ID())
	}
}

func TestSG_DiscoverNetwork_APIError(t *testing.T) {
	boom := errors.New("AccessDenied")
	c := NewSGWithClient(&fakeEC2SG{err: boom})
	_, err := c.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestSG_DiscoverNetwork_EmptyScope(t *testing.T) {
	c := NewSGWithClient(&fakeEC2SG{})
	if _, err := c.DiscoverNetwork(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}
