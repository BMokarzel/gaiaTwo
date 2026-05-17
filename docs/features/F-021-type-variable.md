---
id: F-021
title: Type + Variable nodes (struct/class/interface/enum/alias/union)
status: done (coletor Go cobre Type+Variable+EXTENDS+ALIASES; IMPLEMENTS backlog)
modules: [code]
depends_on: [F-018]
modeling_impact: yes
adrs: [ADR-006, ADR-008]
epic: E-007
updated: 2026-05-17
---

# F-021 — Type & Variable

## Problema

Refactor de contrato (`User struct`, `interface OrderService`) afeta
todos os consumidores. Sem o tipo como nó, impacto fica invisível.
Globals/constantes top-level também são pontos de contato.

## Mudanças

- Novo kind `Type` em `internal/entity/node/type.go` com discriminador
  `TypeKind`: struct/class/interface/enum/alias/union.
- Novo kind `Variable` em `internal/entity/node/variable.go` com
  `Mutability`: const/var. Variáveis locais NÃO viram nó.
- `Fields []FieldSlot` e `Methods []MethodSlot` estruturadas (ADR-008).
  Primitivos ficam literal em `TypeRef`; tipos declarados viram URN.
- Arestas: `IMPLEMENTS`, `EXTENDS`, `ALIASES` (todas Type→Type).
- URN: `urn:ce:code:<repo>:type/<service-module-path>!<namespace>!<symbol>`.

## DoD

- [x] `node.Type` + `node.Variable` + URNs + ContentHash.
- [x] Adjacency IMPLEMENTS/EXTENDS/ALIASES validadas.
- [x] Coletor Go (`ExtractTypes` em `collector/golang/type.go`) varre
      `*ast.TypeSpec` em pacotes relevantes (mesmo filtro de
      `ExtractFunctions`): struct → `TypeKindStruct` + Fields (tags
      capturadas); interface → `TypeKindInterface` + MethodSlot;
      `type X = Y` / `type X Y` → `TypeKindAlias`; var/const top-level
      → `node.Variable` com `Mutability`.
- [x] Test: struct com embedded `Base` em `User` gera EXTENDS
      `User→Base` (resolvido por nome curto no mesmo namespace).
      Cobre interface embedding, ALIASES, e idempotência
      (`type_test.go`).
- [ ] IMPLEMENTS (struct satisfaz interface por set de métodos) —
      backlog. Requer pass de "métodos por receiver" cruzado com
      `Type.Methods` da interface; sintático sem type-resolve dá
      falsos negativos aceitáveis.
