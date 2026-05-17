---
id: F-017
title: Migração de identidade Service/Endpoint/Function para cross-language
status: entity-layer done; collector Go done; collectors Python/TS pendentes
modules: [code]
depends_on: [F-007]
modeling_impact: yes
adrs: [ADR-006]
epic: E-007
updated: 2026-05-17
---

# F-017 — Identidade cross-language

## Problema

A modelagem F-007 ancorava identidade de Function em conceitos
exclusivos de Go (`Package` literal, `go.mod` como manifesto único).
Para suportar serviços poliglotas (Go + Python + TS no mesmo grafo),
identidade precisa ser linguagem-agnóstica.

## Mudanças

- `Function.Package` → `Function.Namespace` (string canônica
  cross-language, ADR-006). Go: `<go.mod>/<rel-pkg-path>`. Python:
  `<pkg>.<sub>`. TS: `<pkg>/<sub>`.
- `Function.Symbol` substitui forma anterior. Métodos viram
  `(Type).Method` / `(*Type).Method`.
- `Service.GoModule` deprecated. Adicionado `Service.Manifest`
  (`go.mod`, `package.json`, ...) + `Service.ManifestType`. `Namespace`
  herda do manifest.
- `Location{File, LineInit, LineEnd, ColInit, ColEnd}` em
  Function/Endpoint como atributo — fora do ContentHash (mover código
  não rebumpa versão).
- URN nova: `urn:ce:code:<repo>:function/<service-module-path>!<namespace>!<symbol>`
  (antes: `<service-module-path>!<pkg>.<symbol>`).
- Coletor Go (`internal/modules/code/collector/golang`) gera Namespace
  corretamente; campos `Package`/`File`/`Line` mantidos como
  deprecated mirrors durante migração.

## DoD

- [x] `internal/entity/node/{service,endpoint,function,location}.go`
      atualizados.
- [x] `code_urn_test.go` cobre URN nova.
- [x] Coletor Go produz Namespace cross-language e Location estruturada.
- [x] Tests existentes do collector Go passam após adaptar assinatura
      `ExtractFunctions(... goModule ...)`.
- [ ] Coletores Python/TS — backlog separado.
