package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// S3ListBuckets é a sub-API S3 necessária para descobrir buckets.
// Declarada como interface para injeção de fake em testes.
type S3ListBuckets interface {
	ListBuckets(ctx context.Context, in *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketLocation(ctx context.Context, in *s3.GetBucketLocationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
}

// S3 implementa collector.PersistenceDiscoverer para buckets S3.
//
// S3 é um serviço global: ListBuckets devolve TODOS os buckets da conta,
// independentemente de scope.Region. Para cada bucket faz GetBucketLocation
// (uma chamada extra; o SDK v2 mais novo expõe BucketRegion no próprio
// output do ListBuckets, mas mantemos o GetBucketLocation para
// compatibilidade com regiões legadas — "" = us-east-1, "EU" = eu-west-1).
//
// Cada bucket vira Persistence(flavor=object) com edge Contains direto
// de Account → Persistence (sem Zone, sem Region intermediária no edge
// — coerente com o fato de que buckets não são localizados em AZ).
type S3 struct {
	cli S3ListBuckets
	now func() time.Time
}

// NewS3 constrói o discoverer a partir de awssdk.Config.
func NewS3(cfg awssdk.Config) *S3 {
	return &S3{cli: s3.NewFromConfig(cfg), now: time.Now}
}

// NewS3WithClient (testes) injeta um cliente S3ListBuckets.
func NewS3WithClient(c S3ListBuckets) *S3 {
	return &S3{cli: c, now: time.Now}
}

// WithClock substitui o relógio (testes determinísticos).
func (s *S3) WithClock(now func() time.Time) *S3 {
	s.now = now
	return s
}

// DiscoverPersistence lista buckets S3 da conta e devolve PersistenceBatch.
func (s *S3) DiscoverPersistence(ctx context.Context, scope collector.Scope, _ collector.ScopeTopology) (collector.PersistenceBatch, error) {
	if scope.Account == "" {
		return collector.PersistenceBatch{}, fmt.Errorf("scope requires Account (got %q)", scope.Account)
	}

	out, err := s.cli.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return collector.PersistenceBatch{}, fmt.Errorf("list buckets: %w", err)
	}

	now := s.now().UTC()
	accountURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindAccount, scope.Account)

	batch := collector.PersistenceBatch{}
	for _, b := range out.Buckets {
		if b.Name == nil || *b.Name == "" {
			continue
		}
		name := *b.Name

		region, err := s.resolveBucketRegion(ctx, b)
		if err != nil {
			return collector.PersistenceBatch{}, fmt.Errorf("get location %s: %w", name, err)
		}

		var regionURN node.URN
		if region != "" {
			regionURN = node.NewURN(node.ProviderAWS, scope.Account, node.KindRegion, region)
		}
		persURN := node.NewURN(node.ProviderAWS, scope.Account, node.KindPersistence, name)

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
			ExtID:      name,
			Flavor:     node.PersistenceObject,
			Engine:     "s3",
		}
		batch.Persistences = append(batch.Persistences, p)

		// Edge Account → Persistence (Contains). S3 não tem Zone; o
		// edge vai direto do Account.
		batch.Edges = append(batch.Edges, edge.Contains{
			Base: edge.Base{
				EdgeID:   edge.DeterministicID(accountURN, edge.TypeContains, persURN, now),
				EdgeType: edge.TypeContains,
				FromURN:  accountURN,
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
	return batch, nil
}

// resolveBucketRegion devolve o código de região do bucket. Prefere o
// campo BucketRegion (presente no SDK v2 moderno) e cai para
// GetBucketLocation caso ausente. Normaliza códigos legados.
func (s *S3) resolveBucketRegion(ctx context.Context, b s3types.Bucket) (string, error) {
	if b.BucketRegion != nil && *b.BucketRegion != "" {
		return normalizeLocation(*b.BucketRegion), nil
	}
	if b.Name == nil {
		return "", nil
	}
	loc, err := s.cli.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: b.Name})
	if err != nil {
		return "", err
	}
	return normalizeLocation(string(loc.LocationConstraint)), nil
}

// normalizeLocation traduz códigos legados da API S3 GetBucketLocation:
//   - "" → "us-east-1" (default legado)
//   - "EU" → "eu-west-1" (legado)
func normalizeLocation(s string) string {
	switch s {
	case "":
		return "us-east-1"
	case "EU":
		return "eu-west-1"
	default:
		return s
	}
}

// Compile-time check.
var _ collector.PersistenceDiscoverer = (*S3)(nil)
