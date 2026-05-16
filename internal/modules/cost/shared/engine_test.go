package shared

import (
	"math"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/allocate"
	"costEngine/internal/modules/cost/shared/rules"
)

func epsEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func sumAmount[T any](rows []T, get func(T) float64) float64 {
	var s float64
	for _, r := range rows {
		s += get(r)
	}
	return s
}

func TestApply_ProportionalToAllocated(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Unix(1700000000, 0).UTC()

	// $90 já alocado: $60 em region=us, $30 em region=eu.
	totals := []TotalByDim{
		{URN: "urn:ce:aws:111:compute/i-a", Dimension: "region", DimensionValue: "us-east-1", Amount: 60, Currency: "USD"},
		{URN: "urn:ce:aws:111:compute/i-b", Dimension: "region", DimensionValue: "eu-west-1", Amount: 30, Currency: "USD"},
	}
	// $100 de support a ratear na proporção de region.
	unalloc := []allocate.UnallocatedRow{
		{BillingPeriod: period, AccountID: "111", Service: "AWSSupport", Reason: allocate.ReasonNoResourceID, Amount: 100, Currency: "USD", LineageCount: 1, AllocatedAt: now},
	}
	rs := []rules.Rule{{
		ID: "support-by-region", Version: 1, Type: rules.TypeProportional,
		Match:  rules.Match{Service: "AWSSupport"},
		Target: rules.Target{Dimension: "region"},
	}}
	out, err := Apply(Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs, AllocatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Shared) != 2 {
		t.Fatalf("shared rows=%d want 2", len(out.Shared))
	}
	// us-east-1 deve receber 60/90 * 100 ≈ 66.67; eu-west-1 receberia 33.33.
	var byVal = map[string]float64{}
	for _, r := range out.Shared {
		byVal[r.DimensionValue] += r.Amount
		if r.AllocationType != allocate.AllocationShared {
			t.Errorf("alloc_type=%q want shared", r.AllocationType)
		}
		if r.RuleID != "support-by-region" || r.RuleVersion != 1 {
			t.Errorf("audit fields wrong: %+v", r)
		}
	}
	if !epsEqual(byVal["us-east-1"], 100*60.0/90.0) {
		t.Errorf("us=%v want %v", byVal["us-east-1"], 100*60.0/90.0)
	}
	if !epsEqual(byVal["eu-west-1"], 100*30.0/90.0) {
		t.Errorf("eu=%v want %v", byVal["eu-west-1"], 100*30.0/90.0)
	}
	if total := sumAmount(out.Shared, func(r allocate.Row) float64 { return r.Amount }); !epsEqual(total, 100.0) {
		t.Errorf("total shared=%v want 100", total)
	}
	if len(out.RemainingUnalloc) != 0 {
		t.Errorf("remaining=%d want 0", len(out.RemainingUnalloc))
	}
}

func TestApply_ProportionalEmptyPool(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	unalloc := []allocate.UnallocatedRow{
		{BillingPeriod: period, Service: "X", Amount: 50, Currency: "USD", LineageCount: 1},
	}
	rs := []rules.Rule{{
		ID: "x", Version: 1, Type: rules.TypeProportional,
		Match: rules.Match{Service: "X"}, Target: rules.Target{Dimension: "team"},
	}}
	out, _ := Apply(Input{Period: period, Unalloc: unalloc, Rules: rs})
	if len(out.Shared) != 0 {
		t.Errorf("shared=%d want 0", len(out.Shared))
	}
	if len(out.Drops) != 1 {
		t.Fatalf("drops=%d want 1", len(out.Drops))
	}
	if out.Drops[0].Amount != 50 {
		t.Errorf("drop amount=%v want 50", out.Drops[0].Amount)
	}
}

func TestApply_ByDestination(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	totals := []TotalByDim{
		{URN: "urn:ce:aws:111:compute/i-a", Dimension: "region", DimensionValue: "us-east-1", Amount: 80, Currency: "USD"},
		{URN: "urn:ce:aws:111:compute/i-b", Dimension: "region", DimensionValue: "us-east-1", Amount: 20, Currency: "USD"},
		{URN: "urn:ce:aws:111:compute/i-c", Dimension: "region", DimensionValue: "eu-west-1", Amount: 100, Currency: "USD"},
	}
	unalloc := []allocate.UnallocatedRow{
		{BillingPeriod: period, Service: "AWSDataTransfer", Amount: 50, Currency: "USD", LineageCount: 1},
	}
	rs := []rules.Rule{{
		ID: "egress-us", Version: 1, Type: rules.TypeByDestination,
		Match: rules.Match{Service: "AWSDataTransfer"}, Target: rules.Target{Region: "us-east-1"},
	}}
	out, err := Apply(Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Shared) != 2 {
		t.Fatalf("shared=%d want 2", len(out.Shared))
	}
	// i-a: 80/100 * 50 = 40; i-b: 20/100 * 50 = 10.
	got := map[node.URN]float64{}
	for _, r := range out.Shared {
		got[r.URN] += r.Amount
	}
	if !epsEqual(got["urn:ce:aws:111:compute/i-a"], 40) {
		t.Errorf("i-a=%v want 40", got["urn:ce:aws:111:compute/i-a"])
	}
	if !epsEqual(got["urn:ce:aws:111:compute/i-b"], 10) {
		t.Errorf("i-b=%v want 10", got["urn:ce:aws:111:compute/i-b"])
	}
}

