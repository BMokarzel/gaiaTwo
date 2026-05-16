# Plano de Implementação — CostEngine

> Plano de execução ordenado para construir o CostEngine, do estado atual (esqueleto de `node`/`edge`) até a plataforma completa descrita em `modelagem.md` e `arquitetura-custos.md`.
>
> Cada fase entrega valor observável e desbloqueia a próxima. Sem estimativas de tempo — só ordem, dependências e DoD (Definition of Done).

---

## Estado atual

✅ `internal/entity/node` — interfaces + structs de infra (Provider, Account, Region, Environment, Compute, Persistence, Messaging, Network)
✅ `internal/entity/edge` — interfaces + 8 tipos canônicos + matriz de adjacência + ID determinístico
✅ Documentos de modelagem (`modelagem.md`, `arquitetura-custos.md`)
⬜ Tudo o mais (repos, coletores, custo, API, simulação)

---

## Visão de fases

```
Fase 0 ─ Foundations (testes, hardening, ExternalID)
  │
  ├─▶ Fase 1 ─ Repositório de grafo (n4j)
  │           │
  │           ├─▶ Fase 2 ─ Coletor AWS (Discoverer)
  │           │
  │           └─▶ Fase 3 ─ URN Resolver (bridge)
  │
  └─▶ Fase 4 ─ Domínio de custo (entity/cost)
              │
              ├─▶ Fase 5 ─ CUR Ingestor (bronze → silver)
              │
              ├─▶ Fase 6 ─ Allocation engine
              │
              ├─▶ Fase 7 ─ Gold rollups
              │
              ├─▶ Fase 8 ─ Query API (HTTP/GraphQL)
              │
              └─▶ Fase 9 ─ Simulation engine + Price catalog
                          │
                          └─▶ Fase 10 ─ Observabilidade, governança, hardening
```

---

## Fase 0 — Foundations

**Objetivo:** consolidar a base atual para que tudo daqui pra frente tenha contrato estável e teste.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 0.1 | Adicionar `ExternalID()` na interface `Resource` | `internal/entity/node/node.go` | Interface atualizada |
| 0.2 | Campo `ExternalID string` + getter em `Compute`, `Persistence`, `Messaging`, `Network` | respectivos arquivos | `go build ./...` limpo |
| 0.3 | Construtores `NewURN(provider, account, kind, id)` e parser `ParseURN(s) (parts, error)` | `internal/entity/node/urn.go` | Tests cobrem URN válida/inválida/edge cases |
| 0.4 | Testes unitários do pacote `node` (Meta bitemporal, IsCurrent, URN) | `internal/entity/node/*_test.go` | Coverage ≥ 80% |
| 0.5 | Testes do pacote `edge` (DeterministicID idempotência, Validate matriz) | `internal/entity/edge/*_test.go` | Coverage ≥ 80% |
| 0.6 | Linter + CI (golangci-lint, gofumpt, govulncheck) | `.github/workflows/ci.yml`, `.golangci.yml` | CI verde em PRs |
| 0.7 | `Makefile` com targets `build`, `test`, `lint`, `cover`, `run` | raiz | `make test` funciona local |

**Saída:** base estável, testada, com CI.

---

## Fase 1 — Repositório de grafo

**Objetivo:** persistir nodes e edges; consultas básicas de current view e histórico.

### Decisões a fechar antes
- Driver de grafo: **Neo4j** (mais maduro, Cypher) **ou** **JanusGraph** (open-core, escala horizontal). Recomendação inicial: **Neo4j** — DX superior, suficiente até dezenas de milhões de nós.
- Convenção de label/property mapping entre Go structs e Cypher.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 1.1 | Interface `NodeRepository` (`Upsert`, `GetByURN`, `ListByKind`, `History`) | `internal/repository/graph.go` | Interface compilando |
| 1.2 | Interface `EdgeRepository` (`Upsert`, `Between`, `Neighbors`, `TraverseFromURN`) | `internal/repository/graph.go` | Idem |
| 1.3 | Implementação Neo4j (`n4j`) | `internal/repository/n4j/` | Suite de integração com Testcontainers passando |
| 1.4 | Mapper `node ↔ cypher` (preserva Meta, Tags, Properties) | `internal/repository/n4j/mapper.go` | Roundtrip test: struct → cypher → struct igual |
| 1.5 | Suporte bitemporal: upsert fecha versão anterior (`validTo = now`) e cria nova | `internal/repository/n4j/upsert.go` | Test: 3 versões da mesma URN; query historica funciona |
| 1.6 | Index secundário `(provider, account, external_id) → urn` | migrations Cypher | Lookup O(1) por external_id |
| 1.7 | Validação de edge usa `edge.Validate` antes de persistir | `internal/repository/n4j/edges.go` | Edge inválida → erro tipado |

