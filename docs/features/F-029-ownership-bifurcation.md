---
id: F-029
title: Ownership bifurcada — Team owna intenção, Person owna execução
status: done
modules: [edge]
depends_on: [F-027]
modeling_impact: yes
adrs: [ADR-010]
epic: E-008
updated: 2026-05-17
---

# F-029 — Ownership bifurcation

## Problema

`OWNS` polimórfico (Team OU Person → qualquer coisa) corrompe semântica:
Person owna `Service` mistura grão grosso de contrato com pessoa
individual; Team owna `Function` mistura grão fino de execução com
agrupamento. Resultado: queries de "blast radius" e "responsável"
ficam ambíguas.

## Decisão (ADR-010)

- **Team owna unidades de intenção/contrato**: Service, Module,
  Endpoint, Feature, Epic.
- **Person owna unidades de execução**: Function, Call, Type, Variable.

`OWNS` continua sendo uma única aresta — a bifurcação é semântica.

## Mudanças

- Adjacency `OWNS` (em `edge/registry.go`) lista as combinações
  permitidas hard (Validate).
- `ValidateOwnership(ownerKind, ownedKind)` é o lint suave: warning
  no coletor quando Team owna unidade fina ou Person owna unidade
  grossa. Não bloqueia ingestão — `Validate` é a hard rule.

## DoD

- [x] `edge.Validate` aceita os pares bifurcados.
- [x] `edge.ValidateOwnership` retorna erro nos pares "errados".
- [x] Tests cobrem ambos os caminhos.
- [x] Coletor `codeowners` (F-011) usa `ValidateOwnership` em `Build`;
      owners que falham (ex.: Person→Service) viram `Result.Rejected`
      e contam em `Stats.Rejected` — não entram no grafo como OWNS.
