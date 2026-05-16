package parser

import (
	"errors"
	"testing"
	"time"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
)

func baseCtx() Context {
	return Context{
		Provider:   node.ProviderAWS,
		ReportID:   "cost-explorer-prod",
		RunID:      "run-2026-05-14",
		SourceFile: "cur-00001.parquet",
		IngestedAt: time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC),
	}
}

func baseRow() map[string]any {
	return map[string]any{
		"identity_line_item_id":             "abc-line-42",
		"bill_billing_period_start_date":    "2026-05-01T00:00:00Z",
		"line_item_usage_start_date":        "2026-05-14T08:00:00Z",
		"line_item_usage_end_date":          "2026-05-14T09:00:00Z",
		"line_item_usage_account_id":        "111111111111",
		"line_item_resource_id":             "i-abc",
		"line_item_product_code":            "AmazonEC2",
		"line_item_usage_type":              "BoxUsage:m6i.large",
		"line_item_operation":               "RunInstances",
		"line_item_line_item_type":          "Usage",
		"line_item_usage_amount":            1.0,
		"line_item_unblended_cost":          0.1,
		"line_item_net_unblended_cost":      0.09,
		"pricing_public_on_demand_cost":     0.10,
		"pricing_unit":                      "Hrs",
		"line_item_currency_code":           "USD",
		"product_region":                    "us-east-1",
		"resource_tags_user_env":            "prod",
		"resource_tags_user_team":           "platform",
		"resource_tags_user_empty":          "",
	}
}

func TestParse_BaselineUsage(t *testing.T) {
	l, err := Parse(baseCtx(), baseRow())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.LineItemID != "abc-line-42" {
		t.Errorf("LineItemID = %q", l.LineItemID)
	}
	if l.Service != "AmazonEC2" {
		t.Errorf("Service = %q", l.Service)
	}
	if l.ResourceID != "i-abc" {
		t.Errorf("ResourceID = %q", l.ResourceID)
	}
	if l.Region != "us-east-1" {
		t.Errorf("Region = %q", l.Region)
	}
	if l.Charge != cost.ChargeUsage {
		t.Errorf("Charge = %q, want usage", l.Charge)
	}
	if l.Pricing != cost.PricingOnDemand {
		t.Errorf("Pricing = %q, want on_demand", l.Pricing)
	}
	if l.BilledCost != 0.09 {
		t.Errorf("BilledCost = %v, want 0.09 (net)", l.BilledCost)
	}
	if l.EffectiveCost != 0.09 {
		t.Errorf("EffectiveCost = %v, want 0.09", l.EffectiveCost)
	}
	if l.Currency != "USD" {
		t.Errorf("Currency = %q", l.Currency)
	}
	if l.Tags["env"] != "prod" || l.Tags["team"] != "platform" {
		t.Errorf("Tags = %v", l.Tags)
	}
	if _, ok := l.Tags["empty"]; ok {
		t.Errorf("empty tag should be dropped")
	}
	if l.IngestedAt.IsZero() || l.SourceFile != "cur-00001.parquet" {
		t.Errorf("ingestion meta missing: %+v", l)
	}
	if !l.HasResource() {
		t.Error("expected HasResource=true")
	}
}

func TestParse_EffectiveCostCoalesce(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   float64
	}{
		{
			name: "RI effective cost wins",
			mutate: func(r map[string]any) {
				r["reservation_effective_cost"] = 0.05
			},
			want: 0.05,
		},
		{
			name: "SP effective cost when no RI",
			mutate: func(r map[string]any) {
				r["savings_plan_savings_plan_effective_cost"] = 0.06
			},
			want: 0.06,
		},
		{
			name: "net unblended when no RI/SP",
			mutate: func(r map[string]any) {
				// baseline already has net=0.09
			},
			want: 0.09,
		},
		{
			name: "unblended fallback when no net",
			mutate: func(r map[string]any) {
				delete(r, "line_item_net_unblended_cost")
			},
			want: 0.1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := baseRow()
			c.mutate(row)
			l, err := Parse(baseCtx(), row)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if l.EffectiveCost != c.want {
				t.Errorf("EffectiveCost = %v, want %v", l.EffectiveCost, c.want)
			}
		})
	}
}

func TestParse_ChargeClassification(t *testing.T) {
	cases := map[string]cost.ChargeCategory{
		"Usage":                   cost.ChargeUsage,
		"DiscountedUsage":         cost.ChargeUsage,
		"SavingsPlanCoveredUsage": cost.ChargeUsage,
		"RIFee":                   cost.ChargePurchase,
		"SavingsPlanRecurringFee": cost.ChargePurchase,
		"Tax":                     cost.ChargeTax,
		"Credit":                  cost.ChargeCredit,
		"Refund":                  cost.ChargeCredit,
		"Fee":                     cost.ChargeAdjustment,
		"EdpDiscount":             cost.ChargeAdjustment,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			r := baseRow()
			r["line_item_line_item_type"] = in
			l, err := Parse(baseCtx(), r)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if l.Charge != want {
				t.Errorf("charge for %q = %q, want %q", in, l.Charge, want)
			}
		})
	}
}

