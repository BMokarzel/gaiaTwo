# Painel de Refinamento

> Estado de cada feature/épico. Atualiza junto com cada mudança de
> estado.
>
> Atualizado: 2026-05-17

## Features por estado

| Estado | Quantidade | IDs |
|---|---|---|
| `idea` | 0 | — |
| `refining` | 0 | — |
| `refined` | 0 | — |
| `ready` | 0 | — |
| `in_progress` | 0 | — |
| `done` | 23 | F-001..F-016 (exceto parciais), F-018, F-020, F-021, F-024, F-027, F-028, F-029, **F-013** |
| `done (com backlog parcial)` | 5 | F-017, F-019, F-022, F-023, F-025 |
| `entity-layer only` | 1 | F-026 |
| `blocked` | 0 | — |

## Épicos por estado

| Épico | Estado | Features `done`/total |
|---|---|---|
| E-001 Infra Foundation | done | 2/2 (F-001, F-002) |
| E-002 Cost Layer | done | 4/4 (F-003..F-006) |
| E-003 Code Plane | done | 3/3 (F-007, F-008, F-009) |
| E-004 Org & Ownership | done | 2/2 (F-010, F-011) |
| E-005 Product & Capability | done | 2/2 (F-012, F-013) |
| E-006 Plataforma de Leitura | done | 3/3 (F-014..F-016) |
| E-007 Code Ontology | parcial | 5/6 + backlog (F-017 partial, F-018 done, F-019 mostly, F-020 done, F-021 done, F-022 mostly, F-023 mostly) |
| E-008 Governance Taxonomy | parcial | 1/4 + backlog (F-024 done; F-025 partial; F-026/F-027/F-028/F-029 misturados — ver lista por feature) |

## Atenção / stale

- Sem features `refined` pendentes. Próximos incrementos são slices
  de backlog dentro das features já done (linha "Backlogs vivos").
- F-026 (Persona): entity-layer existe há tempos, mas sem coletor.
  Sem dor de produto registrada — manter em backlog.

## Backlogs vivos (por feature parcial)

- **F-017** Cross-language identity — coletores Python/TS pendentes
  (Go pronto). Sem dor concreta: usuários atuais são Go-only.
- **F-019** Call boundary — `EventSubscribe` (consumer kafka/pubsub)
  pendente. Provavelmente exige type-resolve (F-021++).
- **F-021** Type/Variable — `IMPLEMENTS` (struct↔interface) pendente.
- **F-022** Schema — JSON Schema standalone, Avro, GraphQL SDL,
  `oneOf`/`allOf`/`anyOf` em FieldSlots — todos backlog.
- **F-023** Framework/License/CVE — pyproject/Cargo, lookup SPDX,
  sync OSV/GHSA — todos backlog.
- **F-025** Epic/UserStory — integrações ALM (Jira/Linear/GitHub
  Issues) pendentes.
## Próximas ações de refinamento

1. Sem features `refined`/`in_progress`. Próximos incrementos são
   slices de backlog dentro das features parciais (linha
   "Backlogs vivos" acima) — priorizar quando aparecer dor concreta
   de usuário.

## Histórico recente

- 2026-05-17: F-013 entregue. Pacote
  `internal/modules/bridge/github_webhook/` (HMAC, parse, longest-prefix
  path→Service, upsert idempotente de `Realizes`). Edge `TypeRealizes`
  + adjacency `Feature→Service` adicionadas em `entity/edge`. Endpoint
  `POST /v1/webhooks/github` é opt-in via `--github-webhook-secret`.
  E-005 fecha 2/2.
- 2026-05-17: F-008 (OpenAPI ingest), F-022 (adapter OpenAPI) e
  F-024 (CLI `ce gov *`) marcadas como done. F-018, F-019 (subkinds
  gRPC/SQS/SNS/Kinesis/cron), F-020 (FunctionCall), F-021 tiveram
  doc-status sincronizado com a realidade do código.
- 2026-05-16: F-016 marcada como `done`. Catálogo de erros publicado em
  `docs/api/v1/errors.md`. `04-modular.md §9` atualizado refletindo
  passos 4-6 executados; passos 1-3 (`entity/*` → `core/*`) movidos
  para `pendencias.md`. Desvios documentados no próprio F-016
  (S-006/S-007 absorvidos por S-008 — `code` e `infra` sem controller
  próprio até aparecer endpoint de domínio).
- 2026-05-15: F-016 refinada (arquitetura modular v2: controllers por
  módulo, `core/errs`, `platform/httpserver`, port-sets em `app/`).
  Implementação executada ao longo do mesmo ciclo (S-001..S-011 no
  código).
- 2026-05-14: F-001 S-002 entregue. Hierarquia `Account → Region → Zone`
  + edges `Contains` via EC2 `DescribeAvailabilityZones`. Novos pacotes
  `infra/collector` (ScopeDiscoverer), `infra/collector/aws/topology`,
  `infra/service`. CLI persiste em backend memory; helper bitemporal de
  ancestrais reutilizável pelas próximas stories. 8 testes novos verdes.
- 2026-05-14: F-001 → `in_progress`. S-001 entregue (scaffold do
  collector AWS em `internal/modules/infra/collector/aws/` + CLI
  `ce extract aws` com validação STS GetCallerIdentity). Testes verdes.
- 2026-05-13: ADR-001 (monolito modular) e ADR-002 (bitemporal como
  invariante) escritas retroativamente.
- 2026-05-13: F-001 fatiada em 8 stories (S-001 a S-008), promovida
  para `ready`.
- 2026-05-13: F-003 fatiada em 5 stories (S-001 a S-005), promovida
  para `ready`.
- 2026-05-13: F-004 a F-015 refinadas em batch (idea → refined).