**Saída:** grafo persistido, queryável, com histórico.

---

## Fase 2 — Coletor AWS (Discoverer)

**Objetivo:** descobrir infra AWS real e popular o grafo.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 2.1 | Interface `Discoverer` genérica (`Discover(ctx) <-chan node.Node`) | `internal/collector/discoverer.go` | Interface compilando |
| 2.2 | Discoverer AWS EC2 (instances, volumes, ENIs) | `internal/collector/aws/ec2.go` | Run real produz Compute+Persistence+Network nodes |
| 2.3 | Discoverer AWS RDS, Lambda, S3, SQS/SNS, Kinesis | `internal/collector/aws/{rds,lambda,s3,messaging}.go` | Cada serviço emite nodes corretamente |
| 2.4 | Discoverer AWS VPC (vpcs, subnets, peering, gateways, LBs) | `internal/collector/aws/vpc.go` | Edges `Contains`, `Routes`, `Peers` emitidas |
| 2.5 | Sink: pipeline `Discoverer → Validator → NodeRepository` | `internal/collector/sink.go` | Run produz delta versionado no grafo |
| 2.6 | Scheduler: cron/periódico por conta/região | `internal/collector/scheduler.go` | Run agendado; idempotente |
| 2.7 | Edges inferidas pós-discovery (ex.: ENI→Compute) | `internal/collector/infer/network.go` | Edges criadas com `confidence < 1` e lineage |

**Saída:** grafo vivo refletindo AWS real.

---

## Fase 3 — URN Resolver

**Objetivo:** dado um `(provider, account, externalID)` qualquer, retornar `node.URN` em O(1) — habilita o CUR ingestor.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 3.1 | Interface `Resolver` (`Resolve(externalID) (URN, error)`) | `internal/resolver/resolver.go` | Interface |
| 3.2 | Implementação backed-by-graph (lookup via index secundário) | `internal/resolver/graph.go` | Tests com fixture |
| 3.3 | Cache em memória (LRU + TTL) | `internal/resolver/cache.go` | Hit rate métricado |
| 3.4 | "Unresolved bin" — externalIDs sem match vão para fila de retry | `internal/resolver/unresolved.go` | Late-arriving infra é reprocessada |

**Saída:** bridge CUR ↔ grafo pronto.

---

## Fase 4 — Domínio de custo (`entity/cost`)

**Objetivo:** tipos canônicos da camada econômica. Sem persistência ainda.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 4.1 | `CostLine` (1:1 com schema silver) | `internal/entity/cost/line.go` | Struct + JSON tags |
| 4.2 | `CostProjection` (resultado de agregação) | `internal/entity/cost/projection.go` | Struct + helpers |
| 4.3 | `ChargeCategory`, `PricingModel` enums | `internal/entity/cost/enums.go` | Constantes FOCUS-aligned |
| 4.4 | `Period` + helpers (`Hour`, `Day`, `Month`) | `internal/entity/cost/period.go` | Tests |
| 4.5 | Interface `CURImporter` | `internal/entity/cost/importer.go` | Interface |
| 4.6 | `AllocationRule` + `LineMatcher` | `internal/entity/cost/allocation.go` | Struct + parser YAML |
| 4.7 | `PriceCatalog` (interface + memory impl) | `internal/entity/cost/catalog.go` | Lookup por (service, region, sku) |

**Saída:** vocabulário tipado para custo.

---

## Fase 5 — CUR Ingestor (bronze → silver)

