# Prioridades correntes

> Ordem de ataque. Atualizar quando concluir feature ou mudar
> direção. Cada item tem **razão da prioridade**.
>
> Atualizado: 2026-05-14 (F-015 concluída)

## Em progresso

(nenhuma — F-015 fechada)

## Próximas (em ordem)

1. **[F-012](../features/F-012-capability-feature-crud.md)** — Capability/Feature CRUD via API
   *Razão:* infra (F-001), cost (F-004/F-005/F-006), code (F-007/F-011),
   org (F-010) e leitura (F-014/F-015) já entregam massa crítica nos
   planos descobertos automaticamente. Falta o plano `product` ganhar
   ancoragem — `Capability/Feature` são **decididos** pelos humanos,
   não descobertos, então sem CRUD básico E-005/E-006 ficam sem
   pivot para amarrar entrega de produto a custo e código. F-012 é
   prerequisito direto de F-013 (Feature → Service via PR), que fecha
   o ciclo produto↔código↔custo.

2. **[F-008](../features/F-008-openapi-ingest.md)** — OpenAPI ingest
   *Razão:* F-007 cobre AST Go; F-008 estende a mesma família de
   `Endpoint`s para serviços externos / linguagens não suportadas
   via contrato OpenAPI. Sem ela, plano `code` fica cego a uma
   fatia importante do estoque. Não bloqueia F-012, mas pode
   andar em paralelo se houver braço.

3. **Follow-up `BusinessArea`** — novo Kind + ingest + endpoint
   `GET /v1/business-areas*` (recorte tirado de F-015 por modeling
   impact). Detalhes em
   [pendencias.md](pendencias.md#follow-up-businessarea-recorte-de-f-015).
   Promover quando algum consumer real pedir agrupamento acima de
   `Team` (não há demanda agora).

## Concluídas

- ✅ [F-001](../features/F-001-aws-resource-discovery.md) — AWS Resource Discovery (S-001 a S-008; cobertura EC2/EBS/S3/RDS/VPC/Subnet/SG/LB com resiliência a erros parciais)
- ✅ [F-002](../features/F-002-bitemporal-upsert.md) — Bitemporal Upsert
- ✅ [F-003](../features/F-003-urn-bridge-to-cur.md) — URN Bridge para AWS CUR (S-001 a S-005; pacote `bridge`, ARN↔short, AsOf, batch ≤5s, ambíguo/cross-account)
- ✅ [F-004](../features/F-004-cur-parser.md) — CUR Parser (S-001 a S-008; entity/cost, parser CUR v1/v2, source local+S3, sink ClickHouse com ReplacingMergeTree, CLI `ce ingest cur`, integration tests CLICKHOUSE_TEST_URI, parser ≥50k rows/s)
- ✅ [F-005](../features/F-005-allocation-engine.md) — Allocation Engine (S-001 a S-008; aggregator in-memory + bridge batch + plugin DimensionResolvers, `fct_cost_by_urn` + `fct_unallocated_cost` RMT(allocated_at), CLI `ce allocate`, integration test do invariante TotalCUR ≈ Allocated + Unallocated)
- ✅ [F-007](../features/F-007-go-ast-extractor.md) — Go AST extractor (S-001 a S-008; URNs `urn:ce:code:<repo>:…`, walker `go.mod`, extractors `go/ast` para Function/Endpoint com suporte a `net/http` + chi, edges `DEFINED_IN`, CLI `ce extract code`, integration test de idempotência)
- ✅ [F-010](../features/F-010-hris-ingest.md) — HRIS ingest CSV (S-001 a S-008; entities `Person/Team/Squad` com hash SHA256 do email, parser CSV com `RowError` não-fatal, emit determinístico + detecção de ciclo em manager, edges `MEMBER_OF/PART_OF/REPORTS_TO`, CLI `ce ingest hris`, integration test 50 linhas + idempotência + end_date + PII)
- ✅ [F-006](../features/F-006-shared-infra-cost.md) — Shared cost allocation (S-001 a S-007; schema CH aditivo `allocation_type/rule_id/rule_version`, regras YAML `proportional_to_allocated`/`by_destination`/`static_override` ([ADR-003](../architecture/decisions/ADR-003-allocation-rules-as-yaml.md)), engine determinístico com Drops, CLI `ce allocate-shared`, integration test do invariante `in ≈ shared + remaining + drops`)
- ✅ [F-014](../features/F-014-rest-architecture-traversal.md) — REST /v1/architecture (S-001 a S-008; extensão `NodeRepository.Search` + `EdgeRepository.Paths` em memory/n4j, scaffold `internal/app/api/v1` stdlib com middlewares RequestID/Recover/Auth, endpoints `nodes/{urn}` + `neighbors` (depth ≤ 3, ≤ 100) + `history` (≤ 500) + `search` (cursor opaco base64+fingerprint SHA256) + `paths` (max_hops ≤ 5, ≤ 100 caminhos), binary `cmd/api` com graceful shutdown e backend memory|neo4j, OpenAPI 3.0.3 em `docs/api/v1/openapi.yaml`)
- ✅ [F-009](../features/F-009-bridge-service-to-compute.md) — Service↔Compute bridge (S-001 a S-008; edge novo `RUNS_ON` ([ADR-004](../architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md)), resolvers tag/service.urn/name_convention com longest-match, engine determinístico open/close/orphan/ambiguous, repo adapter sobre Node/EdgeRepository, CLI `ce bridge service-compute`, integration test full-cycle com drift + idempotência)
- ✅ [F-011](../features/F-011-codeowners-to-owns.md) — CODEOWNERS → Person.Owns(Service) (S-001 a S-008; edge `OWNS` com adjacência `[Person, Team] → [Service]`, `Person.GithubHandle` opcional, parser `CODEOWNERS` regra global + RowError não-fatal, resolver `@user`/`@org/team` com índice in-memory, `ServiceLookup` 1:1 repo→service com detecção mono-repo, engine close-and-reopen com identidade lógica `(from, target)`, CLI `ce extract codeowners`, integration test full-cycle close/reopen + duplicate owner + idempotência)
- ✅ [F-015](../features/F-015-rest-teams-hierarchy.md) — REST /v1/teams/* (S-001 a S-008; rotas `/v1/teams*`, `/v1/squads*`, `/v1/people*` com dispatch por sufixo, paginação cursor reusando fingerprint de F-014, PII via `EmailHint` mascarado, `?as_of` + `?include_inactive` plumbed via `NodeFilter`/`EdgeFilter` em memory + n4j, reports tree com cap depth=3 e detecção de ciclo, OpenAPI estendida com 7 paths + schemas `TeamView`/`SquadView`/`PersonView`/`ReportNode`/`ReportTree`, integration tests end-to-end e estabilidade de cursor entre `as_of`. `BusinessArea` punted como follow-up explícito)

## Adiadas (com justificativa)

(nenhuma agora)

## Princípios de priorização

- **Caminho crítico para MVP útil:** E-001 + E-002 (infra + cost
  básico) = primeiro produto demonstrável.
- **Evitar trabalho preditivo:** features de E-005/E-006 só refinar
  quando E-001/E-002 estiverem desbloqueando valor real.
- **Paralelismo só quando há paralelismo real** (gente disponível).
