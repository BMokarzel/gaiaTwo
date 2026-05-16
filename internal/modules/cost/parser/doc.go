// Package parser converte linhas brutas do CUR (Parquet) em `cost.Line`
// normalizadas (FOCUS-aligned).
//
// Responsabilidades:
//
//   - Mapping de colunas CUR v1/v2 → campos `cost.Line`.
//   - Derivar `EffectiveCost` segundo regra FOCUS
//     (`COALESCE(reservation_effective_cost, sp_effective_cost,
//     net_unblended)`).
//   - Classificar `ChargeCategory` e `PricingModel` a partir de
//     `line_item_line_item_type`.
//   - Extrair tags `resource_tags_user_*` para o map `Line.Tags`.
//
// NÃO faz: resolução de URN (responsabilidade de F-005), alocação de
// custos órfãos (F-005), persistência (sink).
package parser
