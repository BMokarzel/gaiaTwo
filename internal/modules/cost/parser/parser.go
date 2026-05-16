package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"costEngine/internal/entity/cost"
	"costEngine/internal/entity/node"
)

// Context carrega metadados de proveniência aplicados a cada linha
// produzida pelo parser. Reidratado por execução do ingestor.
type Context struct {
	Provider   node.ProviderID // ex.: node.ProviderAWS
	ReportID   string          // nome do relatório CUR
	RunID      string          // identificador da execução (manifest assemblyId)
	SourceFile string          // arquivo Parquet de origem
	IngestedAt time.Time       // transaction time da ingestão
}

// Parse converte uma linha raw do CUR em uma `cost.Line` normalizada.
//
// Aceita tanto CUR v1 (nomes legacy: `lineItem/UsageStartDate`) quanto
// CUR v2 (snake_case: `line_item_usage_start_date`). A normalização
// de colunas é feita por `normKey` (lowercase + remove `/`).
//
// Retorna `cost.ErrInvalidLine` quando colunas obrigatórias estão
// ausentes ou inválidas; o caller deve persistir o erro como
// `cost.ParseError` (não fatal).
func Parse(ctx Context, raw map[string]any) (cost.Line, error) {
	r := view(raw)

	billingPeriod, err := r.timestamp("bill_billing_period_start_date")
	if err != nil {
		return cost.Line{}, fmt.Errorf("%w: billing_period_start: %v", cost.ErrInvalidLine, err)
	}
	usageStart, err := r.timestamp("line_item_usage_start_date")
	if err != nil {
		return cost.Line{}, fmt.Errorf("%w: usage_start: %v", cost.ErrInvalidLine, err)
	}
	usageEnd, err := r.timestamp("line_item_usage_end_date")
	if err != nil {
		return cost.Line{}, fmt.Errorf("%w: usage_end: %v", cost.ErrInvalidLine, err)
	}

	itemType := r.str("line_item_line_item_type")
	line := cost.Line{
		ReportID:    ctx.ReportID,
		LineItemID:  r.firstStr("identity_line_item_id", "lineitem_identity"),
		SourceFile:  ctx.SourceFile,
		SourceRunID: ctx.RunID,

		BillingPeriodStart: billingPeriod,
		UsageStart:         usageStart,
		UsageEnd:           usageEnd,
		IngestedAt:         ctx.IngestedAt,

		Provider:   ctx.Provider,
		AccountID:  r.str("line_item_usage_account_id"),
		ResourceID: r.str("line_item_resource_id"),
		Region:     r.firstStr("product_region", "product_region_code"),

		Service:   r.str("line_item_product_code"),
		UsageType: r.str("line_item_usage_type"),
		Operation: r.str("line_item_operation"),
		Charge:    classifyCharge(itemType),
		Pricing:   classifyPricing(itemType, r.str("line_item_legal_entity")),

		UsageAmount:   r.float("line_item_usage_amount"),
		UsageUnit:     r.firstStr("pricing_unit", "line_item_usage_unit"),
		ListCost:      r.float("pricing_public_on_demand_cost"),
		BilledCost:    r.firstFloat("line_item_net_unblended_cost", "line_item_unblended_cost"),
		EffectiveCost: effectiveCost(r),
		Currency:      r.firstStrDefault("USD", "line_item_currency_code", "pricing_currency"),

		Tags:         extractMap(raw, "resource_tags_user_"),
		CostCategory: extractMap(raw, "cost_category_"),
	}

	if line.LineItemID == "" {
		// CUR v1 nem sempre tem identity_line_item_id; sintetiza um
		// a partir de (resource_id, usage_start, usage_type) para o
		// sink ainda conseguir deduplicar dentro do mesmo report+mês.
		line.LineItemID = syntheticID(line)
	}

	return line, nil
}

// syntheticID gera um identificador determinístico quando o CUR não
// expõe `identity/LineItemId`. NÃO substitui o canônico — só evita
// que múltiplas linhas colidam no `(report_id, line_item_id, period)`.
func syntheticID(l cost.Line) string {
	return strings.Join([]string{
		l.ResourceID,
		l.UsageStart.UTC().Format(time.RFC3339),
		l.UsageType,
		l.Operation,
		strconv.FormatFloat(l.UsageAmount, 'g', -1, 64),
	}, "|")
}

// classifyCharge mapeia line_item_type → ChargeCategory (FOCUS).
func classifyCharge(itemType string) cost.ChargeCategory {
	switch strings.ToLower(itemType) {
	case "usage", "discountedusage", "savingsplancoveredusage", "savingsplannegation":
		return cost.ChargeUsage
	case "rifee", "savingsplanrecurringfee", "savingsplanupfrontfee":
		return cost.ChargePurchase
	case "tax":
		return cost.ChargeTax
	case "credit", "refund":
		return cost.ChargeCredit
	case "fee", "edpdiscount", "privaterateadjustment", "bundleddiscount":
		return cost.ChargeAdjustment
	default:
		return cost.ChargeUsage
	}
}

