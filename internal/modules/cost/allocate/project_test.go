package allocate

import (
	"context"
	"math"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

// TestProject_AllDimensionsInvariant: dado 4 buckets resolvidos +
// 1 NotFound + 1 ambíguo + 1 sem resource_id, o total por DIMENSÃO
// bate com o total CUR.
func TestProject_AllDimensionsInvariant(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	allocAt := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)

	agg := NewAggregator()
	_ = agg.Collect(context.Background(), &fakeSource{lines: []RawLine{
		// Resolvidos:
		{AccountID: "111", ResourceID: "i-1", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 1.00, Currency: "USD"},
		{AccountID: "111", ResourceID: "i-2", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 2.00, Currency: "USD"},
		{AccountID: "111", ResourceID: "vol-1", Region: "us-east-1", Service: "AmazonEBS", EffectiveCost: 0.50, Currency: "USD"},
		{AccountID: "222", ResourceID: "i-1", Region: "us-west-2", Service: "AmazonEC2", EffectiveCost: 3.00, Currency: "USD"},
		// Não resolvíveis:
		{AccountID: "111", ResourceID: "missing", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 0.10, Currency: "USD"},
		{AccountID: "111", ResourceID: "shared", Region: "us-east-1", Service: "AmazonS3", EffectiveCost: 0.30, Currency: "USD"},
		// Sem resource_id (tax/support):
		{AccountID: "111", ResourceID: "", Service: "AWSSupport", EffectiveCost: 5.00, Currency: "USD"},
	}}, period)

	res := Resolution{
		URNByResource: map[resourceLookup]node.URN{
			{"111", "i-1"}:   "urn:ce:aws:111:compute/i-1",
			{"111", "i-2"}:   "urn:ce:aws:111:compute/i-2",
			{"111", "vol-1"}: "urn:ce:aws:111:persistence/vol-1",
			{"222", "i-1"}:   "urn:ce:aws:222:compute/i-1",
		},
		Failures: map[resourceLookup]UnallocatedReason{
			{"111", "missing"}: ReasonNotFound,
			{"111", "shared"}:  ReasonAmbiguous,
		},
	}

	rows, unalloc := Project(agg, res, DefaultResolvers(), period, allocAt)

	// Soma por dimensão deve ser igual a TotalAllocated (não TotalCost!).
	allocated := 1.00 + 2.00 + 0.50 + 3.00 // = 6.50
	unallocated := 0.10 + 0.30 + 5.00      // = 5.40

	for _, dim := range []Dimension{DimensionService, DimensionAccount, DimensionRegion} {
		var sum float64
		for _, r := range rows {
			if r.Dimension == dim {
				sum += r.Amount
			}
		}
		if math.Abs(sum-allocated) > 1e-9 {
			t.Errorf("sum dim=%s = %v, want %v", dim, sum, allocated)
		}
	}

	var unSum float64
	for _, u := range unalloc {
		unSum += u.Amount
	}
	if math.Abs(unSum-unallocated) > 1e-9 {
		t.Errorf("unallocated sum=%v want %v", unSum, unallocated)
	}

	// Invariante macro: allocated + unallocated == TotalCost
	if math.Abs(allocated+unallocated-agg.TotalCost) > 1e-9 {
		t.Errorf("allocated+unallocated = %v, TotalCost = %v", allocated+unallocated, agg.TotalCost)
	}

	// Rows devem ter AllocatedAt e BillingPeriod consistentes.
	for _, r := range rows {
		if !r.AllocatedAt.Equal(allocAt) {
			t.Errorf("row allocated_at = %v want %v", r.AllocatedAt, allocAt)
		}
		if !r.BillingPeriod.Equal(period) {
			t.Errorf("row billing_period = %v want %v", r.BillingPeriod, period)
		}
	}
}

// TestProject_NoResourceIDBucketUnalloc: bucket Unalloc do agg vira
// UnallocatedRow com reason=no_resource_id.
func TestProject_NoResourceIDBucketUnalloc(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	agg := NewAggregator()
	_ = agg.Collect(context.Background(), &fakeSource{lines: []RawLine{
		{AccountID: "111", ResourceID: "", Service: "AWSSupport", EffectiveCost: 1.00, Currency: "USD"},
	}}, period)

	_, unalloc := Project(agg, Resolution{}, DefaultResolvers(), period, time.Now().UTC())
	if len(unalloc) != 1 {
		t.Fatalf("len=%d want 1", len(unalloc))
	}
	if unalloc[0].Reason != ReasonNoResourceID {
		t.Errorf("reason=%v want %v", unalloc[0].Reason, ReasonNoResourceID)
	}
}

// TestResolversFor preserva ordem do request.
func TestResolversFor(t *testing.T) {
	got := ResolversFor([]Dimension{DimensionRegion, DimensionService})
	if len(got) != 2 || got[0].Dimension() != DimensionRegion || got[1].Dimension() != DimensionService {
		t.Errorf("ordem inesperada: %+v", got)
	}
}
