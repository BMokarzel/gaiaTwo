---
id: ADR-012
title: Coletor TypeScript via sidecar Node.js (ts-morph + NDJSON)
status: accepted
date: 2026-05-18
related_features: [F-030]
supersedes: []
superseded_by: []
---

# ADR-012 — Coletor TypeScript via sidecar Node.js

## Contexto

F-030 precisa de coletor TS com paridade total com o Go (Service,
Module, Endpoint, Function, Call, Type, Variable). Quatro opções
viáveis:

- **A. Sidecar Node + `ts-morph`** (compilador TS real)
- **B. Pure Go via `tree-sitter-typescript`** (CST, sem types)
- **C. WASM (port do `tsc` para WebAssembly)**
- **D. Regex / heurísticas puras em Go**

A pergunta central: **resolução de tipos** vale o custo operacional?

Para emitir `Call{subkind:http!|db!|mq!}` com `Targets` corretas, é
necessário saber o tipo do receptor (`axios.get` exige saber que
`axios` foi importado de `axios`; `prisma.user.findMany` exige saber
que `prisma` é instância de `PrismaClient`). TypeScript dinâmico
impossibilita inferência só por nome.

## Decisão

**Adotar sidecar Node.js + ts-morph, comunicado via NDJSON sobre
stdout/stderr, com bundle único embedded no binário Go via
`//go:embed`.**

Concretamente:

1. **Sidecar**: pacote Node em
   `internal/modules/code/collector/typescript/sidecar/`, escrito em
   TypeScript, dependência principal `ts-morph` (que embute o
   `typescript` package). Bundle final via `esbuild`
   (single-file, CommonJS, minify). Bundle vai em
   `sidecar/dist/sidecar.bundle.js`.

2. **Embed**: arquivo `embed.go` declara
   `//go:embed sidecar/dist/sidecar.bundle.js` e expõe
   `SidecarBundle() []byte`. No primeiro uso o coletor escreve o
   bundle em diretório temporário (`os.TempDir()/costengine-sidecar-vXX/`)
   e dispara `node <tmp>/sidecar.js`. Cache por hash do bundle —
   re-uso entre invocações.

3. **Pré-requisito Node**: ≥18 (LTS), descoberto via PATH. Falha de
   exec com mensagem clara: *"Node.js ≥18 not found in PATH. Install
   from https://nodejs.org. The TypeScript collector runs as a
   Node sidecar."*

4. **Protocolo NDJSON**: cada linha um JSON com `{"$schema":"v1",
   "kind":"service|module|endpoint|function|call|type|variable|
   framework|edge|done|error", ...}`. Stderr carrega logs textuais
   (não interpretáveis pelo Go — só passa por `--verbose`).

5. **Versionamento do protocolo**: campo `$schema` em cada evento.
   Sidecar e Go side checam compatibilidade no `init` event. Major
   bumps quebram, minor adiciona campos opcionais.

6. **Cancelamento**: Go fecha stdin → sidecar detecta EOF →
   shutdown ordenado. Timeout duro por evento (configurável,
   default 5min).

## Alternativas consideradas

### A — Sidecar Node + ts-morph (escolhida)
**Pró**: type-info real (only way para Call resolution correta);
ts-morph é o de-facto wrapper sobre `typescript` package; bundle
embedded mantém distribuição simples. Stack mature.

**Contra**: Node ≥18 vira dependência operacional do CLI; binário
Go cresce ~3-5MB; tem que manter dois ecossistemas (Go + Node).

### B — Pure Go via tree-sitter-typescript
**Pró**: zero runtime deps; binário único. Pegada de memória menor.

**Contra**: tree-sitter dá CST, não AST tipado. Sem type checker,
não resolve `axios.get` → framework axios; cai em heurística por
identifier-name (frágil em TS dinâmico). Reproduz a fragilidade que
o coletor Go tem hoje em call resolution. Conhecido — F-019/F-020
indicam que call extraction sem types é frágil mesmo em Go.

**Por que não**: o ganho do coletor TS é justamente type-info; pegar
o caminho que renuncia a ela aniquila o motivo de fazer F-030.

### C — WASM tsc
**Pró**: binário único Go (WASM embedded), sem Node.

**Contra**: tsc é grande (~10MB WASM); cold-start lento; debugging
e dev workflow piora bastante; ts-morph WASM não existe pronto.
Custo de manter o port WASM > custo de exigir Node no host.

**Por que não**: ROI ruim no MVP. Reabrir se distribuição
"single-binary" virar requisito duro do produto.

### D — Regex / heurísticas
**Pró**: simples, sem deps.

**Contra**: TS moderno (decorators, generics, default exports,
union types) inviabiliza regex correto. Manutenção infinita.

**Por que não**: já temos npm/python collectors no nível "regex" —
F-030 existe justamente porque eles não bastam.

## Consequências

### Positivas
- **F-030 destravado** com fidelidade ao nível do Go (e melhor em
  type resolution).
- **Web app costEngine** pode mostrar endpoint flow real em repos
  TS (objetivo imediato).
- **Padrão de extensão**: futuras linguagens podem replicar a
  mesma arquitetura (sidecar isolado, NDJSON, embed).

### Negativas / custo
- **Node ≥18 vira pré-requisito** para `ce extract` em hosts que
  rodem repos TS. Documentar em F-030.md e no `--help`.
- **Tamanho do binário Go** cresce ~3-5MB. Aceitável.
- **Manutenção de dois ecossistemas** (Go + Node deps no sidecar).
  Mitigado por bundle único (sem `node_modules` no user) e
  `package.json` fechado em ranges de patch.
- **Surface de prompt-injection** se sidecar vier do PR (não vem;
  vai embedded no binário).

### Quando reabrir
- Se distribuição "single static binary" virar requisito formal
  (alternativa C fica atrativa).
- Se tree-sitter-typescript ganhar type-info camada acima (LSP-like)
  utilizável em Go (alternativa B).
- Se mais de 2 linguagens "type-aware" entrarem (Python pyright,
  Rust rust-analyzer) — converge em padrão LSP-host?

## Referências

- F-030: docs/features/F-030-typescript-code-collector.md
- F-007: docs/features/F-007-go-ast-extractor.md (paralelo Go)
- F-019/F-020: Call boundary + in-process (motivo do type-info)
- ts-morph: https://ts-morph.com (consumido como dep do sidecar)
