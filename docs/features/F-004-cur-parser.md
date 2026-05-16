---
id: F-004
title: CUR Parser (Parquet → fact table)
status: done
modules: [cost]
depends_on: [F-003]
modeling_impact: no
adrs: []
epic: E-002
updated: 2026-05-14
---

# F-004 — CUR Parser

## Problema

O AWS Cost and Usage Report (CUR) é a fonte autoritativa de custo. Vem
em Parquet, particionado por mês, com schema largo (300+ colunas). Sem
um parser que normalize as linhas em uma fact table consultável, o
custo não tem como entrar no grafo nem no allocation engine.

**Para quem:** allocation engine (F-005) e operador que precisa drilldown
de custo até a linha CUR.

**Dor sem ela:** custo só existe no S3 da AWS, fora de qualquer query
do CostEngine.

## Escopo

**Inclui:**
- Leitura de CUR Parquet de bucket S3 (caminho configurável).
- Parser que normaliza as colunas essenciais: `line_item_usage_account_id`,
  `line_item_resource_id`, `line_item_usage_start_date`,
  `line_item_unblended_cost`, `product_code`, `product_servicecode`,
  `line_item_line_item_type`, `pricing_term`.
- Escrita em ClickHouse fact table `fct_cur_lines` particionada por
  `(billing_period, line_item_type)`.
- Idempotência por `(report_id, line_item_id)`.
- Comando CLI `ce ingest cur --report-path=s3://.../cur/...`.

**NÃO inclui:**
- Resolução `resource_id → URN` (vive em F-003).
- Allocation (F-005).
- CUR 2.0 / Focus 1.0 (parser próprio quando virar prioridade).
- Custo "shared" sem resource_id (F-006).

**Precondições:**
- ClickHouse provisionado (já em `06-system.md`).
- F-003 disponível para etapas downstream (parser não chama; só registra).
- Credencial S3 com leitura do bucket do CUR.

## Toque no grafo

- **Lê:** nada (parser não consulta grafo).
- **Escreve:** nada no grafo. **Fact table** em ClickHouse,
  não Neo4j.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** não aplica diretamente — fact table tem
  `billing_period_start/end` (tempo do mundo real) e `ingested_at`
  (tempo do sistema). Reprocessamento sobrescreve por `(report_id,
  line_item_id)`.

## Critérios de aceite

- [x] Dado bucket S3 com CUR de mês completo (~10M linhas), quando
      `ce ingest cur --source=s3 --bucket=... --prefix=... --report=...`,
      então `fct_cur_lines` contém todas as linhas com colunas
      essenciais preenchidas.
- [x] Dado CUR já ingerido, quando re-rodar o comando, então não há
      duplicação (idempotência por `(report_id, line_item_id,
      billing_period_start)` via `ReplacingMergeTree` + `FINAL`).
- [x] Dado CUR de mês reprocessado (AWS reemitiu), quando re-rodar,
      então linhas mudadas são atualizadas e `ingested_at` reflete
      (verificado pelo integration test).
- [x] Comando reporta totais: linhas, erros, batches, duração.
- [x] Erro em linha individual (campo inesperado) não derruba
      execução; vai para `fct_cur_errors` com motivo.

## Riscos / incerteza

- **Volume.** CUR mensal pode passar de 10GB / 100M linhas em contas
  grandes. Streaming + batch insert (~10k linhas) obrigatórios.
- **Schema dinâmico do CUR.** Colunas de tags (`resource_tags_user_*`)
  variam por conta e mês. Decisão: guardar tags como `Map(String, String)`
  no ClickHouse.
- **Datas e timezone.** CUR usa UTC; period boundaries no fim do mês.
  Validar com 1-2 amostras antes.
- **Custo de re-ingestão.** Reprocessar mês fechado precisa fechar o
  período no grafo bitemporal. Não é problema do parser — sinaliza
  para o allocation.

## Notas de implementação

- Tipos de domínio: `internal/entity/cost/` (espelha o módulo
  `internal/entity/node` em estilo).
- Pipeline: `internal/modules/cost/` com subpacotes
  `parser/` (Parquet → `CostLine`), `source/` (S3 + manifest),
  `sink/` (ClickHouse writer).
- Repositório ClickHouse: `internal/repository/clickhouse/` —
  `client.go` + `schema.go` (migrations idempotentes, padrão espelhado
  do `n4j/`).
- Biblioteca Parquet: `github.com/parquet-go/parquet-go`.
- Driver ClickHouse: `github.com/ClickHouse/clickhouse-go/v2` (oficial).
- Particionamento ClickHouse: `PARTITION BY toYYYYMM(billing_period_start)`.

## Decisões registradas (2026-05-14)

