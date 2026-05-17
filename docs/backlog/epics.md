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

## E-007 — Cross-Language Code Ontology

**Objetivo:** code plane deixa de ser Go-specific e ganha família
completa de nós (Module, Type, Schema, Variable, Call family,
Framework/License/CVE). Estende e em parte substitui F-007.
Fundação técnica para governança fina, simulação de fluxo e análise
de impacto.

**Documentação:** [`docs/architecture/08-code-ontology.md`](../architecture/08-code-ontology.md)
· ADRs [ADR-006](../architecture/decisions/ADR-006-cross-language-code-identity.md)
· [ADR-007](../architecture/decisions/ADR-007-call-as-node-family.md)
· [ADR-008](../architecture/decisions/ADR-008-params-fields-as-metadata.md)

**Features (planejadas — ver [code-ontology-plan.md](code-ontology-plan.md)):**
- F-017 — Migração de identidade Service/Endpoint/Function para cross-language
- F-018 — Module como nó com CONTAINS aninhável
- F-019 — Call family: boundary (HttpCall, RpcCall, EventPublish/Subscribe, QueueSend/Receive, DataAccess, JobSchedule)
- F-020 — Call family: in-process (FunctionCall, MethodCall)
- F-021 — Type e Variable como nós + edges entre Types (IMPLEMENTS/EXTENDS/ALIASES)
- F-022 — Schema como nó (proto/OpenAPI) + Type -SERIALIZES_AS-> Schema
- F-023 — Framework/License/SecurityAdvisory globais

**Critério de épico pronto:** rodar coletor Go em repo real produz
grafo completo (Service/Module/Endpoint/Function/Type/Variable/Schema/Call/Framework),
queries de impacto e patching de CVE executam ponta-a-ponta.

---

## E-008 — Governance & Delivery Mapping

**Objetivo:** governance plane completo (eixos org, product-arch, delivery,
audiência) e ponte denormalizada para o code plane via feature-tag.
Materializa a camada de intenção que ancora as queries de governança.

**Documentação:** [`docs/architecture/08-code-ontology.md`](../architecture/08-code-ontology.md) (seções 3 e 4)
· ADRs [ADR-009](../architecture/decisions/ADR-009-feature-tag-denormalized.md)
· [ADR-010](../architecture/decisions/ADR-010-ownership-bifurcated.md)

**Features (planejadas — ver [code-ontology-plan.md](code-ontology-plan.md)):**
- F-024 — Domain e Capability como nós (estende F-012)
- F-025 — Epic e UserStory como nós
- F-026 — Persona como nó (apenas via UserStory)
- F-027 — Role como nó por (track, level)
- F-028 — Feature-tag denormalizada em código
- F-029 — Ownership bifurcada (Team alto-nível, Person execução; estende F-011)

**Critério de épico pronto:** drift "feature sem código" e "tag órfã"
detectado em CI; reorganização org não destrói histórico bitemporal.

---

## Ordem de épicos sugerida

E-001 → E-002 → (E-003 ‖ E-004) → E-005 → E-006
                                                ↘
                                                  E-007 → E-008

E-001 e E-002 são pré-requisito para tudo. E-003 e E-004 podem ser
paralelos. E-005 precisa de E-003 e E-004 maduros. E-006 começa cedo
mas só dá valor real depois de E-001+E-002.

**E-007 substitui parcialmente E-003:** F-017 reescreve a identidade
de F-007 cross-language. Pode ser feito assim que F-002 e F-007
estiverem estáveis. **E-008 substitui parcialmente E-005:** F-024+
estende F-012 com hierarquia completa.
