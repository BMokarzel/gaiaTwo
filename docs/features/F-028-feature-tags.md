---
id: F-028
title: Atributo denormalizado `feature_tags` em Module/Endpoint/Function/Type/Variable/Call
status: done
modules: [code]
depends_on: [F-024]
modeling_impact: yes
adrs: [ADR-009]
epic: E-008
updated: 2026-05-17
---

# F-028 — feature_tags denormalizado

## Problema

Aresta `Feature→Function` em massa explode o grafo (10⁵+ funções ×
10² features). Solução: `feature_tags []string` como atributo em
nós de código — *array denormalizado* indexável, sem aresta por par.

## Mudanças

- Campo `FeatureTags []string` adicionado em `node.Module`,
  `node.Endpoint`, `node.Function`, `node.Type`, `node.Variable`,
  `node.Call`, `node.Schema`, `node.Service`.
- Valor é `Feature.ShortID` (kebab-case, único por company).
- ContentHash inclui FeatureTags ordenado (ADR-009).
- Reconciliação: coletor lê `// @feature:<short-id>` em comentário
  ou tag em config externa; índice secundário por feature_tag para
  query rápida.

## DoD

- [x] Campo presente em todos os kinds de código.
- [x] ContentHash estável independente da ordem das tags.
- [x] Coletor Go propaga tags de `// @feature: foo, bar` (e
      `// @feature(foo)`) para Function, Type, Variable, Endpoint
      (herdado do FuncDecl pai) e Call (herdado do caller).
- [x] Sanity check em `internal/modules/code/featuretags`:
      `BuildCatalog(features)` + `Reconcile(tagged, catalog)` →
      `Report{Orphans, UniqueTags, UniqueOrphan}`. Warning, não erro
      fatal — ingest segue. Adapters `From{Service,Module,Endpoint,
      Function,Type,Variable,Call,Schema}` evitam boilerplate no caller.