| ID | Decisão | Razão |
|---|---|---|
| D1 | MVP grava **direto em ClickHouse** (silver+gold colapsados); Iceberg/Parquet silver fica para feature separada quando FOCUS entrar | Reduz escopo do F-004; ReplacingMergeTree cobre idempotência sem precisar de partition-swap em S3. Não fecha porta pra evoluir |
| D2 | Driver: `github.com/ClickHouse/clickhouse-go/v2` | Oficial, mantido pela Altinity, suporte nativo a batch insert |
| D3 | Parquet: `github.com/parquet-go/parquet-go` | Streaming row-by-row sem trazer Arrow inteiro; per nota original do doc |
| D4 | CUR v1 (legado) apenas; CUR 2.0 / BCM Data Exports vira feature dedicada | v1 ainda é o caminho universal; v2 implica wire-up de FOCUS schema |
| D5 | Parser **não resolve URN** — guarda `resource_id` raw no fact table | F-005 (allocation) resolve via bridge F-003 na hora de agregar; mantém o parser sem dependência do grafo |
| D6 | Linhas com `resource_id` vazio entram no fact table com URN vazio (tratamento de "shared cost" fica em F-006) | Não perdemos a linha; allocation engine decide alocar ou ignorar |

## Stories

### S-001 — Scaffold `cost` (entity + módulo) + tipos de domínio ✅
**Comportamento:** Dado um caller que importa `internal/entity/cost`, quando referencia `cost.Line`, `cost.Partition`, `cost.ChargeCategory`, `cost.PricingModel`, então compila com tipos estáveis. Pacote `internal/modules/cost/` existe com `parser/`, `source/`, `sink/` vazios + `doc.go` em cada um descrevendo o papel.
**Camadas tocadas:** [entity/cost, modules/cost]
**Notas:** Story zero. `cost.Line` segue o subset da §5.3 de `02-cost.md` — campos essenciais (sem `EffectiveCost` ainda, que entra em F-005). Define também `CURImporter` interface para que F-005 já possa codar contra mock.

### S-002 — Parquet reader streaming (path local) ✅
**Comportamento:** Dado um arquivo Parquet CUR (path local), quando `parser.ReadFile(ctx, path) → <-chan cost.Line, <-chan ParseError`, então emite linhas decodificadas de forma streaming (backpressure pelo consumidor); fixture sintético de ~100 linhas gerado pelo próprio teste com `parquet-go`.
**Camadas tocadas:** [modules/cost/parser]
**Blocked by:** [S-001]
**Notas:** Sem S3 ainda — foco em garantir streaming/buffering corretos. Erros de I/O abortam; erros por linha vão no canal de erros e o stream continua. Decodifica colunas obrigatórias só (S-003 cobre mapping completo).

### S-003 — Mapeamento colunas CUR → `cost.Line` + tags dinâmicas ✅
**Comportamento:**
- Dado uma linha Parquet com as colunas-base da §2 de `02-cost.md`, quando passa pelo mapper, então `cost.Line` tem todos os campos preenchidos com tipos corretos (datas UTC, decimais como `float64` por hora, charge category derivada de `line_item_line_item_type`).
- Dado colunas `resource_tags_user_*` com cardinalidade variável, quando mapeia, então vão para `Line.Tags map[string]string` (chave = nome após prefixo).
- Dado linha com campo essencial faltando ou inválido, quando mapeia, então emite `ParseError` com motivo; não derruba o stream.
**Camadas tocadas:** [modules/cost/parser]
**Blocked by:** [S-002]
**Notas:** Cobre AC-1 + AC-5. `pricing_model` derivado de `line_item_line_item_type` (`RIFee`/`DiscountedUsage` → reserved; `SavingsPlan*` → savings_plan; demais → on_demand).

### S-004 — Cliente + schema ClickHouse + batch insert ✅
**Comportamento:**
- Dado um `Client` configurado (URI + auth), quando `Migrate(ctx)`, então cria `fct_cur_lines` (ReplacingMergeTree por `(report_id, line_item_id, billing_period_start)`, particionada por `toYYYYMM(billing_period_start)`) e `fct_cur_errors` idempotentemente.
- Dado um stream de `cost.Line`, quando `sink.WriteBatch(ctx, lines)`, então faz INSERT em lote (~10k por execução) e reporta `BatchReport{Inserted, Failed}`.
- Erros de driver são fatais; erros de validação por linha vão para `fct_cur_errors`.
**Camadas tocadas:** [repository/clickhouse, modules/cost/sink]
**Notas:** Schema migration usa o mesmo padrão de `n4j/schema.go` (slice de DDLs idempotentes + `Migrate` runner). Cobre base do AC-2 (estrutura para idempotência).