func TestParse_PricingClassification(t *testing.T) {
	cases := map[string]cost.PricingModel{
		"Usage":                   cost.PricingOnDemand,
		"DiscountedUsage":         cost.PricingReserved,
		"RIFee":                   cost.PricingReserved,
		"SavingsPlanCoveredUsage": cost.PricingSavingsPlan,
		"SavingsPlanRecurringFee": cost.PricingSavingsPlan,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			r := baseRow()
			r["line_item_line_item_type"] = in
			l, err := Parse(baseCtx(), r)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if l.Pricing != want {
				t.Errorf("pricing for %q = %q, want %q", in, l.Pricing, want)
			}
		})
	}
}

func TestParse_MissingTimestampReturnsInvalidLine(t *testing.T) {
	r := baseRow()
	delete(r, "line_item_usage_start_date")
	_, err := Parse(baseCtx(), r)
	if !errors.Is(err, cost.ErrInvalidLine) {
		t.Fatalf("err = %v, want ErrInvalidLine", err)
	}
}

func TestParse_LegacyCURv1Keys(t *testing.T) {
	// CUR v1 usa "lineItem/UsageStartDate" — o normalizador deve
	// achatar para snake_case.
	row := map[string]any{
		"bill/BillingPeriodStartDate": "2026-05-01T00:00:00Z",
		"lineItem/UsageStartDate":     "2026-05-14T08:00:00Z",
		"lineItem/UsageEndDate":       "2026-05-14T09:00:00Z",
		"lineItem/UsageAccountId":     "111111111111",
		"lineItem/ResourceId":         "i-xyz",
		"lineItem/ProductCode":        "AmazonEC2",
		"lineItem/LineItemType":       "Usage",
		"lineItem/UnblendedCost":      0.05,
		"lineItem/CurrencyCode":       "USD",
		"identity/LineItemId":         "v1-line-1",
	}
	l, err := Parse(baseCtx(), row)
	if err != nil {
		t.Fatalf("Parse v1: %v", err)
	}
	if l.AccountID != "111111111111" || l.ResourceID != "i-xyz" {
		t.Errorf("v1 mapping failed: %+v", l)
	}
	if l.LineItemID != "v1-line-1" {
		t.Errorf("v1 LineItemID = %q", l.LineItemID)
	}
	if l.BilledCost != 0.05 {
		t.Errorf("v1 BilledCost = %v", l.BilledCost)
	}
}

func TestParse_UnattributableLineHasNoResourceID(t *testing.T) {
	r := baseRow()
	delete(r, "line_item_resource_id")
	r["line_item_line_item_type"] = "Tax"
	l, err := Parse(baseCtx(), r)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.HasResource() {
		t.Error("Tax line should not have resource")
	}
	if l.Charge != cost.ChargeTax {
		t.Errorf("Charge = %q", l.Charge)
	}
}

func TestParse_SyntheticIDWhenMissing(t *testing.T) {
	r := baseRow()
	delete(r, "identity_line_item_id")
	l, err := Parse(baseCtx(), r)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.LineItemID == "" {
		t.Fatal("expected synthetic id")
	}
	// Determinístico: mesma entrada → mesmo ID.
	l2, _ := Parse(baseCtx(), baseRow())
	_ = l2
	r2 := baseRow()
	delete(r2, "identity_line_item_id")
	l3, _ := Parse(baseCtx(), r2)
	if l.LineItemID != l3.LineItemID {
		t.Errorf("synthetic ID not deterministic: %q vs %q", l.LineItemID, l3.LineItemID)
	}
}

func TestParse_TimestampEpochMicros(t *testing.T) {
	r := baseRow()
	micros := time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC).UnixMicro()
	r["line_item_usage_start_date"] = micros
	l, err := Parse(baseCtx(), r)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC)
	if !l.UsageStart.Equal(want) {
		t.Errorf("UsageStart = %v, want %v", l.UsageStart, want)
	}
}

func TestParse_StringNumeric(t *testing.T) {
	// Parquet exportado como CSV pode entregar números como string.
	r := baseRow()
	r["line_item_unblended_cost"] = "0.42"
	l, err := Parse(baseCtx(), r)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.BilledCost != 0.09 {
		// net ainda tem precedência
		t.Errorf("expected net to win, got %v", l.BilledCost)
	}
	delete(r, "line_item_net_unblended_cost")
	l, _ = Parse(baseCtx(), r)
	if l.BilledCost != 0.42 {
		t.Errorf("string numeric parse failed: %v", l.BilledCost)
	}
}
