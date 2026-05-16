package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// EC2SecurityGroups é a sub-API EC2 necessária para descobrir Security
// Groups. Declarada como interface para injeção de fake em testes.
type EC2SecurityGroups interface {
	DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

// SG implementa collector.NetworkDiscoverer para Security Groups EC2.
//
// Lista SGs via DescribeSecurityGroups (paginado por NextToken) e
// converte para node.Network(flavor=securitygroup). Cada SG vive dentro
// de uma VPC; emitimos edge VPC→SG (Contains). Como S-007b roda
// independente de S-007a, o discoverer não conhece quais VPCs foram
// descobertas — emite a edge VPC→SG usando a URN canônica do VpcId
// reportado; a service layer já tolera edges sem nó correspondente no
// batch corrente (idempotência por DeterministicID + upsert).
type SG struct {
	cli EC2SecurityGroups
	now func() time.Time
}

// NewSG constrói o discoverer a partir de awssdk.Config.
func NewSG(cfg awssdk.Config) *SG {
	return &SG{cli: ec2.NewFromConfig(cfg), now: time.Now}
}

// NewSGWithClient (testes) injeta um cliente EC2SecurityGroups.
func NewSGWithClient(c EC2SecurityGroups) *SG {
	return &SG{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (s *SG) WithClock(now func() time.Time) *SG {
	s.now = now
	return s
}

// DiscoverNetwork lista Security Groups e devolve NetworkBatch.
func (s *SG) DiscoverNetwork(ctx context.Context, scope collector.Scope, _ collector.ScopeTopology) (collector.NetworkBatch, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.NetworkBatch{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	now := s.now().UTC()
	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)

	batch := collector.NetworkBatch{}
	var token *string
	for {
		out, err := s.cli.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{NextToken: token})
		if err != nil {
			return collector.NetworkBatch{}, fmt.Errorf("describe security groups: %w", err)
		}
		for _, raw := range out.SecurityGroups {
			if raw.GroupId == nil || *raw.GroupId == "" {
				continue
			}
			id := *raw.GroupId
			urn := node.NewURN(node.ProviderAWS, scope.Account, node.KindNetwork, id)

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
				Flavor:     node.NetworkSecurityGroup,
				Tags:       ec2TagsToMap(raw.Tags),
			}
			batch.Networks = append(batch.Networks, n)

			// VPC → SG (Contains). Emite só se VpcId disponível
			// (default SG em conta sem VPC vem sem VpcId).
			if raw.VpcId != nil && *raw.VpcId != "" {
				vpcURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindNetwork, *raw.VpcId)
				batch.Edges = append(batch.Edges, containsEdge(vpcURN, urn, now))
			}
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		token = out.NextToken
	}

	return batch, nil
}

// Compile-time check.
var _ collector.NetworkDiscoverer = (*SG)(nil)
