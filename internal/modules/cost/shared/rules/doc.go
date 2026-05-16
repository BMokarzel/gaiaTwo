// Package rules carrega e valida `SharedCostRule` a partir de YAML.
//
// Convenções:
//
//   - 1 arquivo .yaml = 1 regra (mais simples para `git blame`).
//   - `id` é slug estável (lowercase, kebab-case); jamais renomear —
//     audit em `fct_cost_by_urn.rule_id` depende disso.
//   - `version` é monotônico crescente. Mudança de comportamento
//     incrementa version no MESMO arquivo (audit completo via git).
//   - `valid_from` / `valid_to` opcionais. Allocator ignora regra
//     cujo intervalo não cobre o período sendo rateado.
//
// O loader não toca em I/O além de ler arquivos: validação é estática
// e determinística. Erro de schema é fatal — relatar e abortar.
package rules
