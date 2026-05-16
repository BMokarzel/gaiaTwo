---
id: F-012
title: Capability/Feature CRUD via API
status: refined
modules: [product]
depends_on: []
modeling_impact: no
adrs: [ADR-001]
epic: E-005
updated: 2026-05-13
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

- [ ] `POST /v1/capabilities` com payload válido cria nó versão 1 e
      retorna URN.
- [ ] `GET /v1/capabilities/{urn}` retorna versão corrente.
- [ ] `GET /v1/capabilities/{urn}?as_of=2026-01-01` retorna versão
      vigente naquela data.
- [ ] `PATCH /v1/capabilities/{urn}` com mudança real cria versão 2 e
      fecha a 1.
- [ ] `DELETE /v1/capabilities/{urn}` fecha `valid_to`. Subsequente
      GET retorna 404 (sem `as_of`).
- [ ] Criar Feature com `parent_capability_urn` inexistente retorna
      400.
- [ ] Resposta sempre inclui `version` e `valid_from`.

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

- Handlers em `internal/modules/product/api/`.
- Reuso de validação genérica de bitemporal upsert (F-002).
