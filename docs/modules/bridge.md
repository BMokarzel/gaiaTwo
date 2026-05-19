---
id: M-bridge
title: Módulo Bridge
status: stable
plane: bridge
package: internal/modules/bridge
updated: 2026-05-18
---

# Módulo `bridge`

> Cruza planos do grafo: liga **infra ↔ cost**, **code ↔ infra** e
> **gov ↔ code**. É o lugar canônico para edges que tocam dois domínios
> sem fazer um deles importar o outro.

## Propósito

`bridge` resolve dois problemas que aparecem sempre que dois planos se
encontram:

1. **Identidade externa → URN canônica.** Sistemas de fora (CUR, IaC,
   provedores de cloud) carregam IDs nativos (`resource_id`, ARN). O
   pacote raiz `bridge` é o ponto único de lookup que traduz esses IDs
   para a URN do grafo, aplicando política `estrito → wildcard
   cross-account` (F-003).
2. **Edges cross-plane idempotentes.** Sub-pacotes orquestram pipelines
   que emitem edges entre planos: `RUNS_ON` (Service→Compute) e
   `Realizes` (Feature→Service). Tudo bitemporal, com source/confidence
   e ID determinístico para re-entrega segura.

## Escopo

**Inclui:**
- `bridge.Resolver` — lookup `externalID → URN`, single e batch.
- `bridge/service_compute` — engine F-009 que materializa
  `RUNS_ON(Service→Compute)` via estratégias tag/name/manifest (ADR-004).
- `bridge/github_webhook` — handler de `POST /v1/webhooks/github` (F-013)
  que materializa `Realizes(Feature→Service)` a partir de PRs mergeados.

**NÃO inclui:**
- Coleta de infra (vive em `modules/infra/collector/`).
- Coleta de código (vive em `modules/code/collector/`).
- Parser de CUR (vive em `modules/cost/parser/`) — `bridge` é
  consumido por ele, não o contrário.
- CRUD de Feature/Service (vivem em `modules/gov` e `modules/code`).

## Sub-pacotes

| Pacote | Propósito | Origem |
|---|---|---|
| `bridge` (raiz) | Resolver de `externalID → URN` (estrito + wildcard) | F-003, F-004 |
| `bridge/service_compute` | Pipeline Service→Compute (edge `RUNS_ON`) | F-009, ADR-004 |
| `bridge/github_webhook` | Handler de PRs mergeados → `Realizes` | F-013 |

## Edges cross-plane emitidos

| Edge | From → To | Emitido por | Identidade determinística |
|---|---|---|---|
| `RUNS_ON` | Service (code) → Compute (infra) | `service_compute.Engine` | `(serviceURN, RUNS_ON, computeURN, validFrom)` |
| `Realizes` | Feature (gov) → Service (code) | `github_webhook.Service` | `(featureURN, REALIZES, serviceURN, mergedAt)` |

Demais edges cross-plane (`BackedBy`, `Maintains`, `BudgetCovers`,
`DeployedOn` code→infra) ainda não são emitidos por pipelines em
`bridge/` — quando entrarem, este doc passa a listar.

## Política de resolução (`bridge.Resolver`)

Para um par `(provider, account, externalID)`:

1. **Match estrito** — `GetByExternalID(provider, account, externalID)`.
   Se 1 hit, devolve. Se 0 hits, segue para passo 2. Se `ErrAmbiguous`
   ou erro fatal, propaga.
2. **Wildcard cross-account** — `GetByExternalID(provider, "", externalID)`.
   Cobre recursos "globais" (S3 bucket, IAM role) que o CUR atribui à
   conta A mas o grafo mantém em conta B. Múltiplos hits → `ErrAmbiguous`
   (caller decide: fila unresolved, log).

`AsOf` é aceito para reprocessar CUR antigo contra o estado histórico
do grafo (F-003/S-003). Não há cache no Resolver — repositories
otimizam (UNWIND no Neo4j) por baixo.

## Política de `service_compute` (F-009)

Para cada `Compute` corrente na conta-alvo, aplica estratégias em
ordem (curto-circuita na primeira que produz Service válido):

1. **`tag`** — `Tags["service"]` (ou similar) → lookup do Service
   em `modules/code` por slug. `confidence=0.95`.
2. **`name_convention`** — regex em `Compute.Name` (ex.: `svc-checkout-prod-a`).
   `confidence=0.8`.
3. **`manifest`** — placeholder para `costengine.yaml` no repo do
   Service. Ainda não implementado (ver `pendencias.md §5`).

Reconciliação contra estado atual:
- **match** → noop.
- **mismatch** (dono mudou) → fecha edge antigo (`valid_to=now`) e
  abre um novo com `source`/`confidence` da estratégia que resolveu.
- **ausente → resolvido** → abre edge novo.
- **ausente → ambíguo/orphan** → não emite; registra em
  `Output.Ambiguous` / `Output.Orphans`.

