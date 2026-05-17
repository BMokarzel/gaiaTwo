---
id: F-024
title: Company / BusinessArea / Domain / Capability / Feature
status: entity + REST CRUD + CLI done (writes em F-012); UI pendente
modules: [governance]
depends_on: [F-017]
modeling_impact: yes
adrs: [ADR-009]
epic: E-008
updated: 2026-05-17
---

# F-024 — Eixo product-arch

## Problema

O grafo precisa de uma taxonomia de produto separada do eixo
organizacional (Team/Person). Domain/Capability/Feature respondem
"o que o produto faz?" sem confundir com "quem é dono?".

## Mudanças

- Novo provider `ProviderGov` (distinto de `ProviderOrg`).
- Kinds: `Company`, `BusinessArea`, `Domain`, `Capability`, `Feature`.
  URN: `urn:ce:gov:<company>:<kind>/<short-id>`.
- `Feature` suporta aninhamento (Feature→Feature via CONTAINS).
- `Feature.ShortID` é kebab-case único por company; é a chave usada
  por `feature_tags` no código (ADR-009).
- Adjacency `CONTAINS` ampliada para Company→BusinessArea,
  BusinessArea→Domain, Domain→Capability, Capability→Feature,
  Feature→Feature.

## DoD

- [x] 5 kinds + URNs + ContentHash.
- [x] Adjacency CONTAINS amplificada.
- [x] Module `gov` com `Service` (port), `service.New` (reader-only),
  controller `/v1/gov/{companies,business_areas,domains,capabilities,features}`
  + `/v1/gov/features/{urn}/children`.
- [x] Convenções espelhadas do `org`: parse → svc → view, RFC 7807,
  cursor pagination com fingerprint, `as_of` + `include_inactive`.
- [x] Write side (POST/PATCH/DELETE) via **F-012** — sobre o mesmo
  módulo `gov`.
- [x] CLI `ce gov *` espelhando o REST (create/get/list/patch/delete
  × company|business-area|domain|capability|feature), backend
  memory|neo4j via flags, PATCH semântico via `flag.Visit`. Tenant
  injetado no contexto pelo helper `httpserver.ContextWithTenant`
  (CLI não viola ADR-001).
- [ ] UI de cadastro — fora do escopo (Kanban em outro projeto).
- [ ] `ListFeatureChildren` via query dedicada no `NodeRepository`
  (hoje filtra in-memory por `ParentFeatureURN`) — backlog para
  catálogos grandes.
