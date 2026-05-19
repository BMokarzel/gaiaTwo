---
id: F-030
title: TypeScript code collector (Express + NestJS)
status: in-progress
modules: [code]
depends_on: [F-007, F-017, F-018, F-019, F-020, F-021, F-023]
modeling_impact: no
adrs: [ADR-012]
epic: E-003
updated: 2026-05-18
---

# F-030 — TypeScript code collector

## Problema

O collector Go (F-007) é o único que emite hoje `Service / Module /
Endpoint / Function / Call / Type / Variable`. Os pacotes `npm/` e
`python/` só extraem `Framework` (deps). Resultado: rodar `ce extract
code` num repo TypeScript produz **zero Services, zero Endpoints** — o
grafo fica vazio para esse repo e a web app não tem o que mostrar.

Para a próxima etapa do produto (visualizar o fluxo completo de um
endpoint na web), o operador precisa extrair serviços TS reais —
especialmente porque o stack interno mistura Go com TS (Node), e o
projeto de comparação (tree/apps/web) é TS.

**Para quem:** F-014 (REST architecture), web app costEngine
(produto Arquitetura), F-011 (CODEOWNERS em mono-repo TS), F-013
(Feature→Service via PR em repos TS).

**Dor sem ela:** repos TS ficam fora do grafo de código; ownership e
custo não amarram ao código TS; web app não consegue demonstrar valor
em quem opera Node.

## Escopo

**Inclui:**
- Coletor `internal/modules/code/collector/typescript/` em Go que
  spawna sidecar Node.js (`ts-morph`) e consome NDJSON via stdout.
- Sidecar TypeScript embedded via `//go:embed` (bundle único produzido
  por `esbuild`).
- Paridade de entidades com `golang.Collect`:
  - Nós: Service, Module, Endpoint, Function, Call, Type, Variable,
    Framework.
  - Edges: Contains, Invokes, Targets, Uses, Extends, Aliases,
    DependsOn, DefinedIn (legado).
- Frameworks suportados no MVP: **Express** e **NestJS**.
- Auto-detect de linguagem no CLI (`go.mod` → Go;
  `tsconfig.json`+`package.json` → TS).
- 1 Service por `package.json` (mono-repo workspaces = N Services).

**NÃO inclui:**
- Outras linguagens (Python full, Rust, Java) — features próprias.
- `.tsx` (JSX rendering, componentes React) — slice futura.
- Manifesto `costengine.yaml` para sobrescrever boundaries
  (deferido para slice de ADR-011).
- Resolução de imports cross-package (`@workspace/foo`) — MVP usa
  longest-prefix-match contra `module_path`.
- Lockfiles (`package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`).

**Precondições:**
- F-007 done (modelo Service/Endpoint/Function estável).
- F-017 done (cross-language identity: ManifestType,
  Namespace, slug).
- F-021 done (Type/Variable nodes).
- Node.js ≥18 disponível no host onde o CLI roda.

## Toque no grafo

- **Lê:** nada (extractor é produtor puro).
- **Escreve:** mesmos Kinds/Edges que `golang.Collect`, via
  `modules/code.Writer`.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** idempotente por URN; re-run de mesmo repo no mesmo
  ref produz mesmas URNs/IDs.

## Critérios de aceite

- [ ] `ce extract code --path=<repo-ts>` auto-detecta TS, spawna
      sidecar e popula grafo.
- [ ] Repo Express com 1 `package.json` + N rotas
      (`app.get('/foo')`, `router.post('/bar')`) emite 1 Service, M
      Modules, N Endpoints com `method`, `path`, `framework="express"`.
- [ ] Repo NestJS com `@Controller('users') + @Get(':id')` emite
      Endpoint com `path="/users/:id"`, `framework="nestjs"`.
- [ ] Mono-repo workspaces com 2 `package.json` emite 2 Services
      com `module_path` correto (`packages/api`, `packages/web`).
- [ ] Re-run produz mesmas URNs e edges (idempotente).
- [ ] `interface User { id: string }` emite `node.Type` com URN
      determinística; `type ID = string` emite `Aliases` edge.
- [ ] Chamadas `axios.get(url)` emitem `Call{subkind: http!}` com
      `Targets` apontando para o framework `axios`.
- [ ] Chamadas `prisma.user.findMany()` emitem `Call{subkind: db!}`.
- [ ] Sem Node no PATH: CLI falha cedo com mensagem clara
      ("install Node.js ≥18 to extract TypeScript repos").

## Decisões

| Tópico | Decisão | Justificativa |
|---|---|---|
| Sidecar lang | Node ≥18 + ts-morph | TS compiler real → type info fiável |
| Empacotamento | esbuild bundle único + `//go:embed` | distribui 1 binário Go + script JS, sem `node_modules` no user |
| IPC | NDJSON via stdout (events) + stderr (logs) | streaming, debugável (`tee`), sem deps de gRPC |
| Schema NDJSON | `{$schema:"v1", kind, ...}` versionado | sidecar/Go evoluem juntos |
| Service boundary | 1 `package.json` = 1 Service | simétrico com Go (1 `go.mod` = 1 Service) |
| Type resolution | `ts.TypeChecker` via ts-morph | única forma de Calls/Targets corretos |
| Frameworks MVP | Express + NestJS | cobre stack interno; Nest similar ao Spring (futuro) |
| Workspaces | sem resolução cross-workspace MVP | iterar quando primeiro caso pedir |

ADR-012 detalha o porquê do sidecar Node (vs. tree-sitter / WASM /
regex puro).

## Slices

- **S-030.1** Esqueleto + transporte (NDJSON, exec, embed, stub).
- **S-030.2** Service + Module + URNs.
- **S-030.3** Functions + Types + Variables.
- **S-030.4** Endpoints Express.
- **S-030.5** Endpoints NestJS.
- **S-030.6** Calls (in-process + boundary: http!, db!, mq!).
- **S-030.7** Frameworks (delega para `npm.Collect`).
- **S-030.8** CLI dispatch auto-detect.
- **S-030.9** Docs + ADR-012 + pendencias.

## Riscos

| Risco | Mitigação |
|---|---|
| Node não instalado no host | Startup check com mensagem clara; doc no F-030 |
| Sidecar lento em repo grande | NDJSON streaming + `--skip` configurável |
| ts-morph engole memória | `skipAddingFilesFromTsConfig:true`; processar por dir; aceitar lentidão MVP |
| Bundle do sidecar pesa muito | esbuild minify; aceitar ~3-5MB no binário Go |
| Default exports vs Nest decorators | Tests cobrem ambos no S-030.4/5 |
| TS dinâmico (eval, dynamic require) | Não resolvido — emit `Call{subkind: unresolved!}` |

## Pendências/futuro

- `.tsx` (componentes React, JSX) — pré-requisito da web app
  costEngine se ela for TS.
- Resolução de workspaces (`@workspace/foo` import) — slice futura.
- `costengine.yaml` override (ADR-011) — slice futura.
- Outros frameworks: Fastify, Koa, Hono, tRPC.
- Outros provedores (axios/fetch) → enriquecer matchers boundary.
