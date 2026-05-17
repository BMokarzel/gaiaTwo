---
id: F-026
title: Persona node
status: entity-layer done
modules: [governance]
depends_on: [F-025]
modeling_impact: yes
adrs: [ADR-009]
epic: E-008
updated: 2026-05-17
---

# F-026 — Persona

## Problema

UserStory existe pra atender alguém. Persona materializa esse "alguém"
e permite tracear código a quem ele serve — sem denormalizar no
código (ADR-009: Persona NÃO é feature-tag).

## Mudanças

- Kind `Persona`. URN: `urn:ce:gov:<company>:persona/<short-id>`.
- Aresta `SERVES` UserStory→Persona.
- `Persona.Segment`: B2B/B2C/internal.

## DoD

- [x] `node.Persona` + URN + ContentHash.
- [x] Adjacency SERVES validada.
- [ ] Sync com ferramenta de produto (Productboard, etc.) — backlog.
