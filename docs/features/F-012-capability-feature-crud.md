---
id: F-012
title: Governance CRUD via REST (Company/BA/Domain/Capability/Feature)
status: done
modules: [gov]
depends_on: [F-024]
modeling_impact: no
adrs: [ADR-001, ADR-009]
epic: E-005
updated: 2026-05-17
---

# F-012 — Capability/Feature CRUD via API

## Problema

`Capability` e `Feature` modelam o que o produto entrega. Diferente de
infra/code (descobertos), capabilities são **decididas** pelos humanos
e precisam de input manual. Sem CRUD básico, plano `product` fica
vazio e E-005/E-006 perdem ancoragem.

**Para quem:** product managers via produto Kanban (futuro) ou
diretamente via CLI; servidor responde tanto a esses quanto a BFF.

**Dor sem ela:** capabilities ficam em planilha externa; impossível
ligar a code/cost no grafo.

## Escopo

**Inclui:**
- Endpoints REST `/v1/capabilities` (POST, GET, PATCH, DELETE soft) e
  `/v1/features` idem.
- Payload: `name`, `description`, `parent_capability_urn` (para
  feature → capability), `domain_urn`, `status` (active/deprecated).
- Validação de hierarquia (Capability sob Domain, Feature sob
  Capability).
- Auditoria via bitemporal (sem endpoint dedicado de histórico no MVP).

**NÃO inclui:**
- UI (vive em produto Kanban — fora deste projeto).
- Sync com Linear/Jira (feature dedicada).
- Permissionamento granular (auth no BFF; serviço confia em tenant
  passado).

**Precondições:**
- Domain pré-existente (por enquanto, criado via mesmo padrão de
  endpoint — adicionar no escopo ou marcar follow-up).
- Bitemporal repository (F-002 ✅).

## Toque no grafo

- **Lê:** `Domain`, `Capability` (validação de pai).
- **Escreve:** `Capability`, `Feature` + edges `PartOf` e
  `BelongsTo` via `internal/modules/product`.
- **Novos Kinds/edges:** nenhum (já em `03-business.md`).
- **Bitemporal:** PATCH cria nova versão; DELETE faz soft (fecha
  `valid_to`).

## Critérios de aceite

- [x] `POST /v1/gov/{kind}` com payload válido cria nó versão 1 e
      retorna URN (201). Cobre `companies`, `business_areas`, `domains`,
      `capabilities`, `features`.
- [x] `GET /v1/gov/{kind}/{urn}` retorna versão corrente. (F-024)
- [x] `GET /v1/gov/{kind}/{urn}?as_of=...` retorna versão vigente.
      (F-024)
- [x] `PATCH /v1/gov/{kind}/{urn}` aplica diff e versiona via repo
      bitemporal (semântica de ponteiro: nil = não tocar).
- [x] `DELETE /v1/gov/{kind}/{urn}` fecha `valid_to`. Subsequente GET
      retorna 404 (sem `as_of`).
- [x] Criar filho com parent inexistente → 400 com `gov.validation`.
- [x] Criar filho com parent de kind errado → 400 com `gov.urn.invalid`.
- [x] `short_id` validado como kebab-case (`[a-z0-9-]`, sem hífen na
      borda, ≤64 chars) — ADR-009.
- [x] Conflito (`short_id` já existe) → 409 `gov.<kind>.conflict`.
- [x] CONTAINS edge emitido pai→filho em todos os Create (Company não
      tem pai; Feature usa `parent_feature_urn` se presente, senão
      `parent_capability_urn`).
- [x] Tenant lido do header `X-Tenant-ID` (ADR-001), nunca do payload.
- [x] `DisallowUnknownFields` no decode (falha com 400 em chaves
      desconhecidas — previne typo silencioso).

## Riscos / incerteza

- **Endpoint na API monolítica vs futuro produto-svc.** Decisão por
  ADR-001: tudo no monolito. Caminho pronto para extrair.
- **Domain handling.** Provavelmente precisa de endpoint similar
  (`/v1/domains`). Não bloqueante mas vale criar no mesmo PR.
- **PATCH semântico.** Apenas campos enviados mudam; envio do mesmo
  valor **não** versiona (idempotência).
- **Tenant.** Multi-tenant exige `tenant_id` no URN e no middleware
  (ADR-001 semente). Endpoint precisa respeitar.

## Notas de implementação

- Handlers em `internal/modules/gov/controller/writes.go`. Service
  layer em `internal/modules/gov/service/writer.go` — receiver é o
  mesmo `*service` que serve os reads (F-024), agora segurando tanto
  `NodeRepository` quanto `EdgeRepository`.
- Reuso do upsert bitemporal: o repo (memory + n4j) versiona se o
  conteúdo mudar e no-op se for idêntico. Não validamos diff antes —
  delegamos pra ele.
- `closeNode`/`touchNode` em `memory.Repo` foram estendidos para cobrir
  os 8 gov kinds (Company, BusinessArea, Domain, Capability, Feature,
  Epic, UserStory, Persona). Antes do F-012, `Delete` era no-op para
  esses tipos (bug latente — F-024 era read-only).

## Backlog

- Endpoints CRUD para Epic / UserStory / Persona (F-025/F-026) — mesmo
  pattern; pendentes pelas integrações ALM.
- CLI `ce gov create-{kind}` espelhando o REST (conveniência ops).
- Bulk import YAML/CSV — fora do MVP.
