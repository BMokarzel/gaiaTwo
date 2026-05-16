package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// ELBv2DescribeLoadBalancers é a sub-API ELBv2 necessária para descobrir
// Load Balancers (Application, Network, Gateway). Declarada como
// interface para injeção de fake em testes.
type ELBv2DescribeLoadBalancers interface {
	DescribeLoadBalancers(ctx context.Context, in *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
}

// LB implementa collector.NetworkDiscoverer para Load Balancers ELBv2.
//
// Lista LBs via DescribeLoadBalancers (paginado por Marker). Cada LB
// vive em uma VPC e estende-se por uma ou mais AZs (campo
// AvailabilityZones do output). Emitimos:
//   - Region → LB    (Contains): LB é regional por padrão
//   - VPC    → LB    (Contains): só se VpcId disponível (CLB clássico
//     pode estar fora de VPC mas não cabe no ELBv2)
//   - Zone   → LB    (Contains): uma edge por AZ presente em topo.Zones;
//     AZ desconhecida é pulada (sem edge órfã)
//
// O Engine/Flavor da Network é fixo `loadbalancer`; o subtype técnico
// (application/network/gateway) entra como Tag `lb:type` para não
// inflar o domínio de flavors antes da hora.
type LB struct {
	cli ELBv2DescribeLoadBalancers
	now func() time.Time
}

// NewLB constrói o discoverer a partir de awssdk.Config.
func NewLB(cfg awssdk.Config) *LB {
	return &LB{cli: elbv2.NewFromConfig(cfg), now: time.Now}
}

// NewLBWithClient (testes) injeta um cliente ELBv2DescribeLoadBalancers.
func NewLBWithClient(c ELBv2DescribeLoadBalancers) *LB {
	return &LB{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (l *LB) WithClock(now func() time.Time) *LB {
	l.now = now
	return l
}

// DiscoverNetwork lista LBs e devolve NetworkBatch.
func (l *LB) DiscoverNetwork(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (collector.NetworkBatch, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.NetworkBatch{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	zoneByCode := make(map[string]node.URN, len(topo.Zones))
	for _, z := range topo.Zones {
		zoneByCode[z.Code] = z.URN()
	}
	now := l.now().UTC()
	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)

	batch := collector.NetworkBatch{}
	var marker *string
	for {
		out, err := l.cli.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{Marker: marker})
		if err != nil {
			return collector.NetworkBatch{}, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, raw := range out.LoadBalancers {
			id := lbExternalID(raw)
			if id == "" {
				continue
			}
			urn := node.NewURN(node.ProviderAWS, scope.Account, node.KindNetwork, id)

			tags := map[string]string{}
			if string(raw.Type) != "" {
				tags["lb:type"] = string(raw.Type)
			}
			if string(raw.Scheme) != "" {
				tags["lb:scheme"] = string(raw.Scheme)
			}

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
				Flavor:     node.NetworkLB,
				Tags:       tags,
			}
			batch.Networks = append(batch.Networks, n)

			// Region → LB (sempre)
			batch.Edges = append(batch.Edges, containsEdge(regionURN, urn, now))

			// VPC → LB (se VpcId disponível)
			if raw.VpcId != nil && *raw.VpcId != "" {
				vpcURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindNetwork, *raw.VpcId)
				batch.Edges = append(batch.Edges, containsEdge(vpcURN, urn, now))
			}

			// Zone → LB (uma por AZ conhecida)
			for _, az := range raw.AvailabilityZones {
				if az.ZoneName == nil {
					continue
				}
				if zoneURN, ok := zoneByCode[*az.ZoneName]; ok {
					batch.Edges = append(batch.Edges, containsEdge(zoneURN, urn, now))
				}
			}
		}
		if out.NextMarker == nil || *out.NextMarker == "" {
			break
		}
		marker = out.NextMarker
	}
	return batch, nil
}

// lbExternalID prefere LoadBalancerArn (estável, único globalmente),
// caindo para LoadBalancerName quando ausente. Vazio = pular.
func lbExternalID(raw elbtypes.LoadBalancer) string {
	if raw.LoadBalancerArn != nil && *raw.LoadBalancerArn != "" {
		return *raw.LoadBalancerArn
	}
	if raw.LoadBalancerName != nil {
		return *raw.LoadBalancerName
	}
	return ""
}

// containsEdge constrói uma edge Contains genérica entre duas URNs.
func containsEdge(from, to node.URN, now time.Time) edge.Contains {
	return edge.Contains{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeContains, to, now),
			EdgeType: edge.TypeContains,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
				Confidence:  1,
				Directional: true,
			},
		},
	}
}

// Compile-time check.
var _ collector.NetworkDiscoverer = (*LB)(nil)
