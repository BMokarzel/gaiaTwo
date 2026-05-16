// Package shared implementa o rateio de custo compartilhado (F-006).
//
// Lê `fct_unallocated_cost` (linhas CUR que F-005 não conseguiu atribuir
// a uma URN — taxa, suporte, data transfer agregado, savings plans) e
// aplica regras versionadas (`SharedCostRule`) para distribuir esses
// valores em `fct_cost_by_urn` com `allocation_type='shared'`.
//
// Regras vivem como YAML em `config/shared_rules/*.yaml` (ADR-003) —
// não como Kind no grafo. Cada linha shared carrega `rule_id` +
// `rule_version` para audit trail; o conteúdo da regra em qualquer
// ponto do tempo é recuperável via `git log`.
//
// Tipos de regra suportados:
//
//   - proportional_to_allocated — rateia na proporção do que já foi
//     alocado por dimensão (ex.: AWS Support por team).
//
//   - by_destination — atribui a recursos de uma região destino
//     (ex.: data transfer inter-region).
//
//   - static_override — percentuais fixos para URNs nomeadas
//     (uso pontual, sempre auditado).
//
// Pipeline: rules.Load → engine.Apply → sink.Write.
package shared
