---
id: ADR-009
title: Feature como tag denormalizada em código, não edge
status: accepted
date: 2026-05-17
supersedes: []
superseded_by: []
related: [ADR-007]
---

# ADR-009 — Feature como tag denormalizada em código, não edge

## Contexto

Com `Call` como nó (ADR-007), o code plane carrega da ordem de milhões de
nós em um repositório real. Cada Call pode implementar uma ou mais
Features de negócio.

Modelar `Call -IMPLEMENTS-> Feature` como edge:
- Cardinalidade N:M com explosão em edges (1M calls × 1.5 features médias).
- Traversal Feature→Code dispara fan-out enorme.
- Bitemporal dobra: cada edge tem sua versão.

Modelar como atributo no nó:
- Tag = string curta (`short_id` da Feature).
- Lookup via index secundário em vez de traversal.
- Persist nó já paga o custo da serialização da array.

## Decisão

A ligação **código → Feature** é via atributo, não edge:

```jsonc
{
  "kind": "HttpCall",
  "id":   "urn:ce:code:repo:call/http!internal/api!handler.GetHome#3",
  "feature_tags": ["home-banner", "promo-strip"]
}
```

Mas a `Feature` **continua sendo nó** no governance plane, com hierarquia
própria (`Capability -CONTAINS-> Feature`), atributos (owner, status,
SLA, prioridade) e relacionamentos (`Feature -DELIVERS-> UserStory`).

A `feature_tags` é uma referência denormalizada via `short_id`:
- `Feature.short_id` é atributo único por company (kebab-case).
- `Feature.URN` é `urn:ce:gov:<company>:feature/<short_id>`.

Aplicável a qualquer nó do code plane (`Service`, `Module`, `Endpoint`,
`Function`, `Type`, `Variable`, `Call`).

**Validação:** coletor emite warning (lint, não erro) quando tag
referencia `short_id` não existente. Job de governance hygiene roda
periodicamente e gera relatório (`tags órfãs` + `features sem código`).

## Consequências

**Positivas:**
- Cardinalidade tratável: 1M calls × 1.5 tags = 1.5M strings, não 1.5M
  edges versionadas.
- Query "todo código com tag X" é index lookup O(log n), não traversal.
- Bitemporal: alterar tag rebumpa o nó (já é versionado), sem edge
  separada.
- `Feature` ainda é nó com hierarquia traversal completa pelo lado
  governance.

**Negativas:**
- Query "todas features implementadas pelo serviço Y" exige scan dos
  `feature_tags` de todos os filhos do Service. Mitigado por index sobre
  o atributo na store (Neo4j: `:Call(feature_tags)`; ClickHouse:
  `bloom_filter` em `Array(String)`).
- Drift detectável (tag aponta para feature inexistente) é responsabilidade
  do coletor + lint, não constraint de schema.
- Persona NÃO segue o mesmo padrão (ADR ainda implícita) — Persona só via
  `UserStory -SERVES->`. Justificativa: forçar disciplina de criar
  UserStory para mapear audiência; código legado sem story fica sem
  Persona, é diagnóstico.

## Como popular `feature_tags`

Coletor combina três fontes, em ordem de prioridade:

1. **Anotação inline em código** (comentário convencional):
   ```go
   // @feature:home-banner
   func renderHomeBanner(...) { ... }
   ```
2. **Manifest externo** (`features.yaml` no repo):
   ```yaml
   features:
     - short_id: home-banner
       owners:
         - urn:ce:code:repo:function/.../home.RenderHomeBanner
         - urn:ce:code:repo:call/http!.../handler.GetHome#3
   ```
3. **PR labels que disparam regra** (extensão de F-013).

Coletor reconcilia: tag mais específica vence (Call > Function > Module).

## Alternativas consideradas

**Edge `Code -IMPLEMENTS-> Feature`.** Rejeitada por cardinalidade.

**Persona-tag também denormalizada.** Considerada para consistência.
Rejeitada: feature é alta-frequência e baixo-overhead de manutenção
(devs marcam no código que estão escrevendo); persona é decisão de
audiência que deve estar formalizada em UserStory — denormalizar
encorajaria persona-drift sem story.

**Hierarquia inteira denormalizada (capability/domain como tags).**
Rejeitada: tags devem apontar para folha (Feature). Resolver a árvore
ancestralmente é responsabilidade da query, não da denormalização.
