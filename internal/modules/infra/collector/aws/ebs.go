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

// EC2DescribeVolumes é a sub-API EC2 necessária para descobrir volumes
// EBS. Declarada como interface para injeção de fake em testes.
type EC2DescribeVolumes interface {
	DescribeVolumes(ctx context.Context, in *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
}

// EBS implementa collector.PersistenceDiscoverer para volumes EBS.
//
// Lista volumes via DescribeVolumes (paginado) e converte para
// node.Persistence (flavor=block). Para cada volume:
//   - Edge Zone→Persistence (Contains) usando AZ do volume.
//   - Para cada Attachment com InstanceId: edge Persistence→Compute
//     (AttachedTo). Volumes detached (sem Attachments) viram nó sem edge.
type EBS struct {
	cli EC2DescribeVolumes
	now func() time.Time
}

// NewEBS constrói o discoverer a partir de awssdk.Config.
func NewEBS(cfg awssdk.Config) *EBS {
	return &EBS{cli: ec2.NewFromConfig(cfg), now: time.Now}
}

// NewEBSWithClient (testes) injeta um cliente EC2DescribeVolumes.
func NewEBSWithClient(c EC2DescribeVolumes) *EBS {
	return &EBS{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (e *EBS) WithClock(now func() time.Time) *EBS {
	e.now = now
	return e
}

// DiscoverPersistence lista volumes EBS e devolve PersistenceBatch.
func (e *EBS) DiscoverPersistence(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (collector.PersistenceBatch, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.PersistenceBatch{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	zoneByCode := make(map[string]node.URN, len(topo.Zones))
	for _, z := range topo.Zones {
		zoneByCode[z.Code] = z.URN()
	}

	batch := collector.PersistenceBatch{}
	var nextToken *string
	for {
		out, err := e.cli.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{NextToken: nextToken})
		if err != nil {
			return collector.PersistenceBatch{}, fmt.Errorf("describe volumes: %w", err)
		}
		for _, v := range out.Volumes {
			p, edges := e.toPersistence(scope, v, zoneByCode)
			if p.URN() == "" {
				continue
			}
			batch.Persistences = append(batch.Persistences, p)
			batch.Edges = append(batch.Edges, edges...)
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}
	return batch, nil
}

// toPersistence converte um Volume EBS em node.Persistence + edges:
// Zone→Persistence (Contains) + 0..N Persistence→Compute (AttachedTo).
// Se VolumeId vazio, retorna Persistence zero (URN=="").
func (e *EBS) toPersistence(scope collector.Scope, v ec2types.Volume, zoneByCode map[string]node.URN) (node.Persistence, []edge.Edge) {
	if v.VolumeId == nil || *v.VolumeId == "" {
		return node.Persistence{}, nil
	}
	now := e.now().UTC()

	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)
	persURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindPersistence, *v.VolumeId)

	tags := make(map[string]string, len(v.Tags))
	for _, t := range v.Tags {
		if t.Key == nil {
			continue
		}
		val := ""
		if t.Value != nil {
			val = *t.Value
		}
		tags[*t.Key] = val
	}

	var size uint64
	if v.Size != nil && *v.Size >= 0 {
		size = uint64(*v.Size)
	}
	var iops uint32
	if v.Iops != nil && *v.Iops >= 0 {
		iops = uint32(*v.Iops)
	}
	encrypted := false
	if v.Encrypted != nil {
		encrypted = *v.Encrypted
	}

	p := node.Persistence{
		Base: node.Base{
			NodeURN:  persURN,
			NodeKind: node.KindPersistence,
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
		ExtID:      *v.VolumeId,
		Flavor:     node.PersistenceBlock,
		Engine:     string(v.VolumeType), // "gp3", "io2", etc.
		SizeGiB:    size,
		IOPS:       iops,
		Encrypted:  encrypted,
		Tags:       tags,
	}

	var edges []edge.Edge

	// Edge Zone → Persistence (Contains).
	var az string
	if v.AvailabilityZone != nil {
		az = *v.AvailabilityZone
	}
	if zoneURN, ok := zoneByCode[az]; ok {
		edges = append(edges, edge.Contains{
			Base: edge.Base{
				EdgeID:   edge.DeterministicID(zoneURN, edge.TypeContains, persURN, now),
				EdgeType: edge.TypeContains,
				FromURN:  zoneURN,
				ToURN:    persURN,
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

	// Edges Persistence → Compute (AttachedTo) por attachment.
	for _, att := range v.Attachments {
		if att.InstanceId == nil || *att.InstanceId == "" {
			continue
		}
		computeURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindCompute, *att.InstanceId)
		mount := ""
		if att.Device != nil {
			mount = *att.Device
		}
		edges = append(edges, edge.AttachedTo{
			Base: edge.Base{
				EdgeID:   edge.DeterministicID(persURN, edge.TypeAttachedTo, computeURN, now),
				EdgeType: edge.TypeAttachedTo,
				FromURN:  persURN,
				ToURN:    computeURN,
				EdgeMeta: edge.Meta{
					ValidFrom:   now,
					ObservedAt:  now,
					Source:      node.Source{Collector: "aws", Method: node.MethodAPI},
					Confidence:  1,
					Directional: true,
				},
			},
			MountPoint: mount,
		})
	}

	return p, edges
}

// Compile-time check.
var _ collector.PersistenceDiscoverer = (*EBS)(nil)
