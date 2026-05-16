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

// EC2DescribeInstances é a sub-API EC2 necessária para descobrir
// instâncias. Declarada como interface para injeção de fake em testes.
type EC2DescribeInstances interface {
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

// EC2 implementa collector.ComputeDiscoverer.
//
// Lista instâncias EC2 via DescribeInstances (paginado) e converte para
// node.Compute. As edges Zone→Compute são pré-computadas usando a AZ
// vinda de Placement.AvailabilityZone, que precisa coincidir com uma
// Zone descoberta em topo (ScopeTopology); instâncias em AZ desconhecida
// são puladas (registrar isso é trabalho do service quando consumir).
type EC2 struct {
	cli EC2DescribeInstances
	now func() time.Time
}

// NewEC2 constrói o discoverer a partir de awssdk.Config (região vem
// da Config, igual a Topology).
func NewEC2(cfg awssdk.Config) *EC2 {
	return &EC2{cli: ec2.NewFromConfig(cfg), now: time.Now}
}

// NewEC2WithClient (testes) injeta um cliente EC2DescribeInstances.
func NewEC2WithClient(c EC2DescribeInstances) *EC2 {
	return &EC2{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (e *EC2) WithClock(now func() time.Time) *EC2 {
	e.now = now
	return e
}

// DiscoverCompute lista as instâncias EC2 do scope e devolve um
// ComputeBatch com nós Compute + edges Zone→Compute (Contains).
func (e *EC2) DiscoverCompute(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (collector.ComputeBatch, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.ComputeBatch{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	// Index AZ name → Zone URN, para edges Zone→Compute.
	zoneByCode := make(map[string]node.URN, len(topo.Zones))
	for _, z := range topo.Zones {
		zoneByCode[z.Code] = z.URN()
	}

	batch := collector.ComputeBatch{}
	var nextToken *string
	for {
		out, err := e.cli.DescribeInstances(ctx, &ec2.DescribeInstancesInput{NextToken: nextToken})
		if err != nil {
			return collector.ComputeBatch{}, fmt.Errorf("describe instances: %w", err)
		}
		for _, res := range out.Reservations {
			for _, inst := range res.Instances {
				c, edgeOK := e.toCompute(scope, inst, zoneByCode)
				if c.URN() == "" {
					continue // sem instance-id válido
				}
				batch.Computes = append(batch.Computes, c)
				if edgeOK.ID() != "" {
					batch.Edges = append(batch.Edges, edgeOK)
				}
			}
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}
	return batch, nil
}

// toCompute converte uma instância EC2 em node.Compute + edge Zone→Compute.
// Se a instância não tiver InstanceId, retorna Compute vazio (URN=="").
// A edge é vazia (ID=="") se a AZ não estiver no topo (defensivo).
func (e *EC2) toCompute(scope collector.Scope, inst ec2types.Instance, zoneByCode map[string]node.URN) (node.Compute, edge.Contains) {
	if inst.InstanceId == nil || *inst.InstanceId == "" {
		return node.Compute{}, edge.Contains{}
	}
	now := e.now().UTC()

	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)
	computeURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindCompute, *inst.InstanceId)

	tags := make(map[string]string, len(inst.Tags))
	for _, t := range inst.Tags {
		if t.Key == nil {
			continue
		}
		v := ""
		if t.Value != nil {
			v = *t.Value
		}
		tags[*t.Key] = v
	}

	instType := ""
	if inst.InstanceType != "" {
		instType = string(inst.InstanceType)
	}

	state := node.LifecycleState("")
	if inst.State != nil {
		state = mapEC2State(inst.State.Name)
	}

	c := node.Compute{
		Base: node.Base{
			NodeURN:  computeURN,
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  now,
				ObservedAt: now,
				Confidence: 1,
				Source:     node.Source{Collector: "aws", Method: node.MethodAPI},
			},
		},
		ProviderID:   node.ProviderAWS,
		Account:      accountURN,
		Region:       regionURN,
		ExtID:        *inst.InstanceId,
		Flavor:       node.ComputeVM,
		InstanceType: instType,
		State:        state,
		Tags:         tags,
	}

	var az string
	if inst.Placement != nil && inst.Placement.AvailabilityZone != nil {
		az = *inst.Placement.AvailabilityZone
	}
	zoneURN, ok := zoneByCode[az]
	if !ok {
		return c, edge.Contains{}
	}

	ed := edge.Contains{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(zoneURN, edge.TypeContains, computeURN, now),
			EdgeType: edge.TypeContains,
			FromURN:  zoneURN,
			ToURN:    computeURN,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
				Confidence:  1,
				Directional: true,
			},
		},
	}
	return c, ed
}

// mapEC2State traduz ec2types.InstanceStateName para node.LifecycleState.
// "shutting-down" e "stopping" são tratados como Stopped (terminal/transient
// que para faturamento não muda nada relevante neste passo).
func mapEC2State(s ec2types.InstanceStateName) node.LifecycleState {
	switch s {
	case ec2types.InstanceStateNamePending:
		return node.StatePending
	case ec2types.InstanceStateNameRunning:
		return node.StateRunning
	case ec2types.InstanceStateNameShuttingDown, ec2types.InstanceStateNameStopping, ec2types.InstanceStateNameStopped:
		return node.StateStopped
	case ec2types.InstanceStateNameTerminated:
		return node.StateTerminated
	default:
		return node.LifecycleState("")
	}
}

// Compile-time check.
var _ collector.ComputeDiscoverer = (*EC2)(nil)