// classifyPricing mapeia line_item_type → PricingModel.
func classifyPricing(itemType, _ string) cost.PricingModel {
	low := strings.ToLower(itemType)
	switch {
	case strings.HasPrefix(low, "savingsplan"):
		return cost.PricingSavingsPlan
	case low == "discountedusage" || low == "rifee":
		return cost.PricingReserved
	case strings.Contains(low, "spot"):
		return cost.PricingSpot
	default:
		return cost.PricingOnDemand
	}
}

// effectiveCost aplica COALESCE FOCUS:
//
//	COALESCE(reservation/EffectiveCost,
//	         savingsPlan/SavingsPlanEffectiveCost,
//	         lineItem/NetUnblendedCost,
//	         lineItem/UnblendedCost)
func effectiveCost(r rowView) float64 {
	for _, k := range []string{
		"reservation_effective_cost",
		"savings_plan_savings_plan_effective_cost",
		"line_item_net_unblended_cost",
		"line_item_unblended_cost",
	} {
		if v, ok := r.tryFloat(k); ok {
			return v
		}
	}
	return 0
}

// extractMap coleta colunas com prefixo dado (ex.: "resource_tags_user_")
// num map "chave → string". O prefixo é removido da chave final.
func extractMap(raw map[string]any, prefix string) map[string]string {
	var out map[string]string
	for k, v := range raw {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[strings.TrimPrefix(k, prefix)] = s
	}
	return out
}

// ----- row view (lookup tolerante a CUR v1/v2) -----

type rowView struct{ m map[string]any }

func view(raw map[string]any) rowView {
	// normaliza chaves uma vez: lowercase, "/" → "_".
	norm := make(map[string]any, len(raw))
	for k, v := range raw {
		norm[normKey(k)] = v
	}
	return rowView{m: norm}
}

func normKey(k string) string {
	// Achata separadores comuns do CUR.
	k = strings.ReplaceAll(k, "/", "_")
	k = strings.ReplaceAll(k, ".", "_")
	// Insere "_" em fronteiras camelCase: aB → a_B; aBC → a_BC; ABc → A_Bc.
	// Cobre v1 "lineItem/UsageStartDate" → "line_item_usage_start_date".
	var b strings.Builder
	b.Grow(len(k) + 8)
	for i, r := range k {
		if i > 0 && isUpper(r) {
			prev := rune(k[i-1])
			// caso aB → a_B
			if isLower(prev) || isDigit(prev) {
				b.WriteByte('_')
			} else if isUpper(prev) && i+1 < len(k) && isLower(rune(k[i+1])) {
				// caso ABc → A_Bc
				b.WriteByte('_')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func (r rowView) str(key string) string {
	v, ok := r.m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func (r rowView) firstStr(keys ...string) string {
	for _, k := range keys {
		if s := r.str(k); s != "" {
			return s
		}
	}
	return ""
}

func (r rowView) firstStrDefault(def string, keys ...string) string {
	if s := r.firstStr(keys...); s != "" {
		return s
	}
	return def
}

func (r rowView) tryFloat(key string) (float64, bool) {
	v, ok := r.m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	case string:
		if x == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func (r rowView) float(key string) float64 {
	v, _ := r.tryFloat(key)
	return v
}

func (r rowView) firstFloat(keys ...string) float64 {
	for _, k := range keys {
		if v, ok := r.tryFloat(k); ok {
			return v
		}
	}
	return 0
}

// timestamp aceita string RFC3339, ISO 8601, ou int64 epoch-millis/seconds.
func (r rowView) timestamp(key string) (time.Time, error) {
	v, ok := r.m[key]
	if !ok || v == nil {
		return time.Time{}, fmt.Errorf("missing column %s", key)
	}
	switch x := v.(type) {
	case string:
		// Tenta RFC3339, depois layouts conhecidos do CUR.
		for _, layout := range []string{
			time.RFC3339Nano, time.RFC3339,
			"2006-01-02T15:04:05.000Z",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
		} {
			if t, err := time.Parse(layout, x); err == nil {
				return t.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("unparseable timestamp %q", x)
	case int64:
		// CUR v2 Parquet costuma serializar TIMESTAMP_MICROS.
		// Heurística: >1e15 ⇒ micros; >1e12 ⇒ millis; senão segundos.
		switch {
		case x > 1_000_000_000_000_000:
			return time.UnixMicro(x).UTC(), nil
		case x > 1_000_000_000_000:
			return time.UnixMilli(x).UTC(), nil
		default:
			return time.Unix(x, 0).UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp type %T", v)
}
