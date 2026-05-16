package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// fakeS3 simula ListBuckets + GetBucketLocation.
type fakeS3 struct {
	buckets       []s3types.Bucket
	locByBucket   map[string]string
	listErr       error
	locErr        error
	listCalls     int
	locCalls      int
}

func (f *fakeS3) ListBuckets(ctx context.Context, in *s3.ListBucketsInput, opts ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &s3.ListBucketsOutput{Buckets: f.buckets}, nil
}

func (f *fakeS3) GetBucketLocation(ctx context.Context, in *s3.GetBucketLocationInput, opts ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	f.locCalls++
	if f.locErr != nil {
		return nil, f.locErr
	}
	loc := ""
	if in.Bucket != nil {
		loc = f.locByBucket[*in.Bucket]
	}
	return &s3.GetBucketLocationOutput{LocationConstraint: s3types.BucketLocationConstraint(loc)}, nil
}

func mkBucket(name string, region *string) s3types.Bucket {
	n := name
	return s3types.Bucket{Name: &n, BucketRegion: region}
}

func TestS3_DiscoverPersistence_HappyPath_UsesBucketRegion(t *testing.T) {
	clock := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	use := "us-east-1"
	eu := "eu-west-1"
	fake := &fakeS3{buckets: []s3types.Bucket{
		mkBucket("logs-prod", &use),
		mkBucket("data-eu", &eu),
	}}
	s := NewS3WithClient(fake).WithClock(fixedClock(clock))
	got, err := s.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, collector.ScopeTopology{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got.Persistences) != 2 || len(got.Edges) != 2 {
		t.Fatalf("expected 2 persistences and 2 edges, got %d/%d", len(got.Persistences), len(got.Edges))
	}
	if fake.locCalls != 0 {
		t.Fatalf("BucketRegion presente — GetBucketLocation não devia ser chamado; calls=%d", fake.locCalls)
	}

	p0 := got.Persistences[0]
	if p0.URN() != "urn:ce:aws:123456789012:persistence/logs-prod" {
		t.Fatalf("p0 URN = %q", p0.URN())
	}
	if p0.Flavor != node.PersistenceObject || p0.Engine != "s3" {
		t.Fatalf("p0 flavor/engine wrong: %+v", p0)
	}
	if p0.Region != "urn:ce:aws:123456789012:region/us-east-1" {
		t.Fatalf("p0 region = %q", p0.Region)
	}

	// Edges: Account → Persistence
	accountURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindAccount, "123456789012")
	for _, e := range got.Edges {
		if e.From() != accountURN {
			t.Fatalf("edge from %q, want Account URN", e.From())
		}
		if e.Type() != edge.TypeContains {
			t.Fatalf("edge type = %q", e.Type())
		}
	}
}

func TestS3_DiscoverPersistence_FallbackToGetBucketLocation(t *testing.T) {
	clock := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	fake := &fakeS3{
		buckets:     []s3types.Bucket{mkBucket("legacy-bucket", nil)}, // sem BucketRegion
		locByBucket: map[string]string{"legacy-bucket": "ap-southeast-1"},
	}
	s := NewS3WithClient(fake).WithClock(fixedClock(clock))
	got, err := s.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if fake.locCalls != 1 {
		t.Fatalf("expected 1 GetBucketLocation call, got %d", fake.locCalls)
	}
	if got.Persistences[0].Region != "urn:ce:aws:1:region/ap-southeast-1" {
		t.Fatalf("region resolved wrong: %q", got.Persistences[0].Region)
	}
}

func TestS3_DiscoverPersistence_NormalizesLegacyLocations(t *testing.T) {
	clock := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		loc      string
		wantCode string
	}{
		{"empty → us-east-1", "", "us-east-1"},
		{"EU → eu-west-1", "EU", "eu-west-1"},
		{"unchanged", "ap-northeast-1", "ap-northeast-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeS3{
				buckets:     []s3types.Bucket{mkBucket("b", nil)},
				locByBucket: map[string]string{"b": tc.loc},
			}
			s := NewS3WithClient(fake).WithClock(fixedClock(clock))
			got, _ := s.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
			wantURN := node.NewURN(node.ProviderAWS, "1", node.KindRegion, tc.wantCode)
			if got.Persistences[0].Region != wantURN {
				t.Fatalf("got region URN %q, want %q", got.Persistences[0].Region, wantURN)
			}
		})
	}
}

func TestS3_DiscoverPersistence_SkipsBlankBucketName(t *testing.T) {
	clock := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	blank := ""
	use := "us-east-1"
	fake := &fakeS3{buckets: []s3types.Bucket{
		{Name: &blank, BucketRegion: &use},
		mkBucket("good", &use),
	}}
	s := NewS3WithClient(fake).WithClock(fixedClock(clock))
	got, _ := s.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if len(got.Persistences) != 1 {
		t.Fatalf("expected 1 persistence (blank skipped), got %d", len(got.Persistences))
	}
}

func TestS3_DiscoverPersistence_DeterministicEdgeIDs(t *testing.T) {
	clock := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	use := "us-east-1"
	mk := func() *S3 {
		return NewS3WithClient(&fakeS3{buckets: []s3types.Bucket{mkBucket("b", &use)}}).WithClock(fixedClock(clock))
	}
	a, _ := mk().DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	b, _ := mk().DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if a.Edges[0].ID() != b.Edges[0].ID() {
		t.Fatalf("expected deterministic edge ID, got %q vs %q", a.Edges[0].ID(), b.Edges[0].ID())
	}
}

func TestS3_DiscoverPersistence_ListError(t *testing.T) {
	boom := errors.New("AccessDenied")
	s := NewS3WithClient(&fakeS3{listErr: boom})
	_, err := s.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestS3_DiscoverPersistence_LocationError(t *testing.T) {
	boom := errors.New("AccessDenied")
	s := NewS3WithClient(&fakeS3{
		buckets: []s3types.Bucket{mkBucket("b", nil)},
		locErr:  boom,
	})
	_, err := s.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, collector.ScopeTopology{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

func TestS3_DiscoverPersistence_EmptyAccount(t *testing.T) {
	s := NewS3WithClient(&fakeS3{})
	if _, err := s.DiscoverPersistence(context.Background(), collector.Scope{}, collector.ScopeTopology{}); err == nil {
		t.Fatal("expected error for empty account")
	}
}
