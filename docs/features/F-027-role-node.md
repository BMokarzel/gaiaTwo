---
id: F-027
title: Role como nó por (track, level)
status: done
modules: [org]
depends_on: [F-010]
modeling_impact: yes
adrs: [ADR-009]
epic: E-008
updated: 2026-05-17
---

# F-027 — Role

## Problema

`Person.Role` como string livre acumula entropia (`SENIOR`, `Senior`,
`Sr.`, `eng-senior`) e impede query agregada. Role como nó normaliza
e habilita análise organizacional (custo por nível, distribuição de
seniority).

## Mudanças

- Kind `Role` no org plane. URN: `urn:ce:org:<tenant>:role/<track>-<level>`.
  Cardinalidade: 1 Role para muitas Persons.
- `Role.Track`: backend/frontend/mobile/data/sre/qa/pm/design/security/fullstack.
- `Role.Level`: junior/mid/senior/staff/principal/director/vp/c-level.
- `Role.IsLeadership` (bool).
- Arestas: `HAS_ROLE` Person→Role; `LED_BY` Team→Person (lideranças).
- Atributo `Person.Role` (F-010) fica deprecated durante migração.

## DoD

- [x] `node.Role` + `NewRoleURN` + ContentHash.
- [x] Adjacencies HAS_ROLE/LED_BY validadas.
- [x] Coletor HRIS (F-010) parseia free-text `role` para
      `(track, level)` via `parseRoleString`, dedup por URN, emite
      `node.Role` + aresta `HAS_ROLE` (Person→Role). Quando o parse
      falha, mantém só `Person.Role` string como fallback.
- [x] Reconciliação `LED_BY` em `hris.Build`: sinais combinados
      (alguém-reporta-pra-ele OR título com keyword Manager/Lead/Head/
      Director/VP/C-level OR Role parsed com `IsLeadership`). Emite
      Team→Person via Team do squad do líder. Pessoas terminadas
      (end_date preenchido) excluídas. Cross-team management fora do
      MVP.
