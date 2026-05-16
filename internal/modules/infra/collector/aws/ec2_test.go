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

// fakeEC2Instances simula DescribeInstances. Quando pages tem >1 elemento,
// cada chamada serve uma página com NextToken apontando para a próxima,
// até esgotar.
type fakeEC2Instances struct {
	pages [][]ec2types.Instance
	err   error
	calls int
}

func (f *fakeEC2Instances) DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.pages) {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	idx := f.calls
	f.calls++
	out := &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{{Instances: f.pages[idx]}},
	}
	if idx < len(f.pages)-1 {
		tok := "tok"
		out.NextToken = &tok
	}
	return out, nil
}

func mkTopo(account, region string, zones []string, clock time.Time) collector.ScopeTopology {
	topo := collector.ScopeTopology{}
	accURN := node.NewURN(node.ProviderAWS, account, node.KindAccount, account)
	regURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	topo.Account = node.Account{Base: node.Base{NodeURN: accURN, NodeKind: node.KindAccount, NodeMeta: node.Meta{Version: 1, ValidFrom: clock, ObservedAt: clock}}}
	topo.Region = node.Region{Base: node.Base{NodeURN: regURN, NodeKind: node.KindRegion, NodeMeta: node.Meta{Version: 1, ValidFrom: clock, ObservedAt: clock}}, Code: region}
	for _, z := range zones {
		zURN := node.NewURN(node.ProviderAWS, account, node.KindZone, z)
		topo.Zones = append(topo.Zones, node.Zone{Base: node.Base{NodeURN: zURN, NodeKind: node.KindZone, NodeMeta: node.Meta{Version: 1, ValidFrom: clock, ObservedAt: clock}}, RegionURN: regURN, Code: z})
	}
	return topo
}

func mkInstance(id, instType, az, state string, tags map[string]string) ec2types.Instance {
	idCopy := id
	azCopy := az
	out := ec2types.Instance{
		InstanceId:   &idCopy,
		InstanceType: ec2types.InstanceType(instType),
		Placement:    &ec2types.Placement{AvailabilityZone: &azCopy},
		State:        &ec2types.InstanceState{Name: ec2types.InstanceStateName(state)},
	}
	for k, v := range tags {
		kk, vv := k, v
		out.Tags = append(out.Tags, ec2types.Tag{Key: &kk, Value: &vv})
	}
	return out
}

func TestEC2_DiscoverCompute_HappyPath(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := mkTopo("123456789012", "us-east-1", []string{"us-east-1a", "us-east-1b"}, clock)

	insts := []ec2types.Instance{
		mkInstance("i-aaa", "m6i.large", "us-east-1a", "running", map[string]string{"env": "prod", "team": "core"}),
		mkInstance("i-bbb", "t3.micro", "us-east-1b", "stopped", nil),
	}
	ec2c := NewEC2WithClient(&fakeEC2Instances{pages: [][]ec2types.Instance{insts}}).WithClock(fixedClock(clock))

	got, err := ec2c.DiscoverCompute(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Computes) != 2 {
		t.Fatalf("expected 2 computes, got %d", len(got.Computes))
	}
	if len(got.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(got.Edges))
	}

	// i-aaa
	c0 := got.Computes[0]
	if c0.URN() != "urn:ce:aws:123456789012:compute/i-aaa" {
		t.Fatalf("compute[0] URN = %q", c0.URN())
	}
	if c0.ExternalID() != "i-aaa" || c0.InstanceType != "m6i.large" {
		t.Fatalf("compute[0] fields wrong: %+v", c0)
	}
	if c0.State != node.StateRunning {
		t.Fatalf("compute[0] state = %q", c0.State)
	}
	if c0.Tags["env"] != "prod" || c0.Tags["team"] != "core" {
		t.Fatalf("compute[0] tags wrong: %+v", c0.Tags)
	}
	if c0.Flavor != node.ComputeVM {
		t.Fatalf("compute[0] flavor = %q", c0.Flavor)
	}
	if c0.Meta().Version != 1 || !c0.Meta().IsCurrent() {
		t.Fatal("compute[0] meta wrong")
	}

	// Edge: zone us-east-1a → compute i-aaa
	wantFrom := node.NewURN(node.ProviderAWS, "123456789012", node.KindZone, "us-east-1a")
	wantTo := node.URN("urn:ce:aws:123456789012:compute/i-aaa")
	var foundEdge edge.Contains
	for _, e := range got.Edges {
		if e.From() == wantFrom && e.To() == wantTo {
			foundEdge = e
		}
	}
	if foundEdge.ID() == "" {
		t.Fatalf("missing edge zone→compute for i-aaa; got edges %+v", got.Edges)
	}
	if foundEdge.Type() != edge.TypeContains {
		t.Fatalf("edge type = %q", foundEdge.Type())
	}
	if !foundEdge.Meta().Directional {
		t.Fatal("Contains must be directional")
	}

	// i-bbb stopped
	c1 := got.Computes[1]
	if c1.State != node.StateStopped {
		t.Fatalf("compute[1] state = %q", c1.State)
	}
}

