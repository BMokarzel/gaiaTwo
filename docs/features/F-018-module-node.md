---
id: F-018
title: Module como nó com CONTAINS aninhável
status: done
modules: [code]
depends_on: [F-017]
modeling_impact: yes
adrs: [ADR-006]
epic: E-007
updated: 2026-05-17
---

# F-018 — Module node

## Problema

F-007 prendia Function/Endpoint diretamente em Service via DEFINED_IN.
Para suportar agrupamento físico (pasta, package, namespace) com
aninhamento arbitrário e query ergonômica (`MATCH
(:Module)-[:CONTAINS*]->(:Function)`), precisamos de Module como nó
de primeira classe.

## Mudanças

- Novo kind `Module` em `internal/entity/node/module.go`.
- URN: `urn:ce:code:<repo>:module/<service-module-path>!<namespace>`.
- Aresta `CONTAINS` (já existente no infra plane) extendida para
  Service→Module, Module→Module, Module→Function/Endpoint/Type/
  Variable/Call.
- Coletor Go (`ExtractModules`) descobre namespaces a partir dos
  `.go` files, deriva ancestrais e popula `ParentURN`.
- Function/Endpoint ganham `ModuleURN` (opcional durante transição).
- `DEFINED_IN` mantido para compat — deprecated em favor de CONTAINS.

## DoD

- [x] `node.Module` + `NewModuleURN` + `ContentHash`.
- [x] `edge.Contains` adjacency aceita Service→Module, Module→Module,
      Module→Function/Endpoint/Type/Variable/Call.
- [x] Coletor Go emite Modules + Contains edges; re-run idempotente.
- [x] Test cobre nesting (`internal/api/users` → 3 Modules).
