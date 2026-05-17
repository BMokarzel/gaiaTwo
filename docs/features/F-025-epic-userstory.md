---
id: F-025
title: Epic + UserStory (delivery layer)
status: entity-layer done; integrações ALM pendentes
modules: [governance]
depends_on: [F-024]
modeling_impact: yes
adrs: [ADR-009]
epic: E-008
updated: 2026-05-17
---

# F-025 — Epic & UserStory

## Problema

Feature é categórica; Epic/UserStory descrevem entregas concretas com
prazo e métricas. Permite ligar código a delivery sem inflar Feature.

## Mudanças

- Kinds: `Epic`, `UserStory`.
- URN: `urn:ce:gov:<company>:epic/<short-id>` e
  `urn:ce:gov:<company>:user_story/<short-id>`.
- Arestas:
  - `CONTAINS` Epic→UserStory.
  - `DELIVERS` Feature→UserStory.
  - `ASSIGNED_TO` Person→UserStory (cardinalidade 1).
- `Epic.BusinessGoal`, `SuccessMetrics`, `TargetDate` (ISO 8601),
  `Status` (draft/active/done/cancelled).

## DoD

- [x] `node.Epic` + `node.UserStory` + URNs + ContentHash.
- [x] Adjacencies CONTAINS/DELIVERS/ASSIGNED_TO validadas.
- [ ] Sync Jira/Linear — backlog.
- [ ] Controllers REST — backlog.
