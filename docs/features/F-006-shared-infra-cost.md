---
id: F-006
title: Shared infra cost (data transfer, support, savings plans)
status: done
modules: [cost]
depends_on: [F-005]
modeling_impact: no
adrs: [ADR-003]
epic: E-002
updated: 2026-05-14
---

# F-006 — Shared infra cost

## Problema

Parte do custo no CUR não tem `resource_id` (ou tem mas o recurso é
"compartilhado"): data transfer entre regiões, AWS Support,
Savings Plans / Reserved Instances credits, taxes. Ignorar isso
subestima o custo total dos serviços/times. Atribuir tudo a "unallocated"
piora drill-down. Precisa de regras explícitas de rateio.

**Para quem:** finance/FinOps e qualquer dimensão de cost reporting
que prometa fidelidade (capability, team, business_area).

**Dor sem ela:** "alocado" não bate com a fatura AWS; usuário perde
confiança.

## Escopo

**Inclui:**
- Catálogo de regras `SharedCostRule` (config-as-data inicial; YAML
  versionado no repo).
- Tipos de regra suportados no MVP:
  - **Proporcional ao alocado:** rateia `amount` na proporção do que
    já foi alocado por dimensão (ex.: AWS Support).
  - **Pelo destino:** linhas de data transfer com `Region` de destino
    rateiam para os recursos daquela região.
  - **Override estático:** atribuir percentuais fixos a URNs
    nomeadas (uso pontual).
- Job `cost-allocate-shared` roda após `cost-allocate` (F-005).
- Reprocessamento idempotente por período.

**NÃO inclui:**
- UI para editar regras (vive como YAML por enquanto).
- Forecast.
- Refund handling complexo (creditos negativos passam direto).

**Precondições:**
- F-005 completo (precisa do total alocado para regra proporcional).
- Tabela `fct_unallocated_cost` populada.

## Toque no grafo

- **Lê:** mesmas dimensões que F-005 (ownership chain).
- **Escreve:** nada no grafo. Escreve em `fct_cost_by_urn` com
  `allocation_type = "shared"` para distinguir.
- **Novos Kinds/edges:** possivelmente `SharedCostRule` como Kind
  do plano `cost` se quisermos versionar regras no grafo —
  **`modeling_impact: unknown`**, rodar prompt 02 antes de iniciar.
- **Bitemporal:** regras têm `valid_from/valid_to` para mudança ao
  longo do tempo.

## Critérios de aceite

- [x] Dado regra `proportional_to_allocated`, quando `ce allocate-shared
      --period=2026-03`, então cada valor da dimensão alvo recebe linha
      em `fct_cost_by_urn` com `allocation_type=shared` somando ao valor
      matched na proporção do `direct` correspondente.
      *(`TestApply_ProportionalToAllocated`, `TestIntegration_FullInvariant`)*
- [x] Invariante: `sum(unalloc_in) ≈ sum(shared_out) + sum(remaining) + sum(drops)`
      até `ε=1e-6` para os 3 tipos de regra rodando juntos.
      *(`TestIntegration_FullInvariant`)*
- [x] Reprocessar `allocate-shared` produz mesmo output (engine
      determinístico); `--reset` faz `ALTER DELETE` do shared do
      período antes de inserir, evitando órfãs.
      *(`TestIntegration_ReprocessIsIdempotent`, `TestResetShared_IssuesExec`)*
- [x] Linha sem regra aplicável continua em `fct_unallocated_cost`
      (engine devolve em `RemainingUnalloc`).
      *(`TestApply_NoMatchGoesRemaining`)*
- [x] Dado regra com `valid_from`/`valid_to`, regra é ignorada fora
      do intervalo.
      *(`TestIntegration_RuleOutsideValidityWindow`, `TestCoversPeriod`)*

## Riscos / incerteza

- **Regras crescem exponencialmente.** Precisa de UX (mesmo que CLI
  primeiro) para listar/inspecionar regras aplicáveis a uma linha.
- **Decisão Kind vs YAML.** Modelar regras no grafo dá histórico
  bitemporal grátis mas custa ergonomia. Decisão pendente — prompt 02.
- **Discount allocations.** Savings Plans creditam linhas; rateio
  inverso (subtrair) precisa ser explícito.
- **Audit trail.** Toda linha "shared" precisa apontar para a regra
  que a originou (já no schema).

## Notas de implementação

- Pacote em `internal/modules/cost/shared/`.
- Config em `config/shared_rules/*.yaml`, hot-reloaded por execução
  (não em runtime).
- Lock por `(period, allocation_type=shared)` para evitar concorrência.

## Decisões (implementação)

- **D1 — Regras como YAML, não como Kind.**
  Registrada em [ADR-003](../architecture/decisions/ADR-003-allocation-rules-as-yaml.md).
  Audit via `git log` + `rule_id`/`rule_version` por linha em
  `fct_cost_by_urn`.

