# Épicos

> Agrupam features relacionadas para conversa de estratégia. Não são
> docs autônomos — apontam para features.

---

## E-001 — Infra Foundation

**Objetivo:** o grafo reflete a realidade da infra AWS de uma conta.

**Features:**
- [F-001](../features/F-001-aws-resource-discovery.md) — AWS Resource Discovery — `refined`
- [F-002](../features/F-002-bitemporal-upsert.md) — Bitemporal Upsert no Neo4j — `done`

**Critério de épico pronto:** rodar `ce extract aws --account=X` numa
conta real popula o grafo com fidelidade e idempotência.

---

## E-002 — Cost Layer

**Objetivo:** o grafo carrega custo atribuído por recurso e período.

**Features:**
- [F-003](../features/F-003-urn-bridge-to-cur.md) — URN Bridge para AWS CUR — `ready`
- [F-004](../features/F-004-cur-parser.md) — CUR Parser (Parquet → fact table) — `refined`
- [F-005](../features/F-005-allocation-engine.md) — Allocation Engine (URN × period × dimension) — `refined`
- [F-006](../features/F-006-shared-infra-cost.md) — Shared infra cost (data transfer, support) — `refined` (⚠ modeling_impact unknown)

**Critério de épico pronto:** consulta `cost(urn, period)` retorna valor
correto e auditável (drill-down até linha CUR).

---

## E-003 — Code Plane

**Objetivo:** serviços/endpoints/funções/tables são nós do grafo,
linkados a infra.

**Features:**
- [F-007](../features/F-007-go-ast-extractor.md) — Go AST extractor — `refined`
- [F-008](../features/F-008-openapi-ingest.md) — OpenAPI ingest — `refined`
- [F-009](../features/F-009-bridge-service-to-compute.md) — Bridge Service → Compute (DeployedOn) — `refined`

---

## E-004 — Org & Ownership

**Objetivo:** pessoas, times e squads no grafo, linkados a código.

**Features:**
- [F-010](../features/F-010-hris-ingest.md) — HRIS ingest (CSV mínimo) — `refined`
- [F-011](../features/F-011-codeowners-to-owns.md) — CODEOWNERS → Person.Owns(Service) — `refined`

---

## E-005 — Product & Capability

**Objetivo:** capabilities/features mapeadas a serviços e custo.

**Features:**
- [F-012](../features/F-012-capability-feature-crud.md) — Capability/Feature CRUD via API — `refined`
- [F-013](../features/F-013-feature-service-link-via-pr.md) — Feature → Service link via PR label — `refined`

---

## E-006 — Plataforma de Leitura

**Objetivo:** APIs públicas que os produtos web consomem.

**Features:**
- [F-014](../features/F-014-rest-architecture-traversal.md) — REST `/v1/architecture/*` (traversal de grafo) — `refined`
- [F-015](../features/F-015-rest-teams-hierarchy.md) — REST `/v1/teams/*` (hierarquia org) — `refined`

---

## Ordem de épicos sugerida

E-001 → E-002 → (E-003 ‖ E-004) → E-005 → E-006

E-001 e E-002 são pré-requisito para tudo. E-003 e E-004 podem ser
paralelos. E-005 precisa de E-003 e E-004 maduros. E-006 começa cedo
mas só dá valor real depois de E-001+E-002.
