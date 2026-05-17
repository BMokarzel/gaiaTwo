---
id: F-020
title: Call in-process (FunctionCall, MethodCall)
status: done (coletor Go)
modules: [code]
depends_on: [F-019]
modeling_impact: yes
adrs: [ADR-007, ADR-008]
epic: E-007
updated: 2026-05-17
---

# F-020 — Call in-process

## Problema

Para reconstruir caminho de execução completo dentro de um service e
suportar simulação de blast radius / refactor, callsites in-process
também são nós da família Call.

## Mudanças

- Subkinds `function` e `method` no enum `CallKind` (ADR-007).
- `MethodCall.IsDynamic=true` quando dispatch via interface
  não-resolvível estaticamente.
- Coletor Go produz `TARGETS` resolvido quando possível, vazio quando
  dinâmico — sem erro.

## DoD

- [x] Enum `CallFunctionCall`, `CallMethodCall` em `node/call.go`.
- [x] Coletor Go (`internal/modules/code/collector/golang/call.go`,
      `tryInProcess`) popula MethodCall em chamadas qualificadas
      (`recv.Method(...)` / `pkg.Func(...)`) e FunctionCall em
      chamadas diretas (`Helper(...)`) quando o símbolo bate em uma
      Function indexada.
- [x] `TargetURN` preenchido + edge `TARGETS` quando há resolução única.
      Múltiplos matches → `IsDynamic=true`, sem `TARGETS`.
      Confidence=0.7 (sintático, sem type-resolve).
- [x] Tests:
      `TestExtractCalls_InProcessMethodCall` (svc.UC.Process resolvido),
      `TestExtractCalls_InProcessAmbiguousIsDynamic` (interface dispatch),
      `TestExtractCalls_InProcessFunctionCall` (`Helper()` direto).

## Notas

Resolução estritamente por **nome curto** do método (`bySymbol`). Em
codebases grandes, colisões são esperadas: tratamos como dispatch
dinâmico (IsDynamic) sem inventar TARGETS. F-021/type-resolve futuro
desambigua via receiver type.

Chamada qualificada `pkg.Func(...)` ainda é classificada como MethodCall
(não FunctionCall) — distinguir requer type-resolve (ident é pacote ou
variável?). Não bloqueante: queries por `Kind=function|method` ainda
funcionam; uma futura migração reclassifica via pass de type-resolution.