Determinismo: iteração de Computes em ordem lexicográfica de URN
(testabilidade).

## Política de `github_webhook` (F-013)

Endpoint: `POST /v1/webhooks/github` (registrado via flag
`--github-webhook-secret`).

1. **Verifica HMAC SHA-256** do header `X-Hub-Signature-256`. Falha →
   401, payload não é parseado.
2. **Filtra evento** — só processa `action=closed` + `merged=true`.
   Demais ações retornam 204.
3. **Extrai Feature URN** da label `feature:<urn>`. Sem label, ignora
   (204). Label apontando para URN inexistente devolve 400 (ver fila
   pendente em `pendencias.md §3`).
4. **Resolve cada path** para a URN de um `Service` por
   longest-prefix-match contra `Service.ModulePath`. Paths que não
   batem ficam fora.
5. **Upsert idempotente** de `Realizes(Feature→Service)` por par
   distinto, com `pr_url` e `merged_at` em `Meta.Properties`.

ID determinístico (`DeterministicID(featureURN, REALIZES, serviceURN,
mergedAt)`) garante que re-entregas do mesmo webhook são noop.

## Storage

`bridge/*` não tem storage próprio. Lê e escreve via
`repository.NodeRepository` / `repository.EdgeRepository` injetados.
Não importa `n4j` nem `memory` diretamente — tudo passa pela interface
(ADR-005).

## Contratos públicos

```go
// bridge (raiz)
type Resolver interface {
    ResolveURN(ctx, provider, account, externalID, asOf) (URN, error)
    ResolveURNBatch(ctx, provider, account, externalIDs, asOf) (map[string]URN, []ResolveError, error)
}

// bridge/service_compute
type Engine interface {
    Run(ctx, RunInput) (Output, error)
}

// bridge/github_webhook
type Service interface {
    HandlePullRequest(ctx, payload) (HandleResult, error)
}
// Controller registrado via httpserver.Registrar.
```

Erros não tipados (sem `HTTPProblem` próprio). Falhas propagam
sentinels de `repository` (`ErrNotFound`, `ErrAmbiguous`,
`ErrInvalidArgument`); o controller de `github_webhook` faz o mapping
via `core/errs.Render`.

## Bitemporal

Edges emitidos são bitemporais como qualquer outro:
- `valid_from` = momento da reconciliação (RUNS_ON) ou `merged_at`
  (Realizes).
- `valid_to` = `now()` quando reconciliação detecta mismatch.
- `observed_at` = sempre `now()` da execução.
- `confidence` = vem da estratégia/fonte.

Re-execuções com mesmo input são noop — comparação compara o
"diff" lógico antes de gravar.

## Dependências

- `core/urn` — parser/builder de URN.
- `core/edge` — interface Edge + adjacency matrix.
- `core/bitemporal` — Meta, ValidFrom/ValidTo, ConfidenceSource.
- `repository.NodeRepository`/`EdgeRepository` (injetados).
- `modules/code` (entity-only) — `Service.ModulePath`, `Service.Slug`.
- `modules/infra` (entity-only) — `Compute.Tags`, `Compute.Name`.
- `modules/gov` (entity-only) — `Feature.URN` (para validação).

Bridge **não importa** `controller/` dos outros módulos (depguard).

## Features relacionadas

- **F-003** — URN bridge to CUR
- **F-004** — CUR parser (consumer do Resolver)
- **F-009** — Bridge service→compute (`service_compute`)
- **F-013** — Feature→Service via PR (`github_webhook`)

## ADRs relacionadas

- **ADR-001** — Monolito modular inicial
- **ADR-004** — `RUNS_ON` como edge canônico de Service→Compute
- **ADR-005** — Controllers não importam repository

## Open questions

1. **Manifesto `costengine.yaml`** — formato decidido em
   [ADR-011](../architecture/decisions/ADR-011-costengine-yaml-manifest.md).
   Falta implementar parser/validador e a estratégia `manifest` em
   `service_compute` (consome `services[].compute`). Promover a feature
   quando primeiro repo declarar o manifesto.
2. **Fila de pendentes** para `github_webhook` (label aponta para
   Feature inexistente): replay quando URN aparecer — F-013 backlog.
3. **Hidratação de `files[]`** via GitHub REST API: hoje exige proxy
   que enriquece o payload; payload nativo não traz a lista (F-013
   backlog).
4. **Outros providers VCS** (GitLab MR, Bitbucket PR): feature
   dedicada futura, não slice de F-013.

## Estado de implementação

- ✅ `Resolver` (estrito + wildcard, single + batch)
- ✅ `service_compute.Engine` (tag + name strategies, reconciliação)
- ✅ `github_webhook` (HMAC, payload, resolver, service, controller)
- ⏳ Estratégia `manifest` em `service_compute` (formato decidido por
  ADR-011; falta implementação)
- ⏳ Fila de pendentes + hidratação `files[]` no `github_webhook`
- ⏳ Outros providers VCS (feature futura)
