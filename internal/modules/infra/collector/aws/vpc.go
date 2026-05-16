package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// EC2VPC é a sub-API EC2 necessária para descobrir componentes de rede
// (VPCs e Subnets). Declarada como interface para injeção de fake em
// testes. Vai crescer com S-007b (security groups, load balancers, ...).
type EC2VPC interface {
	DescribeVpcs(ctx context.Context, in *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
}

// VPC implementa collector.NetworkDiscoverer para VPCs e Subnets.
//
// Lista VPCs via DescribeVpcs (paginado por NextToken) e Subnets via
// DescribeSubnets (idem). Converte para node.Network:
//   - VPC      → flavor=vpc,    edge Region→VPC (Contains)
//   - Subnet   → flavor=subnet, edges Zone→Subnet + VPC→Subnet (Contains)
//
// Subnets cuja AZ não está no topo.Zones recebem apenas a edge VPC→Subnet
// (sem Zone). Subnets cujo VpcId não corresponde a nenhum VPC descoberto
// recebem apenas a edge Zone→Subnet (sem VPC). Isso evita poluir o batch
// com edges órfãs.
type VPC struct {
	cli EC2VPC
	now func() time.Time
}

// NewVPC constrói o discoverer a partir de awssdk.Config.
func NewVPC(cfg awssdk.Config) *VPC {
	return &VPC{cli: ec2.NewFromConfig(cfg), now: time.Now}
}

// NewVPCWithClient (testes) injeta um cliente EC2VPC.
func NewVPCWithClient(c EC2VPC) *VPC {
	return &VPC{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (v *VPC) WithClock(now func() time.Time) *VPC {
	v.now = now
	return v
}

// DiscoverNetwork lista VPCs e Subnets e devolve NetworkBatch.
func (v *VPC) DiscoverNetwork(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (collector.NetworkBatch, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.NetworkBatch{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	zoneByCode := make(map[string]node.URN, len(topo.Zones))
	for _, z := range topo.Zones {
		zoneByCode[z.Code] = z.URN()
	}
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)
	now := v.now().UTC()

	batch := collector.NetworkBatch{}

	// VPCs primeiro: precisamos do mapa VpcId → URN para amarrar subnets.
	vpcURNs := map[string]node.URN{}
	var vpcToken *string
	for {
		out, err := v.cli.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{NextToken: vpcToken})
		if err != nil {
			return collector.NetworkBatch{}, fmt.Errorf("describe vpcs: %w", err)
		}
		for _, raw := range out.Vpcs {
			n, e := v.toVPC(scope, raw, regionURN, now)
			if n.URN() == "" {
				continue
			}
			vpcURNs[*raw.VpcId] = n.URN()
			batch.Networks = append(batch.Networks, n)
			if e != nil {
				batch.Edges = append(batch.Edges, e)
			}
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		vpcToken = out.NextToken
	}

	// Subnets na sequência.
	var subnetToken *string
	for {
		out, err := v.cli.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{NextToken: subnetToken})
		if err != nil {
			return collector.NetworkBatch{}, fmt.Errorf("describe subnets: %w", err)
		}
		for _, raw := range out.Subnets {
			n, edges := v.toSubnet(scope, raw, zoneByCode, vpcURNs, now)
			if n.URN() == "" {
				continue
			}
			batch.Networks = append(batch.Networks, n)
			batch.Edges = append(batch.Edges, edges...)
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		subnetToken = out.NextToken
	}

	return batch, nil
}

// toVPC converte ec2types.Vpc → node.Network(flavor=vpc) + edge Region→VPC.
// Se VpcId vazio, retorna Network zero (URN==""), sinal para o caller pular.
func (v *VPC) toVPC(scope collector.Scope, raw ec2types.Vpc, regionURN node.URN, now time.Time) (node.Network, edge.Edge) {
	if raw.VpcId == nil || *raw.VpcId == "" {
		return node.Network{}, nil
	}
	id := *raw.VpcId

	tags := ec2TagsToMap(raw.Tags)
	cidr := ""
	if raw.CidrBlock != nil {
		cidr = *raw.CidrBlock
	}

	urn := node.NewURN(node.ProviderAWS, scope.Account, node.KindNetwork, id)
	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)

	n := node.Network{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindNetwork,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  now,
				ObservedAt: now,
				Confidence: 1,
				Source:     node.Source{Collector: "aws", Method: node.MethodAPI},
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    accountURN,
		Region:     regionURN,
		ExtID:      id,
		Flavor:     node.NetworkVPC,
		CIDR:       cidr,
		Tags:       tags,
	}

	containsEdge := edge.Contains{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(regionURN, edge.TypeContains, urn, now),
			EdgeType: edge.TypeContains,
			FromURN:  regionURN,
			ToURN:    urn,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
				Confidence:  1,
				Directional: true,
			},
		},
	}
	return n, containsEdge
}

// toSubnet converte ec2types.Subnet → node.Network(flavor=subnet) + edges
// Zone→Subnet (Contains) e VPC→Subnet (Contains). Cada edge é emitida
// apenas se o referente está disponível no batch corrente — evita
// arestas órfãs.
func (v *VPC) toSubnet(scope collector.Scope, raw ec2types.Subnet, zoneByCode map[string]node.URN, vpcURNs map[string]node.URN, now time.Time) (node.Network, []edge.Edge) {
	if raw.SubnetId == nil || *raw.SubnetId == "" {
		return node.Network{}, nil
	}
	id := *raw.SubnetId

	tags := ec2TagsToMap(raw.Tags)
	cidr := ""
	if raw.CidrBlock != nil {
		cidr = *raw.CidrBlock
	}

	urn := node.NewURN(node.ProviderAWS, scope.Account, node.KindNetwork, id)
	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)

	n := node.Network{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindNetwork,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  now,
				ObservedAt: now,
				Confidence: 1,
				Source:     node.Source{Collector: "aws", Method: node.MethodAPI},
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    accountURN,
		Region:     regionURN,
		ExtID:      id,
		Flavor:     node.NetworkSubnet,
		CIDR:       cidr,
		Tags:       tags,
	}

	var edges []edge.Edge

	// Zone → Subnet
	az := ""
	if raw.AvailabilityZone != nil {
		az = *raw.AvailabilityZone
	}
	if zoneURN, ok := zoneByCode[az]; ok {
		edges = append(edges, edge.Contains{
			Base: edge.Base{
				EdgeID:   edge.DeterministicID(zoneURN, edge.TypeContains, urn, now),
				EdgeType: edge.TypeContains,
				FromURN:  zoneURN,
				ToURN:    urn,
				EdgeMeta: edge.Meta{
					ValidFrom:   now,
					ObservedAt:  now,
					Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
					Confidence:  1,
					Directional: true,
				},
			},
		})
	}

	// VPC → Subnet
	vpcID := ""
	if raw.VpcId != nil {
		vpcID = *raw.VpcId
	}
	if vpcURN, ok := vpcURNs[vpcID]; ok {
		edges = append(edges, edge.Contains{
			Base: edge.Base{
				EdgeID:   edge.DeterministicID(vpcURN, edge.TypeContains, urn, now),
				EdgeType: edge.TypeContains,
				FromURN:  vpcURN,
				ToURN:    urn,
				EdgeMeta: edge.Meta{
					ValidFrom:   now,
					ObservedAt:  now,
					Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
					Confidence:  1,
					Directional: true,
				},
			},
		})
	}

	return n, edges
}

// ec2TagsToMap converte []ec2types.Tag em map[string]string. Tags sem
// Key são ignoradas; Value nil vira "".
func ec2TagsToMap(tags []ec2types.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for _, t := range tags {
		if t.Key == nil {
			continue
		}
		v := ""
		if t.Value != nil {
			v = *t.Value
		}
		out[*t.Key] = v
	}
	return out
}

// Compile-time check.
var _ collector.NetworkDiscoverer = (*VPC)(nil)