### S-005 — S3 source: manifest + listagem de parts ✅
**Comportamento:** Dado `s3://bucket/prefix/cur-name/year=2026/month=05/`, quando `source.Discover(ctx, uri) → Partition`, então retorna a `Partition` com lista de objetos Parquet (`*.snappy.parquet`) ordenados, RunID extraído do manifest, e total de bytes. `source.Open(ctx, p, key) → io.ReadCloser` faz streaming do objeto.
**Camadas tocadas:** [modules/cost/source, repository/aws]
**Blocked by:** [S-002]
**Notas:** Usa aws-sdk-go-v2 que já está em uso (config.LoadDefaultConfig). Lê o `manifest.json` do CUR para validar schema; aborta se schema incompatível com mapper.

### S-006 — Idempotência + reprocessamento ✅
**Comportamento:**
- Dado CUR já ingerido inteiro, quando re-rodar a ingestão, então `fct_cur_lines` não cresce — `ReplacingMergeTree` colapsa por `(report_id, line_item_id, billing_period_start)` mantendo o registro com maior `ingested_at`.
- Dado CUR reemitido pela AWS (mesmo mês, valores diferentes), quando re-rodar, então linhas mudadas têm `ingested_at` atualizado e a versão antiga é eventualmente recolhida pelo merge.
- Documentar que reads imediatamente após reprocessamento podem ver duplicados (ClickHouse colapsa em background) — usar `FINAL` ou `argMax(*, ingested_at)` nas queries de gold (responsabilidade de F-005).
**Camadas tocadas:** [repository/clickhouse, modules/cost/sink]
**Blocked by:** [S-004]
**Notas:** Cobre AC-2 e AC-3. Decisão de leitura (FINAL vs argMax) sai do escopo aqui — vira ADR quando F-005 começar a consumir.

### S-007 — CLI `ce ingest cur` + relatório ✅
**Comportamento:** Dado `ce ingest cur --report-path=s3://... --clickhouse-uri=tcp://... [--clickhouse-user=... --clickhouse-pass=...]`, quando executa, então wire end-to-end (manifest → parts → parquet → mapper → batch write) e ao final imprime relatório no mesmo padrão de `ce extract aws`: linhas lidas / inseridas / falharam por parsing / falharam por driver. Exit codes: 0 ok, 2 uso, 3 auth (S3/ClickHouse), 5 partial (erros parciais de parsing — análogo ao `errPartial` do extract).
**Camadas tocadas:** [cmd/cli, modules/cost]
**Blocked by:** [S-003, S-005, S-006]
**Notas:** Cobre AC-4. Reaproveita helper de exit codes de `cmd/cli/main.go`.

### S-008 — Integration tests + benchmark streaming ✅
**Comportamento:**
- Dado `CLICKHOUSE_TEST_URI` apontado para uma instância viva, quando rodar `make test-integration`, então testes E2E exercitam: migrate idempotente, batch insert, idempotência via re-run, reprocessamento com bump de `ingested_at`, escrita de linhas inválidas em `fct_cur_errors`.
- Benchmark `BenchmarkParseLargeCUR_NoOOM` gera fixture de 100k linhas Parquet (proxy de 10M com `b.N`), processa via `parser.ReadFile` + `sink.WriteBatch` (modo "dry-run" sem cliente CH real) e mede pico de heap via `runtime.MemStats`. Falha se pico > 256 MiB.
**Camadas tocadas:** [modules/cost, repository/clickhouse]
**Blocked by:** [S-007]
**Notas:** Cobre AC-1 (10M linhas via extrapolação por benchmark) e o risco de "Volume". Pattern espelha o que está em `internal/repository/n4j/integration_test.go` (build tag `integration`, skip se env ausente).

## Verificação

Os ACs são exercitados em três camadas:

- **Unidade:** `internal/modules/cost/{parser,sink,source/local,source/s3}` —
  cobertura do mapping (FOCUS, CUR v1/v2, tags), classificação charge/pricing,
  batching/flush do sink, descoberta de partitions (manifest e fallback),
  fake S3 client.
- **Throughput:** `internal/modules/cost/parser/bench_test.go` — `BenchmarkParse`
  + `TestParseRate_BudgetCheck` exige ≥50k rows/s (medido ~420k rows/s no
  baseline 2026-05-14).
- **Integration (build tag `integration`, env `CLICKHOUSE_TEST_URI`):**
  `internal/repository/clickhouse/integration_test.go` cobre:
  - migration idempotente (2 execuções → 2 tabelas),
  - insert via sink + count,
  - reprocessamento `ReplacingMergeTree` + `FINAL` (versão mais recente vence),
  - tabela `fct_cur_errors`.

Para rodar:

```sh
docker run --rm -p 9000:9000 clickhouse/clickhouse-server
export CLICKHOUSE_TEST_URI=localhost:9000
make test-integration
```

Sem `CLICKHOUSE_TEST_URI`, os testes skipam (não derrubam `make test`).
