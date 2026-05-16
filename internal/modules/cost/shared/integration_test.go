package shared_test

import (
	"math"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/allocate"
	"costEngine/internal/modules/cost/shared"
	"costEngine/internal/modules/cost/shared/rules"
)

// TestIntegration_FullInvariant verifica a propriedade central de F-006:
//
//	sum(unalloc_in) ≈ sum(shared_out) + sum(remaining_out) + sum(drops)
//
// para os 3 tipos de regra rodando juntas no mesmo período.
func TestIntegration_FullInvariant(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Unix(1700000000, 0).UTC()

	// Pool já alocado direct ($300 total):
	//   region=us-east-1: $150 distribuídos em 2 URNs (i-a $100, i-b $50)
	//   region=eu-west-1: $100 distribuídos em 1 URN  (i-c $100)
	//   region=ap-south-1:$50  distribuídos em 1 URN  (i-d $50)
	totals := []shared.TotalByDim{
		{URN: "urn:ce:aws:111:compute/i-a", Dimension: "region", DimensionValue: "us-east-1", Amount: 100, Currency: "USD"},
		{URN: "urn:ce:aws:111:compute/i-b", Dimension: "region", DimensionValue: "us-east-1", Amount: 50, Currency: "USD"},
		{URN: "urn:ce:aws:111:compute/i-c", Dimension: "region", DimensionValue: "eu-west-1", Amount: 100, Currency: "USD"},
		{URN: "urn:ce:aws:111:compute/i-d", Dimension: "region", DimensionValue: "ap-south-1", Amount: 50, Currency: "USD"},
	}

	// Unalloc input:
	//   $90 de AWSSupport → proporcional por region (regra A)
	//   $40 de DataTransfer us-east-1 → by_destination (regra B)
	//   $100 de contract → static (regra C)
	//   $10 de Random sem regra → permanece em remaining
	unalloc := []allocate.UnallocatedRow{
		{BillingPeriod: period, AccountID: "111", Service: "AWSSupport", Reason: allocate.ReasonNoResourceID, Amount: 90, Currency: "USD", LineageCount: 1, AllocatedAt: now},
		{BillingPeriod: period, AccountID: "111", Service: "AWSDataTransfer", Reason: allocate.ReasonNoResourceID, Amount: 40, Currency: "USD", LineageCount: 1, AllocatedAt: now},
		{BillingPeriod: period, AccountID: "111", Service: "contract", Reason: allocate.ReasonNoResourceID, Amount: 100, Currency: "USD", LineageCount: 1, AllocatedAt: now},
		{BillingPeriod: period, AccountID: "111", Service: "Random", Reason: allocate.ReasonNotFound, Amount: 10, Currency: "USD", LineageCount: 1, AllocatedAt: now},
	}

	rs := []rules.Rule{
		{
			ID: "a-support-by-region", Version: 1, Type: rules.TypeProportional,
			Match: rules.Match{Service: "AWSSupport"}, Target: rules.Target{Dimension: "region"},
		},
		{
			ID: "b-egress-us", Version: 1, Type: rules.TypeByDestination,
			Match: rules.Match{Service: "AWSDataTransfer"}, Target: rules.Target{Region: "us-east-1"},
		},
		{
			ID: "c-contract", Version: 1, Type: rules.TypeStaticOverride,
			Match: rules.Match{Service: "contract"},
			Target: rules.Target{Shares: map[string]float64{
				"urn:ce:org:acme:team/payments": 0.6,
				"urn:ce:org:acme:team/growth":   0.4,
			}},
		},
	}

	out, err := shared.Apply(shared.Input{
		Period:          period,
		Unalloc:         unalloc,
		AllocatedTotals: totals,
		Rules:           rs,
		AllocatedAt:     now,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Invariante macro.
	var in float64
	for _, u := range unalloc {
		in += u.Amount
	}
	var outShared, outRem, outDrops float64
	for _, r := range out.Shared {
		outShared += r.Amount
	}
	for _, r := range out.RemainingUnalloc {
		outRem += r.Amount
	}
	for _, d := range out.Drops {
		outDrops += d.Amount
	}
	if math.Abs(in-(outShared+outRem+outDrops)) > 1e-6 {
		t.Errorf("invariante violada: in=%v shared=%v rem=%v drops=%v (sum=%v)",
			in, outShared, outRem, outDrops, outShared+outRem+outDrops)
	}

	// Sanidades específicas:
	// - support $90 distribuído inteiramente (sem drop).
	if outShared < 90+40+100-1e-6 {
		t.Errorf("shared total=%v, esperava ≥ 230", outShared)
	}
	// - remaining = $10 (a linha Random).
	if math.Abs(outRem-10) > 1e-6 {
		t.Errorf("remaining=%v want 10", outRem)
	}
	// - rule_id auditável em toda linha shared.
	for _, r := range out.Shared {
		if r.RuleID == "" || r.RuleVersion == 0 {
			t.Errorf("linha shared sem audit: %+v", r)
		}
		if r.AllocationType != allocate.AllocationShared {
			t.Errorf("alloc_type=%q want shared", r.AllocationType)
		}
	}
}

// TestIntegration_ReprocessIsIdempotent — rodar Apply 2x com mesmo
// input produz mesma saída (determinismo do engine, fora do CH).
func TestIntegration_ReprocessIsIdempotent(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Unix(1700000000, 0).UTC()
	totals := []shared.TotalByDim{
		{URN: "urn:x:a", Dimension: "region", DimensionValue: "us", Amount: 10},
	}
	unalloc := []allocate.UnallocatedRow{{Service: "X", Amount: 5, Currency: "USD"}}
	rs := []rules.Rule{{
		ID: "r", Version: 1, Type: rules.TypeByDestination,
		Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"},
	}}
	in := shared.Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs, AllocatedAt: now}
	o1, _ := shared.Apply(in)
	o2, _ := shared.Apply(in)
	if len(o1.Shared) != len(o2.Shared) {
		t.Fatalf("len mismatch: %d vs %d", len(o1.Shared), len(o2.Shared))
	}
	for i := range o1.Shared {
		if o1.Shared[i].URN != o2.Shared[i].URN || o1.Shared[i].Amount != o2.Shared[i].Amount {
			t.Errorf("row %d differs: %+v vs %+v", i, o1.Shared[i], o2.Shared[i])
		}
	}
}

// TestIntegration_RuleOutsideValidityWindow — regra com valid_to no
// passado não captura nada; tudo vira remaining.
func TestIntegration_RuleOutsideValidityWindow(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	expired := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	totals := []shared.TotalByDim{{URN: "u", Dimension: "region", DimensionValue: "us", Amount: 10}}
	unalloc := []allocate.UnallocatedRow{{Service: "X", Amount: 7, Currency: "USD"}}
	rs := []rules.Rule{{
		ID: "r", Version: 1, Type: rules.TypeByDestination,
		Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"},
		ValidTo: &expired,
	}}
	out, _ := shared.Apply(shared.Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs})
	if len(out.Shared) != 0 {
		t.Errorf("shared=%d want 0 (regra expirada)", len(out.Shared))
	}
	if len(out.RemainingUnalloc) != 1 {
		t.Errorf("remaining=%d want 1", len(out.RemainingUnalloc))
	}
}

// Sanity: a Row carrega node.URN (não string solta) — pega regressão se
// alguém trocar o tipo sem perceber.
func TestIntegration_RowTypesSane(t *testing.T) {
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	totals := []shared.TotalByDim{{URN: node.URN("urn:x:a"), Dimension: "region", DimensionValue: "us", Amount: 10}}
	unalloc := []allocate.UnallocatedRow{{Service: "X", Amount: 5, Currency: "USD"}}
	rs := []rules.Rule{{
		ID: "r", Version: 1, Type: rules.TypeByDestination,
		Match: rules.Match{Service: "X"}, Target: rules.Target{Region: "us"},
	}}
	out, _ := shared.Apply(shared.Input{Period: period, Unalloc: unalloc, AllocatedTotals: totals, Rules: rs})
	if len(out.Shared) != 1 {
		t.Fatalf("shared=%d want 1", len(out.Shared))
	}
	var _ node.URN = out.Shared[0].URN
}