**Objetivo:** ler CUR da AWS, normalizar para FOCUS, vincular URN, escrever silver.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 5.1 | S3 client + listing de manifest CUR | `internal/repository/aws/cur/manifest.go` | Detecta novo `manifest.json` |
| 5.2 | Parser de manifest (colunas, paths Parquet, assemblyId) | `internal/repository/aws/cur/parse.go` | Manifest real desserializa |
| 5.3 | Reader Parquet streaming (`<-chan rawLine`) | `internal/repository/aws/cur/reader.go` | Memória O(1); processa Parquet grandes |
| 5.4 | Normalizer CUR → `CostLine` (incluindo `effective_cost`) | `internal/repository/aws/cur/normalize.go` | Tests com fixtures CUR sintético |
| 5.5 | Pipeline: `Read → Resolve URN → Normalize → Write Silver` | `internal/cost/ingest/pipeline.go` | E2E: arquivo CUR → silver populado |
| 5.6 | Silver writer: Iceberg ou Parquet particionado | `internal/repository/lake/silver.go` | Partition swap idempotente por `(billing_period, run_id)` |
| 5.7 | Detecção de "late-arriving infra" — URN vazia gera evento | `internal/cost/ingest/unresolved.go` | Reprocessamento dispara após novo discovery |
| 5.8 | Trigger event-driven (S3 EventBridge → ingestor) | `internal/cost/ingest/trigger.go` | Manifest novo dispara run automático |

**Saída:** silver populado e consultável via Trino/DuckDB/Athena.

---

## Fase 6 — Allocation engine

**Objetivo:** atribuir custos órfãos (sem `resource_id`) via regras declarativas.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 6.1 | Parser de regras (YAML → `[]AllocationRule`) | `internal/cost/alloc/parser.go` | Regras válidas/inválidas testadas |
| 6.2 | Engine: dado lote de linhas, aplicar regras, gerar derivadas | `internal/cost/alloc/engine.go` | Tests cobrem 4 strategies |
| 6.3 | Estratégias: `proportional`, `even_split`, `fixed`, `tag_override` | `internal/cost/alloc/strategy/*.go` | Cada strategy isolada |
| 6.4 | Linhas alocadas têm `lineage.rule = "allocator/<id>"` e `confidence < 1` | embutido | Audit query consegue rastrear |
| 6.5 | Job de re-alocação (quando regra muda) | `internal/cost/alloc/rerun.go` | Idempotente; só toca partições afetadas |
| 6.6 | UI / CLI mínima para listar/validar regras | `cmd/ce alloc list|validate` | Comandos funcionais |

**Saída:** 100% do gasto atribuído (com confidence rastreável).

---

## Fase 7 — Gold rollups

**Objetivo:** materializar agregações comuns para consulta sub-segundo.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 7.1 | Schema ClickHouse (ou Timescale): `cost_by_urn_hour`, `cost_by_service_day`, `cost_by_account_day`, `network_egress_hour`, `unattributed_day` | `migrations/olap/*.sql` | Tabelas criadas |
| 7.2 | Job incremental silver → gold (watermark por `billing_period`) | `internal/cost/rollup/runner.go` | Run idempotente; só processa partições novas |
| 7.3 | Repositório gold (interface `RollupRepository`) | `internal/repository/olap/` | Query por dimensão funciona |
| 7.4 | Backfill: replay completo da silver para reconstruir gold | `cmd/ce rollup backfill` | Comando executa do zero |
| 7.5 | Métricas: lag silver→gold, contagem de partições processadas | observabilidade | Dashboards Grafana |

**Saída:** rollups consultáveis em ms.

---

## Fase 8 — Query API

**Objetivo:** expor cálculos via API estável; ponte para UI/automação.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 8.1 | Servidor HTTP (chi/echo) com auth básica | `cmd/ce-api/main.go` | Healthcheck OK |
| 8.2 | Endpoint `GET /nodes/:urn` | `internal/controller/node.go` | Retorna Node JSON |
| 8.3 | Endpoint `GET /nodes/:urn/neighbors?type=...&depth=N` | idem | Traversal funciona |
| 8.4 | Endpoint `GET /cost?subject=<urn>&period=<>&groupBy=<>` | `internal/controller/cost.go` | Retorna `CostProjection` |
| 8.5 | Endpoint `GET /cost/top?dim=service&period=<>&limit=N` | idem | Top-N por dimensão |
| 8.6 | Endpoint `GET /cost/movers?period=<>&baseline=<>` | idem | WoW/MoM deltas |
| 8.7 | (Opcional) GraphQL gateway unificando node + cost | `internal/controller/graphql/` | Schema introspectável |
| 8.8 | OpenAPI 3 spec gerada + Postman collection | `docs/openapi.yaml` | Spec versionada |