func TestApply_StaticOverride(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	unalloc := []allocate.UnallocatedRow{
		{BillingPeriod: period, Service: "contract", Amount: 1000, Currency: "USD", LineageCount: 1},
		{BillingPeriod: period, Service: "contract", Amount: 200, Currency: "USD", LineageCount: 1},
	}
	rs := []rules.Rule{{
		ID: "annual", Version: 7, Type: rules.TypeStaticOverride,
		Match: rules.Match{Service: "contract"},
		Target: rules.Target{Shares: map[string]float64{
			"urn:ce:org:acme:team/p":  0.7,
			"urn:ce:org:acme:team/q":  0.3,
		}},
	}}
	out, err := Apply(Input{Period: period, Unalloc: unalloc, Rules: rs})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Shared) != 2 {
		t.Fatalf("shared=%d want 2", len(out.Shared))
	}
	got := map[node.URN]float64{}
	for _, r := range out.Shared {
		got[r.URN] += r.Amount
		if r.RuleVersion != 7 {
			t.Errorf("rule version=%d want 7", r.RuleVersion)
		}
	}
	if !epsEqual(got["urn:ce:org:acme:team/p"], 1200*0.7) {
		t.Errorf("p=%v want 840", got["urn:ce:org:acme:team/p"])
	}
	if !epsEqual(got["urn:ce:org:acme:team/q"], 1200*0.3) {
		t.Errorf("q=%v want 360", got["urn:ce:org:acme:team/q"])
	}
}

func TestApply_NoMatchGoesRemaining(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	unalloc := []allocate.UnallocatedRow{
		{BillingPeriod: period, Service: "Y", Amount: 7, Currency: "USD", LineageCount: 1},
	}
	rs := []rules.Rule{{
		ID: "x", Version: 1, Type: rules.TypeByDestination,
		Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"},
	}}
	out, _ := Apply(Input{Period: period, Unalloc: unalloc, Rules: rs})
	if len(out.Shared) != 0 {
		t.Errorf("shared=%d want 0", len(out.Shared))
	}
	if len(out.RemainingUnalloc) != 1 || out.RemainingUnalloc[0].Amount != 7 {
		t.Errorf("remaining wrong: %+v", out.RemainingUnalloc)
	}
}

func TestApply_RuleOutsideValidityIgnored(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	vt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	totals := []TotalByDim{{URN: "u", Dimension: "region", DimensionValue: "us", Amount: 10}}
	unalloc := []allocate.UnallocatedRow{{BillingPeriod: period, Service: "X", Amount: 5, Currency: "USD"}}
	rs := []rules.Rule{{
		ID: "expired", Version: 1, Type: rules.TypeByDestination,
		Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"},
		ValidTo: &vt,
	}}
	out, _ := Apply(Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs})
	if len(out.Shared) != 0 {
		t.Errorf("shared=%d want 0 (rule expired)", len(out.Shared))
	}
	if len(out.RemainingUnalloc) != 1 {
		t.Errorf("remaining=%d want 1", len(out.RemainingUnalloc))
	}
}

func TestApply_NonOverlapFirstRuleWins(t *testing.T) {
	// Duas regras matchariam a mesma linha. Como o engine consome a
	// primeira por id ordenado, "a-rule" ganha; "b-rule" não vê nada.
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	totals := []TotalByDim{{URN: "u", Dimension: "region", DimensionValue: "us", Amount: 10}}
	unalloc := []allocate.UnallocatedRow{{BillingPeriod: period, Service: "X", Amount: 50, Currency: "USD"}}
	rs := []rules.Rule{
		{ID: "a-rule", Version: 1, Type: rules.TypeByDestination, Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"}},
		{ID: "b-rule", Version: 1, Type: rules.TypeByDestination, Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"}},
	}
	out, _ := Apply(Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs})
	if len(out.Shared) != 1 {
		t.Fatalf("shared=%d want 1", len(out.Shared))
	}
	if out.Shared[0].RuleID != "a-rule" {
		t.Errorf("rule_id=%q want a-rule", out.Shared[0].RuleID)
	}
}
