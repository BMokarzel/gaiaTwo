package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// EC2DescribeAZ é a sub-API EC2 necessária para descobrir AZs.
// Declarada como interface para permitir injeção de fake em testes.
type EC2DescribeAZ interface {
	DescribeAvailabilityZones(ctx context.Context, in *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
}

// Topology implementa collector.ScopeDiscoverer para AWS.
//
// Constrói os nós Account, Region e Zones (via EC2 DescribeAvailabilityZones)
// e as edges Contains correspondentes. Não persiste — quem consome o
// ScopeTopology é a service layer.
type Topology struct {
	ec2 EC2DescribeAZ
	now func() time.Time
}

// NewTopology constrói um Topology a partir de awssdk.Config. A região
// do client EC2 vem da Config; quem chama deve ter carregado a Config
// com a região alvo (ver Collector.New).
func NewTopology(cfg awssdk.Config) *Topology {
	return &Topology{ec2: ec2.NewFromConfig(cfg), now: time.Now}
}

// NewTopologyWithEC2 (testes) injeta um EC2DescribeAZ explícito.
func NewTopologyWithEC2(c EC2DescribeAZ) *Topology {
	return &Topology{ec2: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (t *Topology) WithClock(now func() time.Time) *Topology {
	t.now = now
	return t
}

// DiscoverScope monta a hierarquia Account → Region → Zone para o
// Scope dado. Retorna a topologia pronta para upsert.
func (t *Topology) DiscoverScope(ctx context.Context, scope collector.Scope) (collector.ScopeTopology, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.ScopeTopology{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	out, err := t.ec2.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return collector.ScopeTopology{}, fmt.Errorf("describe AZs: %w", err)
	}

	now := t.now().UTC()
	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)

	// meta retorna um novo Meta (cada nó/aresta tem cópia independente).
	meta := func() node.Meta {
		return node.Meta{
			Version:    1,
			ValidFrom:  now,
			ObservedAt: now,
			Confidence: 1,
			Source: node.Source{
				Collector: "aws",
				Method:    node.MethodAPI,
			},
		}
	}

	topo := collector.ScopeTopology{
		Account: node.Account{
			Base:        node.Base{NodeURN: accountURN, NodeKind: node.KindAccount, NodeMeta: meta()},
			ProviderID:  node.ProviderAWS,
			ExternalID:  scope.Account,
			DisplayName: scope.Account,
		},
		Region: node.Region{
			Base:       node.Base{NodeURN: regionURN, NodeKind: node.KindRegion, NodeMeta: meta()},
			ProviderID: node.ProviderAWS,
			Code:       scope.Region,
		},
	}

	// Edge Account → Region.
	topo.Edges = append(topo.Edges, edge.Contains{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(accountURN, edge.TypeContains, regionURN, now),
			EdgeType: edge.TypeContains,
			FromURN:  accountURN,
			ToURN:    regionURN,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
				Confidence:  1,
				Directional: true,
			},
		},
	})

	// Zones + edges Region → Zone.
	for _, az := range out.AvailabilityZones {
		if az.ZoneName == nil || *az.ZoneName == "" {
			continue
		}
		zoneCode := *az.ZoneName
		zoneURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindZone, zoneCode)

		topo.Zones = append(topo.Zones, node.Zone{
			Base:      node.Base{NodeURN: zoneURN, NodeKind: node.KindZone, NodeMeta: meta()},
			RegionURN: regionURN,
			Code:      zoneCode,
		})

		topo.Edges = append(topo.Edges, edge.Contains{
			Base: edge.Base{
				EdgeID:   edge.DeterministicID(regionURN, edge.TypeContains, zoneURN, now),
				EdgeType: edge.TypeContains,
				FromURN:  regionURN,
				ToURN:    zoneURN,
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

	return topo, nil
}

// Compile-time check.
var _ collector.ScopeDiscoverer = (*Topology)(nil)
