package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// RDSDescribeDBInstances é a sub-API RDS necessária para descobrir
// instâncias. Declarada como interface para injeção de fake em testes.
type RDSDescribeDBInstances interface {
	DescribeDBInstances(ctx context.Context, in *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// RDS implementa collector.PersistenceDiscoverer para instâncias RDS.
//
// Lista instâncias via DescribeDBInstances (paginado com Marker) e
// converte cada DBInstance em node.Persistence (flavor=rdbms). Para
// cada instância, gera edge Zone→Persistence (Contains) via AZ.
// Instâncias MultiAZ continuam ancoradas na AZ primária reportada
// pela API (AvailabilityZone) — o aspecto MultiAZ pode ser modelado
// como atributo posteriormente, sem alterar a topologia atual.
type RDS struct {
	cli RDSDescribeDBInstances
	now func() time.Time
}

// NewRDS constrói o discoverer a partir de awssdk.Config.
func NewRDS(cfg awssdk.Config) *RDS {
	return &RDS{cli: rds.NewFromConfig(cfg), now: time.Now}
}

// NewRDSWithClient (testes) injeta um cliente RDSDescribeDBInstances.
func NewRDSWithClient(c RDSDescribeDBInstances) *RDS {
	return &RDS{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (r *RDS) WithClock(now func() time.Time) *RDS {
	r.now = now
	return r
}

// DiscoverPersistence lista instâncias RDS e devolve PersistenceBatch.
func (r *RDS) DiscoverPersistence(ctx context.Context, scope collector.Scope, topo collector.ScopeTopology) (collector.PersistenceBatch, error) {
	if scope.Account == "" || scope.Region == "" {
		return collector.PersistenceBatch{}, fmt.Errorf("scope requires Account and Region (got %q/%q)", scope.Account, scope.Region)
	}

	zoneByCode := make(map[string]node.URN, len(topo.Zones))
	for _, z := range topo.Zones {
		zoneByCode[z.Code] = z.URN()
	}

	batch := collector.PersistenceBatch{}
	var marker *string
	for {
		out, err := r.cli.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{Marker: marker})
		if err != nil {
			return collector.PersistenceBatch{}, fmt.Errorf("describe db instances: %w", err)
		}
		for _, d := range out.DBInstances {
			p, edges := r.toPersistence(scope, d, zoneByCode)
			if p.URN() == "" {
				continue
			}
			batch.Persistences = append(batch.Persistences, p)
			batch.Edges = append(batch.Edges, edges...)
		}
		if out.Marker == nil || *out.Marker == "" {
			break
		}
		marker = out.Marker
	}
	return batch, nil
}

// toPersistence converte uma DBInstance em node.Persistence + 0..1 edge
// Zone→Persistence (Contains). Se DBInstanceIdentifier vazio, retorna
// Persistence zero (URN==""), sinal para o caller pular.
func (r *RDS) toPersistence(scope collector.Scope, d rdstypes.DBInstance, zoneByCode map[string]node.URN) (node.Persistence, []edge.Edge) {
	if d.DBInstanceIdentifier == nil || *d.DBInstanceIdentifier == "" {
		return node.Persistence{}, nil
	}
	now := r.now().UTC()
	id := *d.DBInstanceIdentifier

	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)
	regionURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, scope.Region)
	persURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindPersistence, id)

	tags := make(map[string]string, len(d.TagList))
	for _, t := range d.TagList {
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
	if d.AllocatedStorage != nil && *d.AllocatedStorage >= 0 {
		size = uint64(*d.AllocatedStorage)
	}
	var iops uint32
	if d.Iops != nil && *d.Iops >= 0 {
		iops = uint32(*d.Iops)
	}
	encrypted := false
	if d.StorageEncrypted != nil {
		encrypted = *d.StorageEncrypted
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
		ExtID:      id,
		Flavor:     node.PersistenceRelDB,
		Engine:     rdsEngineLabel(d.Engine, d.EngineVersion),
		SizeGiB:    size,
		IOPS:       iops,
		Encrypted:  encrypted,
		Tags:       tags,
	}

	var edges []edge.Edge

	var az string
	if d.AvailabilityZone != nil {
		az = *d.AvailabilityZone
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

	return p, edges
}

// rdsEngineLabel monta o rótulo canônico "engine-version" (ex.:
// "postgres-15.3"). Se a versão for vazia, devolve só o engine. Se
// engine for vazio, devolve string vazia (caller já tolera).
func rdsEngineLabel(engine, version *string) string {
	e := ""
	if engine != nil {
		e = strings.TrimSpace(*engine)
	}
	v := ""
	if version != nil {
		v = strings.TrimSpace(*version)
	}
	switch {
	case e == "" && v == "":
		return ""
	case v == "":
		return e
	case e == "":
		return v
	default:
		return e + "-" + v
	}
}

// Compile-time check.
var _ collector.PersistenceDiscoverer = (*RDS)(nil)
