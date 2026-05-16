package bridge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func mkCompute(urn, account, ext string, vfrom time.Time) node.Compute {
	return node.Compute{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  vfrom,
				ObservedAt: vfrom,
				Confidence: 1,
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    node.URN("urn:ce:aws:" + account + ":account/" + account),
		Region:     node.URN("urn:ce:aws:" + account + ":region/us-east-1"),
		ExtID:      ext,
		Flavor:     node.ComputeVM,
	}
}

func mkPersistence(urn, account, ext string, vfrom time.Time) node.Persistence {
	return node.Persistence{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindPersistence,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  vfrom,
				ObservedAt: vfrom,
				Confidence: 1,
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    node.URN("urn:ce:aws:" + account + ":account/" + account),
		Region:     node.URN("urn:ce:aws:" + account + ":region/us-east-1"),
		ExtID:      ext,
		Flavor:     node.PersistenceObject,
	}
}

func newBridge(t *testing.T) (bridge.Resolver, *memory.Repo) {
	t.Helper()
	r := memory.New()
	return bridge.New(r), r
}

// ----------------------------------------------------------------------------
// AC-1: short ID → URN (versão corrente).
// ----------------------------------------------------------------------------

func TestResolveURN_StrictMatch(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()
	c := mkCompute("urn:ce:aws:111:compute/i-1", "111", "i-0abc1234", time.Now())
	if err := repo.Upsert(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	urn, err := br.ResolveURN(ctx, node.ProviderAWS, "111", "i-0abc1234", repository.AsOf{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if urn != c.URN() {
		t.Fatalf("got %q want %q", urn, c.URN())
	}
}

// ----------------------------------------------------------------------------
// S-002: ARN ↔ short ID — qualquer forma resolve.
// ----------------------------------------------------------------------------

func TestResolveURN_ARNToShortID(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()

	// Recurso indexado por ARN.
	arn := "arn:aws:ec2:us-east-1:111:instance/i-0abc1234"
	c := mkCompute("urn:ce:aws:111:compute/i-0abc1234", "111", arn, time.Now())
	if err := repo.Upsert(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Lookup pelo ARN.
	urn, err := br.ResolveURN(ctx, node.ProviderAWS, "111", arn, repository.AsOf{})
	if err != nil || urn != c.URN() {
		t.Fatalf("ARN lookup: urn=%q err=%v", urn, err)
	}

	// Lookup pelo short ID derivado.
	urn, err = br.ResolveURN(ctx, node.ProviderAWS, "111", "i-0abc1234", repository.AsOf{})
	if err != nil || urn != c.URN() {
		t.Fatalf("short lookup: urn=%q err=%v", urn, err)
	}
}

func TestResolveURN_S3BucketARN(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()

	arn := "arn:aws:s3:::my-bucket"
	p := mkPersistence("urn:ce:aws:111:persistence/my-bucket", "111", arn, time.Now())
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	urn, err := br.ResolveURN(ctx, node.ProviderAWS, "111", "my-bucket", repository.AsOf{})
	if err != nil || urn != p.URN() {
		t.Fatalf("short lookup S3: urn=%q err=%v", urn, err)
	}
}

// ----------------------------------------------------------------------------
// S-003: bitemporal — AsOf vs corrente.
// ----------------------------------------------------------------------------

func TestResolveURN_AsOfPastVersion(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()

	urn := "urn:ce:aws:111:compute/i-1"
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	v1 := mkCompute(urn, "111", "i-1", t0)
	v2 := mkCompute(urn, "111", "i-1", t1)
	v2.NodeMeta.Version = 2

	clk := t0
	repo.WithClock(func() time.Time {
		clk = clk.Add(24 * time.Hour)
		return clk
	})

	if err := repo.Upsert(ctx, v1); err != nil {
		t.Fatalf("upsert v1: %v", err)
	}
	if err := repo.Upsert(ctx, v2); err != nil {
		t.Fatalf("upsert v2: %v", err)
	}

	// Corrente → v2 (mesma URN).
	got, err := br.ResolveURN(ctx, node.ProviderAWS, "111", "i-1", repository.AsOf{})
	if err != nil || got != v1.URN() {
		t.Fatalf("current: got=%q err=%v", got, err)
	}

	// AsOf entre t0 e a hora da segunda upsert → v1 visível.
	got, err = br.ResolveURN(ctx, node.ProviderAWS, "111", "i-1", repository.AsOf(t0.Add(time.Hour)))
	if err != nil || got != v1.URN() {
		t.Fatalf("as-of v1: got=%q err=%v", got, err)
	}
}

func TestResolveURN_DeletedReturnsNotFound(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()
	c := mkCompute("urn:ce:aws:111:compute/i-x", "111", "i-x", time.Now())
	if err := repo.Upsert(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.Delete(ctx, c.URN()); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := br.ResolveURN(ctx, node.ProviderAWS, "111", "i-x", repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestResolveURN_UnknownReturnsNotFound(t *testing.T) {
	br, _ := newBridge(t)
	ctx := context.Background()

	_, err := br.ResolveURN(ctx, node.ProviderAWS, "111", "i-doesnotexist", repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// S-005: cross-account / ambiguidade.
// ----------------------------------------------------------------------------

// CUR atribui bucket à conta A; grafo guarda em conta B. Bridge resolve
// via fallback wildcard.
func TestResolveURN_CrossAccountFallback(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()

	// Bucket vive em conta B no grafo.
	arn := "arn:aws:s3:::shared-bucket"
	p := mkPersistence("urn:ce:aws:B:persistence/shared-bucket", "B", arn, time.Now())
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// CUR vem com conta A. Bridge deve resolver (fallback wildcard).
	urn, err := br.ResolveURN(ctx, node.ProviderAWS, "A", arn, repository.AsOf{})
	if err != nil {
		t.Fatalf("cross-account: %v", err)
	}
	if urn != p.URN() {
		t.Fatalf("got %q want %q", urn, p.URN())
	}
}

// Dois recursos com mesmo external_id em contas diferentes → ErrAmbiguous
// quando wildcard. CUR não consegue desambiguar.
func TestResolveURN_AmbiguousAcrossAccounts(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()

	// Mesmo short ID em duas contas distintas.
	c1 := mkCompute("urn:ce:aws:A:compute/i-dup", "A", "i-dup", time.Now())
	c2 := mkCompute("urn:ce:aws:B:compute/i-dup", "B", "i-dup", time.Now())
	if err := repo.Upsert(ctx, c1); err != nil {
		t.Fatalf("upsert c1: %v", err)
	}
	if err := repo.Upsert(ctx, c2); err != nil {
		t.Fatalf("upsert c2: %v", err)
	}

	// Estrito por A → resolve A (não-ambíguo dentro da conta).
	urn, err := br.ResolveURN(ctx, node.ProviderAWS, "A", "i-dup", repository.AsOf{})
	if err != nil || urn != c1.URN() {
		t.Fatalf("strict A: urn=%q err=%v", urn, err)
	}

	// Wildcard sem account → ambíguo.
	_, err = br.ResolveURN(ctx, node.ProviderAWS, "", "i-dup", repository.AsOf{})
	if !errors.Is(err, repository.ErrAmbiguous) {
		t.Fatalf("want ErrAmbiguous, got %v", err)
	}

	// Conta C (que não tem o recurso): estrito falha; fallback wildcard
	// também ambíguo (dois donos possíveis) → ErrAmbiguous.
	_, err = br.ResolveURN(ctx, node.ProviderAWS, "C", "i-dup", repository.AsOf{})
	if !errors.Is(err, repository.ErrAmbiguous) {
		t.Fatalf("fallback C: want ErrAmbiguous, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// Batch
// ----------------------------------------------------------------------------

func TestResolveURNBatch_MixedResults(t *testing.T) {
	br, repo := newBridge(t)
	ctx := context.Background()

	c1 := mkCompute("urn:ce:aws:111:compute/i-1", "111", "i-1", time.Now())
	c2 := mkCompute("urn:ce:aws:111:compute/i-2", "111", "i-2", time.Now())
	for _, n := range []node.Node{c1, c2} {
		if err := repo.Upsert(ctx, n); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	got, failures, err := br.ResolveURNBatch(ctx, node.ProviderAWS, "111",
		[]string{"i-1", "i-2", "i-missing"}, repository.AsOf{})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 resolved, got %d", len(got))
	}
	if got["i-1"] != c1.URN() || got["i-2"] != c2.URN() {
		t.Fatalf("mapping wrong: %+v", got)
	}
	if len(failures) != 1 || failures[0].ExternalID != "i-missing" {
		t.Fatalf("want 1 failure on i-missing, got %+v", failures)
	}
	if !errors.Is(failures[0].Err, repository.ErrNotFound) {
		t.Fatalf("failure should wrap ErrNotFound: %v", failures[0].Err)
	}
}
