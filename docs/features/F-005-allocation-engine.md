---
id: F-005
title: Allocation Engine (URN × period × dimension)
status: done
modules: [cost, bridge]
depends_on: [F-003, F-004]
modeling_impact: no
adrs: []
epic: E-002
updated: 2026-05-14
---

# F-005 — Allocation Engine

## Problema

A fact table de CUR (F-004) tem custo por `resource_id`, mas o usuário
final pergunta: "quanto custou o serviço X em março?", "qual o custo
do time Y por capability?". Falta um motor que percorra `(linha CUR)
→ URN → grafo` e produza custo agregado por URN × período × dimensão
(team, capability, service, etc.), sem perder a auditabilidade
(drill-down até a linha CUR).

**Para quem:** produto Arquitetura, produto Dashboard, agente — todos
consomem `cost(urn, period, dimension)`.

**Dor sem ela:** custo é fact table fria, sem correspondência com a
realidade modelada no grafo.

## Escopo

**Inclui:**
- Job `cost-allocate` que lê `fct_cur_lines FINAL` de um período,
  resolve `resource_id → URN` via F-003 (`AsOf = period_end`), e popula
  `fct_cost_by_urn (urn, billing_period, dimension, dimension_value,
  amount, lineage_count)`.
- Dimensões MVP: `service`, `account`, `region` — derivadas direto da
  linha CUR (sem dependência de org plane). `team`/`capability`
  entram via plugin quando F-010/F-011 estiverem online.
- Lineage *aggregate*: cada linha de `fct_cost_by_urn` carrega
  `lineage_count` (#linhas CUR somadas). Drill-down até a linha CUR
  individual é via ad-hoc query em `fct_cur_lines` (mesma chave
  `(urn, billing_period)`), não materializado.
- Reprocessamento: `ReplacingMergeTree(allocated_at)` colapsa
  automaticamente — re-rodar `ce allocate` sobrescreve a alocação
  anterior por `(urn, billing_period, dimension, dimension_value)`.
- Tabela `fct_unallocated_cost` para linhas sem URN, segregadas por
  `reason ∈ {no_resource_id, not_found, ambiguous}` — input para F-006.

**NÃO inclui:**
- Alocação de shared cost / fee (F-006).
- Forecast / previsão.
- API pública (F-014 vai ler).
- Per-row lineage (decisão: ad-hoc query é suficiente até F-006).

**Precondições:**
- F-003 (URN bridge) com `AsOf` funcionando.
- F-004 (`fct_cur_lines` populada).
- Grafo populado com pelo menos `Compute/Persistence/Network` (allocator
  é tolerante a grafo vazio — vai tudo para `not_found`).

## Toque no grafo

- **Lê:** `NodeRepository.GetByExternalID` em batch via
  `bridge.Resolver.ResolveURNBatch`. Tudo com `AsOf(period_end)`.
- **Escreve:** nada no grafo. Escreve em ClickHouse
  `fct_cost_by_urn` e `fct_unallocated_cost`.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** consulta o grafo "como estava no fim do período"
  para evitar contaminação com mudanças posteriores.

## Critérios de aceite

- [x] Dado período com `fct_cur_lines` populada e grafo povoado,
      quando `ce allocate --period=2026-03 --clickhouse=... --neo4j=...`,
      então `fct_cost_by_urn` contém uma linha por
      `(urn, dimension, dimension_value)` com soma correta.
- [x] Invariante macro:
      `sum(fct_cost_by_urn.amount WHERE dim='service') +
      sum(fct_unallocated_cost.amount) ≈ sum(fct_cur_lines.effective_cost FINAL)`
      com delta ≤ 1e-6 (validado em integration test S-008).
- [x] Dado `cost(urn=X, period=2026-03, dimension=service)`, então
      drill-down até as linhas CUR é via query ad-hoc por
      `(account_id, resource_id, billing_period_start)` em
      `fct_cur_lines` — `lineage_count` no rollup dá ordem de grandeza.
- [x] Dado reprocessamento (`ce allocate` 2× no mesmo período), então
      `ReplacingMergeTree(allocated_at) FINAL` colapsa para uma linha
      por chave (verificado em integration test).
- [x] Linhas CUR sem URN resolvido vão para `fct_unallocated_cost`
      com `reason` classificada (`no_resource_id`, `not_found`,
      `ambiguous`).

## Riscos / incerteza

- **Grafo incompleto no MVP.** Sem org plane (F-010/F-011), dimensões
  `team`/`capability` não existem. **Resolvido:** MVP só expõe
  `service`/`account`/`region` (derivadas da linha CUR), sem traversal
  no grafo.
- **Performance.** Alocar um mês pode envolver milhões de linhas.
  **Resolvido:** agregação in-memory por `(account, resource_id)`
  antes do resolver (cardinalidade típica ~10k–100k recursos vs ~10M
  linhas CUR); batch-resolve via F-003.
- **Reprocessamento concorrente.** Job rodando enquanto CUR é
  re-ingerido. **Decisão (D5):** sem lock para MVP. ReplacingMergeTree
  garante última-leitura-vence; operação manual aceita.
- **Memória in-memory.** ~1M tuplas distintas usam ~150MB. Para
  tenants maiores, estratégia futura: shardar por account ou
  push-down (`GROUP BY` no ClickHouse).

## Notas de implementação

- Implementação em `internal/modules/cost/allocate/`.
- Pipeline em 4 etapas:
  1. **Source** — `ClickHouseSource.Stream(period)` lê `fct_cur_lines
     FINAL` via cursor (6 colunas só — economiza IO).
  2. **Aggregator** — `Aggregator.Collect` agrupa in-memory por
     `(account, resource_id, region, service)` e separa linhas sem
     `resource_id` em bucket `Unalloc` (reason=`no_resource_id`).
  3. **Resolve** — `Resolve` chama `bridge.Resolver.ResolveURNBatch`
     por account; `AsOf = AsOfForPeriod(period)` (último ms do mês).
  4. **Project** — `Project` aplica os `DimensionResolver`s e emite
     `Row` (alocado) ou `UnallocatedRow` (NotFound/Ambiguous).
  5. **Sink** — `Sink.Write` batch-inserta as duas tabelas.
- `DimensionResolver` é plugin-style: `Dimension() + Resolve(key, urn) string`.
  MVP: `service/account/region` (info da própria linha CUR). Futuros
  resolvers (team/capability) usarão `urn` para traversal no grafo.
- Driver ClickHouse: `clickhouse-go/v2` (mesmo do F-004).

## Decisões registradas (2026-05-14)

| ID | Decisão | Razão |
|---|---|---|
| D1 | Schema `fct_cost_by_urn` usa `ReplacingMergeTree(allocated_at)` com chave `(urn, billing_period, dimension, dimension_value)` | Idempotência mesmo padrão de F-004; reprocessar substitui sem delete |
| D2 | Sem lineage per-row (só `lineage_count` agregado) | Per-row dobraria volume da fact table; drill-down ad-hoc em `fct_cur_lines` resolve até F-006 |
| D3 | MVP só `service/account/region` — todas derivadas da linha CUR | Independe de org plane (F-010/F-011); contrato `DimensionResolver` mantém porta aberta |
| D4 | `fct_unallocated_cost` separada com `reason` classificado | Feed limpo para F-006; auditoria explícita ("por que esse custo não foi alocado?") |
| D5 | Sem lock cross-process no MVP | Operação manual; RMT(allocated_at) protege idempotência. Lock entra se concorrência virar problema real |
| D6 | `AsOf = último ms do BillingPeriod` | Recursos decomissionados no mês mantêm URN; recursos criados depois do mês não vazam |
| D7 | Agregação in-memory (sem GROUP BY no ClickHouse) | ~150MB para 1M tuplas — cabe; mantém allocator stateless e fácil de testar. Push-down vira escape hatch quando precisar |

## Stories

### S-001 — Scaffold `allocate` + tipos ✅
**Comportamento:** Dado um caller que importa `internal/modules/cost/allocate`, quando referencia `allocate.Row`, `allocate.UnallocatedRow`, `allocate.Dimension`, `allocate.Report`, então compila com tipos estáveis. `doc.go` documenta o pipeline e imports permitidos.
**Camadas tocadas:** [modules/cost/allocate]
**Notas:** Define os 3 valores de `Dimension` (MVP) e os 3 `UnallocatedReason`.

### S-002 — ClickHouse schema `fct_cost_by_urn` + `fct_unallocated_cost` ✅
**Comportamento:** Dado `Client.Migrate(ctx)`, então cria ambas as tabelas idempotentemente (`IF NOT EXISTS`). `fct_cost_by_urn` usa `ReplacingMergeTree(allocated_at)` particionada por `toYYYYMM(billing_period)`; `fct_unallocated_cost` análogo. SQLs de INSERT exportados como constantes para o sink.
**Camadas tocadas:** [repository/clickhouse]
**Blocked by:** [S-001]
**Notas:** `SelectCURLinesForAllocateSQL`, `SumCostByURNSQL`, `SumUnallocatedSQL` em `query.go` para o engine e integration tests.

### S-003 — Aggregator (stream + group by) ✅
**Comportamento:** Dado uma `LineSource` que streama `RawLine`, quando `Aggregator.Collect(ctx, src, period)`, então agrupa por `(account, resource_id, region, service)` em `Buckets` (com Amount/LineageCount/Currency) e separa linhas sem `resource_id` em `Unalloc` por `(account, service)`. Marca `Currency="MIXED"` se um bucket somar moedas diferentes.
**Camadas tocadas:** [modules/cost/allocate]
**Blocked by:** [S-002]
**Notas:** `ClickHouseSource` em arquivo separado (`source_clickhouse.go`) consome `SelectCURLinesForAllocateSQL`.

### S-004 — URN resolution batch via `bridge` ✅
**Comportamento:** Dado `aggregator.ResourceIDsByAccount()`, quando `Resolve(ctx, resolver, provider, ids, period)`, então retorna `Resolution{URNByResource, Failures}`. `AsOfForPeriod(period)` deriva o `AsOf` para o resolver. Erros sentinela do `bridge` (`ErrNotFound`, `ErrAmbiguous`) viram `UnallocatedReason`; erro fatal aborta com wrap.
**Camadas tocadas:** [modules/cost/allocate, modules/bridge]
**Blocked by:** [S-003]
**Notas:** Um batch por account — preserva semântica do resolver F-003 (estrito-com-fallback-wildcard).

### S-005 — DimensionResolvers + Project ✅
**Comportamento:**
- Dado `DefaultResolvers()`, então retorna `[service, account, region]` na ordem.
- Dado `Project(agg, resolution, dims, period, allocAt)`, então emite `[]Row` e `[]UnallocatedRow` agregados por chave de saída. Bucket sem URN vira UnallocatedRow com reason vindo de `Resolution.Failures` (default `not_found`); bucket Unalloc vira UnallocatedRow com `reason=no_resource_id`.
- Soma por dimensão dentro de Rows == TotalAllocated (testado).
**Camadas tocadas:** [modules/cost/allocate]
**Blocked by:** [S-004]
**Notas:** `ResolversFor(requested)` permite o CLI filtrar dimensões.

### S-006 — Sink + Engine orchestrator ✅
**Comportamento:**
- Dado `Sink.Write(ctx, rows, unalloc)`, então insere em lotes (`BatchSize`, default 5000) via `InsertCostByURNSQL` e `InsertUnallocatedSQL`. Tabelas vazias → no-op.
- Dado `Engine.Run(ctx, period)`, então encadeia Source → Aggregator → Resolve → Project → Sink e devolve `Report{CURLinesRead, UniqueResIDs, Allocated, Unallocated, TotalCUR, TotalAllocated, TotalUnallocated, Duration}`.
- `DryRun=true` calcula tudo mas não escreve.
**Camadas tocadas:** [modules/cost/allocate]
**Blocked by:** [S-005]
**Notas:** `Batcher` interface (subset de `driver.Conn`) replica padrão do sink F-004 para mock fácil.

### S-007 — CLI `ce allocate` ✅
**Comportamento:** Dado `ce allocate --period=YYYY-MM --clickhouse=... [--neo4j=...] [--dimensions=service,account,region] [--batch-size=N] [--dry-run]`, então conecta CH (e Neo4j se passado), roda Engine, imprime Report (CUR lines, unique res_ids, allocated/unallocated com totais e delta do invariante). Sem `--neo4j` usa `repository/memory` vazio (TUDO vira `not_found` — útil para debug do pipeline). Exit 5 (errPartial) se invariante violada (delta > 1¢).
**Camadas tocadas:** [cmd/cli]
**Blocked by:** [S-006]
**Notas:** Usage atualizado em `cmd/cli/main.go`. `parseDimensions` valida set fechado (MVP).

### S-008 — Integration test: invariante TotalCUR ≈ Allocated + Unallocated ✅
**Comportamento:** Build tag `integration`, env `CLICKHOUSE_TEST_URI`. Seed `fct_cur_lines` com 6 linhas curadas (4 resolvíveis + 1 NotFound + 1 sem resource_id); roda `Engine.Run`. Asserta:
- `TotalCUR == sum(seed.cost)` (1e-9).
- `TotalCUR ≈ TotalAllocated + TotalUnallocated` (1e-6).
- Read-back: `sum(fct_cost_by_urn.amount WHERE dim=service)` == `TotalAllocated`; `sum(fct_unallocated_cost.amount)` == `TotalUnallocated`.
- Idempotência: rodar 2× resulta em `count(fct_cost_by_urn FINAL) = #URN × #dims`.
**Camadas tocadas:** [modules/cost/allocate]
**Blocked by:** [S-007]
**Notas:** Usa `stubResolver` in-memory — não exige Neo4j up, só ClickHouse.

