package allocate

import (
	"context"
	"math"
	"sort"
	"testing"
	"time"
)

func approx(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

// fakeSource implementa LineSource a partir de um slice estático.
type fakeSource struct {
	lines []RawLine
	err   error
}

func (f *fakeSource) Stream(ctx context.Context, _ time.Time, yield func(RawLine) error) error {
	if f.err != nil {
		return f.err
	}
	for _, l := range f.lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := yield(l); err != nil {
			return err
		}
	}
	return nil
}

// TestAggregator_GroupsByResource garante que linhas com mesmo
// (account, resource_id, region, service) somam num único bucket.
func TestAggregator_GroupsByResource(t *testing.T) {
	src := &fakeSource{lines: []RawLine{
		{AccountID: "111", ResourceID: "i-1", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 0.10, Currency: "USD"},
		{AccountID: "111", ResourceID: "i-1", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 0.20, Currency: "USD"},
		{AccountID: "111", ResourceID: "i-2", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 0.50, Currency: "USD"},
		{AccountID: "222", ResourceID: "i-1", Region: "us-east-1", Service: "AmazonEC2", EffectiveCost: 1.00, Currency: "USD"},
	}}
	a := NewAggregator()
	if err := a.Collect(context.Background(), src, time.Time{}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if a.LinesRead != 4 {
		t.Errorf("LinesRead=%d want 4", a.LinesRead)
	}
	if got, want := a.TotalCost, 1.80; !approx(got, want) {
		t.Errorf("TotalCost=%v want %v", got, want)
	}
	if len(a.Buckets) != 3 {
		t.Errorf("Buckets=%d want 3", len(a.Buckets))
	}

	// (111, i-1) deve ter 0.30 e LineageCount=2
	k := resourceKey{AccountID: "111", ResourceID: "i-1", Region: "us-east-1", Service: "AmazonEC2"}
	if !approx(a.Buckets[k].Amount, 0.30) || a.Buckets[k].LineageCount != 2 {
		t.Errorf("bucket (111,i-1) = %+v want 0.30/2", a.Buckets[k])
	}
}

// TestAggregator_UnallocatedNoResourceID separa linhas sem resource_id
// no mapa Unalloc, agrupado por (account, service).
func TestAggregator_UnallocatedNoResourceID(t *testing.T) {
	src := &fakeSource{lines: []RawLine{
		{AccountID: "111", ResourceID: "", Service: "AmazonEC2", EffectiveCost: 0.05, Currency: "USD"}, // tax
		{AccountID: "111", ResourceID: "", Service: "AmazonEC2", EffectiveCost: 0.05, Currency: "USD"},
		{AccountID: "111", ResourceID: "", Service: "AWSSupport", EffectiveCost: 1.00, Currency: "USD"},
	}}
	a := NewAggregator()
	if err := a.Collect(context.Background(), src, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if len(a.Buckets) != 0 {
		t.Errorf("Buckets=%d, want 0 (todas sem resource_id)", len(a.Buckets))
	}
	if len(a.Unalloc) != 2 {
		t.Errorf("Unalloc=%d want 2", len(a.Unalloc))
	}
	ec2 := a.Unalloc[unallocatedKey{AccountID: "111", Service: "AmazonEC2"}]
	if ec2 == nil || !approx(ec2.Amount, 0.10) || ec2.LineageCount != 2 {
		t.Errorf("unalloc ec2 = %+v, want amount=0.10/2", ec2)
	}
}

// TestAggregator_ResourceIDsByAccount produz set por conta para
// alimentar o resolver.
func TestAggregator_ResourceIDsByAccount(t *testing.T) {
	a := NewAggregator()
	_ = a.Collect(context.Background(), &fakeSource{lines: []RawLine{
		{AccountID: "111", ResourceID: "i-1", Service: "AmazonEC2", EffectiveCost: 1},
		{AccountID: "111", ResourceID: "i-2", Service: "AmazonEC2", EffectiveCost: 1},
		{AccountID: "111", ResourceID: "i-1", Service: "AmazonEC2", EffectiveCost: 1}, // dup
		{AccountID: "222", ResourceID: "i-1", Service: "AmazonEC2", EffectiveCost: 1},
	}}, time.Time{})

	got := a.ResourceIDsByAccount()
	if len(got["111"]) != 2 {
		t.Errorf("account 111 ids = %v, want 2 distinct", got["111"])
	}
	if len(got["222"]) != 1 {
		t.Errorf("account 222 ids = %v, want 1 distinct", got["222"])
	}
	sort.Strings(got["111"])
	if got["111"][0] != "i-1" || got["111"][1] != "i-2" {
		t.Errorf("account 111 ids unexpected: %v", got["111"])
	}
	if u := a.UniqueResIDs(); u != 3 {
		t.Errorf("UniqueResIDs=%d want 3", u)
	}
}

// TestAggregator_MixedCurrency sinaliza moeda mista no bucket.
func TestAggregator_MixedCurrency(t *testing.T) {
	a := NewAggregator()
	_ = a.Collect(context.Background(), &fakeSource{lines: []RawLine{
		{AccountID: "111", ResourceID: "i-1", Service: "AmazonEC2", EffectiveCost: 1, Currency: "USD"},
		{AccountID: "111", ResourceID: "i-1", Service: "AmazonEC2", EffectiveCost: 1, Currency: "EUR"},
	}}, time.Time{})
	k := resourceKey{AccountID: "111", ResourceID: "i-1", Region: "", Service: "AmazonEC2"}
	if a.Buckets[k].Currency != "MIXED" {
		t.Errorf("currency = %q, want MIXED", a.Buckets[k].Currency)
	}
}