- **D2 — Schema CH estendido (não tabela nova).**
  `fct_cost_by_urn` ganhou `allocation_type` (`direct|shared`),
  `rule_id`, `rule_version`. `ORDER BY` estende a chave com
  `allocation_type` → direct e shared coexistem sem colidir no RMT.
  Migração aditiva: `ADD COLUMN IF NOT EXISTS` + `MODIFY ORDER BY`.

- **D3 — 3 tipos de regra no MVP.**
  `proportional_to_allocated` (rateia pela dimensão), `by_destination`
  (rateia pela região destino), `static_override` (frações fixas em
  URNs nomeadas). Cobrem ~80% dos casos reais de FinOps; demais ficam
  para iterações.

- **D4 — Apply consome `direct` para denominador, não `shared`.**
  Evita recursão e instabilidade quando regras mudam: rateios
  proporcionais sempre usam o `direct` como base estável. Conseqüência:
  reordenar regras não afeta resultado individual de cada uma.

- **D5 — Não-overlap por id ordenado.**
  Se duas regras matchariam a mesma linha, a primeira por `id`
  ordenado consome. Em vez de erro fatal, opta-se por comportamento
  determinístico e bem documentado: defeito de configuração detectado
  no operador, não no allocator.

- **D6 — Pool vazio = Drop, não fatal.**
  `proportional_to_allocated` com pool zero (dimensão sem nenhum
  direct) emite um `Drop` para diagnóstico (CLI imprime em stderr).
  Não vira shared, não vira remaining — registra a perda. Operador
  decide se cria regra fallback.

- **D7 — `id` como slug imutável.**
  `^[a-z0-9]+(-[a-z0-9]+)*$`. Renomear quebra audit trail em
  `fct_cost_by_urn.rule_id` — restritivo de propósito.

- **D8 — `--reset` para limpar órfãs.**
  Shared rows convivem como versões mais antigas no RMT até OPTIMIZE.
  Se uma regra é removida e a URN antiga não recebe mais shared,
  `OPTIMIZE` não limpa; `ALTER DELETE WHERE billing_period AND
  allocation_type='shared'` antes de reescrever resolve. Opt-in para
  caller assumir o custo de mutation pesada.

## Verificação

```sh
# Unit (parser/engine/sink/cli) — todos os pacotes da feature
go test ./internal/modules/cost/shared/... ./cmd/cli/...

# Integration (invariante TotalCUR ≈ Allocated + Shared + Remaining + Drops)
go test -run TestIntegration_ ./internal/modules/cost/shared/...

# Smoke CLI
ce allocate-shared --period=2026-03 --clickhouse=tcp://... \
  --rules=config/shared_rules --dry-run
```

**Resultado esperado:**
- `TestApply_*` — 3 tipos de regra funcionam isoladamente; expired ignored; first-rule-wins.
- `TestIntegration_FullInvariant` — `in ≈ shared + remaining + drops` com 3 regras juntas.
- `TestIntegration_ReprocessIsIdempotent` — mesmo input ⇒ mesmo output.
- `TestRunAllocateShared_*` — handler CLI valida flags e short-circuita em rules vazio.

## Entregue em

- **S-001..S-007** (slicing interno): schema migration + types,
  rule loader YAML, engine de rateio, sink+idempotência, CLI
  `ce allocate-shared`, integration test, docs + priorities.

## Impacto na modelagem

**Classe:** No impact (grafo) + Schema CH aditivo.

**Decisão registrada em [ADR-003](../architecture/decisions/ADR-003-allocation-rules-as-yaml.md):**
regras vivem como YAML versionado em git, **não** como Kind no grafo.
Audit trail via `rule_id`/`rule_version` por linha em
`fct_cost_by_urn` + `git log`.

**Mudanças propostas:**
- `architecture/01-modeling.md`: **nenhuma** (sem novos Kinds, sem novas edges).
- `modules/cost.md`: descrever pacote `shared/` quando o módulo tiver doc.
- Schema CH (`internal/repository/clickhouse/schema.go`):
  - `ALTER TABLE fct_cost_by_urn ADD COLUMN allocation_type LowCardinality(String) DEFAULT 'direct'`
  - `ALTER TABLE fct_cost_by_urn ADD COLUMN rule_id String DEFAULT ''`
  - `ALTER TABLE fct_cost_by_urn ADD COLUMN rule_version UInt32 DEFAULT 0`
  - `ALTER TABLE fct_cost_by_urn MODIFY ORDER BY (urn, billing_period, dimension, dimension_value, allocation_type)`
    *(nova chave contém a antiga — ClickHouse aceita; dedup direct vs shared não colide)*.
  - F-005 sink passa a escrever `allocation_type='direct'` explicitamente.
- ADR-003: status `proposed` → `accepted` ao concluir F-006 S-006.