func TestEC2_DiscoverCompute_Pagination(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)

	page1 := []ec2types.Instance{
		mkInstance("i-1", "t3.nano", "us-east-1a", "running", nil),
		mkInstance("i-2", "t3.nano", "us-east-1a", "running", nil),
	}
	page2 := []ec2types.Instance{
		mkInstance("i-3", "t3.nano", "us-east-1a", "running", nil),
	}
	fake := &fakeEC2Instances{pages: [][]ec2types.Instance{page1, page2}}
	ec2c := NewEC2WithClient(fake).WithClock(fixedClock(clock))

	got, err := ec2c.DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Computes) != 3 {
		t.Fatalf("expected 3 computes across pages, got %d", len(got.Computes))
	}
	if fake.calls != 2 {
		t.Fatalf("expected 2 calls (paginated), got %d", fake.calls)
	}
}

func TestEC2_DiscoverCompute_DeterministicEdgeID(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	mk := func() *EC2 {
		return NewEC2WithClient(&fakeEC2Instances{pages: [][]ec2types.Instance{{
			mkInstance("i-1", "t3.nano", "us-east-1a", "running", nil),
		}}}).WithClock(fixedClock(clock))
	}
	a, _ := mk().DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	b, _ := mk().DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if a.Edges[0].ID() != b.Edges[0].ID() {
		t.Fatalf("expected deterministic edge ID, got %q vs %q", a.Edges[0].ID(), b.Edges[0].ID())
	}
}

func TestEC2_DiscoverCompute_SkipsBlankInstanceID(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	blank := ""
	insts := []ec2types.Instance{
		{InstanceId: &blank},
		mkInstance("i-1", "t3.nano", "us-east-1a", "running", nil),
	}
	ec2c := NewEC2WithClient(&fakeEC2Instances{pages: [][]ec2types.Instance{insts}}).WithClock(fixedClock(clock))
	got, _ := ec2c.DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Computes) != 1 {
		t.Fatalf("expected 1 compute (blank skipped), got %d", len(got.Computes))
	}
}

func TestEC2_DiscoverCompute_UnknownAZ_NoEdge(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock) // só 1a no topo
	insts := []ec2types.Instance{
		mkInstance("i-1", "t3.nano", "us-east-1z", "running", nil), // 1z não está no topo
	}
	ec2c := NewEC2WithClient(&fakeEC2Instances{pages: [][]ec2types.Instance{insts}}).WithClock(fixedClock(clock))
	got, _ := ec2c.DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if len(got.Computes) != 1 {
		t.Fatalf("expected 1 compute, got %d", len(got.Computes))
	}
	if len(got.Edges) != 0 {
		t.Fatalf("expected 0 edges (AZ unknown), got %d", len(got.Edges))
	}
}

func TestEC2_DiscoverCompute_APIError(t *testing.T) {
	clock := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	topo := mkTopo("1", "us-east-1", []string{"us-east-1a"}, clock)
	boom := errors.New("AccessDenied")
	ec2c := NewEC2WithClient(&fakeEC2Instances{err: boom}).WithClock(fixedClock(clock))
	_, err := ec2c.DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, topo)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestEC2_DiscoverCompute_EmptyScope(t *testing.T) {
	ec2c := NewEC2WithClient(&fakeEC2Instances{})
	if _, err := ec2c.DiscoverCompute(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty scope")
	}
}