**Saída:** plataforma usável via API.

---

## Fase 9 — Simulation engine

**Objetivo:** permitir what-if e otimizações.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 9.1 | `PriceCatalog` populado a partir da AWS Price List API | `internal/cost/catalog/aws_loader.go` | Catálogo atualizado periodicamente |
| 9.2 | Simulador `RegionMigration` | `internal/cost/sim/region.go` | Input: URN + target region → Δ cost |
| 9.3 | Simulador `InstanceResize` | `internal/cost/sim/resize.go` | Input: URN + target type → Δ cost |
| 9.4 | Simulador `RICoverage` (otimização) | `internal/cost/sim/ri.go` | Sugere commitment com max saving |
| 9.5 | Simulador `EgressAvoidance` (usa grafo) | `internal/cost/sim/egress.go` | Detecta tráfego cross-AZ caro |
| 9.6 | Simulador `ShutdownWindow` | `internal/cost/sim/shutdown.go` | Janela horária → economia projetada |
| 9.7 | Endpoint `POST /simulate/<kind>` | `internal/controller/simulate.go` | Resposta inclui `lineage` da simulação |

**Saída:** plataforma de decisão, não só observação.

---

## Fase 10 — Observabilidade, governança, hardening

**Objetivo:** produção-readiness.

### Tarefas

| # | Entrega | Caminho | DoD |
|---|---------|---------|-----|
| 10.1 | Tracing OTel em todos os pacotes | `internal/observability/` | Traces visíveis em Jaeger/Tempo |
| 10.2 | Métricas Prometheus (ingest lag, cache hit, query latency) | idem | Dashboards Grafana |
| 10.3 | Audit log (toda mudança de regra/policy) | `internal/audit/` | Persistência imutável (append-only) |
| 10.4 | RBAC por organização/tenant | `internal/auth/rbac.go` | Permissões testadas |
| 10.5 | Dogfooding: CostEngine observa a si próprio | config + dashboards | Custo da plataforma visível |
| 10.6 | Documentação: arquitetura, runbooks, operações | `docs/` | Onboarding em < 1 dia |
| 10.7 | Disaster recovery: backup de silver/gold/grafo + replay test | runbooks | DR test executado com sucesso |

**Saída:** sistema mantenível, auditável, escalável.

---

## Princípios transversais (aplicam-se a todas as fases)

1. **Testes antes de avançar de fase.** Sem CI verde, não há promoção.
2. **Documentar decisões em ADRs.** `docs/adr/0001-graph-engine.md`, etc.
3. **Sem feature flags permanentes.** Toggle só durante migração; remover depois.
4. **Bitemporal sempre.** Toda mudança preserva história — nunca update destrutivo.
5. **Lineage obrigatório.** Toda inferência/agregação carrega `Lineage`.
6. **Idempotência por design.** Coletores e ingestores rerunáveis sem efeito colateral.
7. **Dogfooding.** Cada nova feature da plataforma é exercitada nela mesma.

---

## Definition of Done — geral

Uma tarefa só é "feita" quando:

- [ ] Código compilando sem warnings
- [ ] Testes unitários cobrindo casos felizes e edge cases (≥ 80%)
- [ ] Testes de integração (quando aplicável) com containers reais
- [ ] Lint verde (`golangci-lint`)
- [ ] Sem `TODO` no diff (ou referenciando issue tracker)
- [ ] Documentação atualizada (godoc + docs/ se aplicável)
- [ ] ADR registrado se a decisão for arquiteturalmente relevante
- [ ] Métricas e logs estruturados emitidos
- [ ] Review aprovada

---

## Próximo passo concreto

Começar pela **Fase 0.1**: adicionar `ExternalID()` na interface `Resource` em `internal/entity/node/node.go` e propagar para os 4 structs (`Compute`, `Persistence`, `Messaging`, `Network`).

É a menor mudança que desbloqueia toda a Fase 5 (CUR ingestor) — bridge entre infra e custo. ~30 LOC.