## Verificação

Os ACs são exercitados em três camadas:

- **Unidade:** `internal/modules/cost/allocate/{aggregator,resolver,project,sink,dimension}_test.go` —
  cobertura de agregação (group by, mixed currency, unique IDs), resolução
  (hits/miss/ambiguous, AsOf, erro fatal), projeção (invariante por dimensão,
  no-resource-id bucket), sink (batch mock), parse de dimensões no CLI.
- **CLI:** `cmd/cli/allocate_test.go` — validação de flags (period
  obrigatório, formato, clickhouse obrigatório, dimensões válidas).
- **Integration (build tag `integration`, env `CLICKHOUSE_TEST_URI`):**
  `internal/modules/cost/allocate/integration_test.go` cobre:
  - invariante macro (`TotalCUR ≈ Allocated + Unallocated`, delta < 1e-6),
  - read-back via `SumCostByURNSQL` / `SumUnallocatedSQL`,
  - idempotência: 2 runs → 1 linha por `(URN, dimension, dim_value)` FINAL.

Para rodar:

```sh
docker run --rm -p 9000:9000 clickhouse/clickhouse-server
export CLICKHOUSE_TEST_URI=localhost:9000
go test -tags=integration ./internal/modules/cost/allocate/...
```

Sem `CLICKHOUSE_TEST_URI`, os testes skipam.
