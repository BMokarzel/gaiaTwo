---
id: F-007
title: Go AST extractor
status: done
modules: [code]
depends_on: []
modeling_impact: no
adrs: []
epic: E-003
updated: 2026-05-14
---

# F-007 — Go AST extractor

## Problema

O plano `code` está vazio. Sem nós `Service / Endpoint / Function /
Table`, é impossível ligar capability/feature a custo, ou time a
serviço, ou medir blast radius. Como o stack interno é majoritariamente
Go, começar com um extractor Go cobre a maior parte do território.

**Para quem:** futuro F-009 (bridge Service↔Compute), F-011
(CODEOWNERS), F-013 (Feature↔Service via PR), produto Arquitetura
(F-014).

**Dor sem ela:** custo, ownership e capability ficam "soltos" sem
nós de código para ancorar.

## Escopo

**Inclui:**
- Comando CLI `ce extract code --repo=<path>` que percorre um
  repositório Go local e produz:
  - 1 nó `Service` por módulo Go (raiz com `go.mod`).
  - Nós `Endpoint` para handlers HTTP detectados em routers comuns
    (`net/http`, `chi`, `gin`, `echo`).
  - Nós `Function` para funções exportadas em pacotes "service" /
    "handler" / "usecase".
  - Edge `DefinedIn` (Endpoint/Function → Service).
- Idempotência por `(repo_url, file_path, symbol_name)` no `external_id`.
- Upsert bitemporal via `NodeRepository`.

**NÃO inclui:**
- Outras linguagens (TypeScript, Python — features próprias).
- Tabela SQL via AST (vive em F-008 / OpenAPI ou feature dedicada).
- Análise de dependências entre serviços (futuro).
- Ligação Service↔Compute (F-009).

**Precondições:**
- F-002 done (bitemporal upsert).
- Repositório acessível localmente (CLI roda no host do operador).

## Toque no grafo

- **Lê:** nada (extractor é puro produtor).
- **Escreve:** `Service`, `Endpoint`, `Function` (Kinds do plano `code`)
  + edges `DefinedIn` via `internal/modules/code`.
- **Novos Kinds/edges:** nenhum (Kinds já modelados em `01-modeling.md`).
- **Bitemporal:** primeira execução cria v1; re-run com mudanças no
  AST fecha versão e cria nova; hash determinístico para idempotência.

## Critérios de aceite

- [x] Dado repo Go com 1 `go.mod`, handlers HTTP em pacotes
      `*service*/*handler*/*usecase*` e funções exportadas, quando
      rodar `ce extract code --repo=<name> --path=<dir>`, então o
      grafo contém 1 Service por `go.mod`, Endpoints detectados,
      Functions exportadas e edges `DEFINED_IN`.
- [x] Cada `Endpoint` carrega `method`, `route`, `handler` e
      `framework` ("net/http" ou "chi"). URN inclui `METHOD:route`.
- [x] Cada `Function` carrega `package`, `symbol`, `signature_hash`
      e `signature` canônica. Métodos viram `(T).Method`.
- [x] Re-run produz mesmas URNs/edge IDs (idempotência testada em
      `integration_test.go::TestIntegration_Idempotent`).
- [x] Renomear função: hash da URN muda → tratada como "nova". MVP
      não correlaciona renames; aceito (risco já documentado).
- [x] Comando reporta totais por kind ao final.

## Decisões

- **D1 — Reuso do slot `<account>` na URN para `<repo>`**. Em vez de
  refatorar `ParseURN`, code plane usa `ProviderCode = "code"` e o
  segundo slot da URN guarda o nome do repo. Mantém parser uniforme.
- **D2 — Separador `!` entre `<service-module-path>` e o
  `(method:route)` / `(pkg.symbol)`**. `ParseURN` faz `SplitN("/", 2)`
  no segmento final; usar `/` dentro do id geraria ambiguidade.
  `!` é livre nesse alfabeto.
- **D3 — Filtro por substring de nome de pacote (default:
  `service`/`handler`/`usecase`)**. Conservador por design — falsos
  negativos são preferíveis a poluir o grafo. Configurável via
  `Config.IncludePkgSubstrings`.
- **D4 — Sem `golang.org/x/mod/modfile`**. Lemos só a primeira linha
  `module …` do `go.mod` com `bufio.Scanner`. Mantém o módulo sem
  deps externas além da stdlib.
- **D5 — Submódulos pulam o pai**. Quando o walker encontra outro
  `go.mod` em subdir, esse subdir vira Service independente e
  *também* é pulado pelos extractors do Service pai (não duplica
  funções/endpoints).
- **D6 — Detecção de framework**: `net/http` (HandleFunc/Handle) +
  go-chi (`Get/Post/Put/Delete/Patch/Head/Options/Connect/Trace/
  Method/MethodFunc`). `gin`/`echo` ficam para iteração futura.
- **D7 — Routes dinâmicas (variáveis) são descartadas**. O AST
  precisa ver string literal — match estritamente sintático. URNs
  inferidas dinamicamente quebrariam idempotência.

## Verificação

- `go test ./internal/modules/code/...`  — collector + writer + integração.
- `go test ./cmd/cli/...`  — smoke do subcomando `ce extract code`.
- `go test ./internal/entity/node/...`  — URN constructors do code plane.

## Riscos / incerteza

- **Detecção de handler HTTP é heurística.** `chi.HandleFunc(...)`
  é fácil; padrões customizados (`router.Mount`, middlewares
  envolvendo handler) podem escapar. **MVP:** suportar `net/http`
  e `chi`; documentar limitação para os outros.
- **Renames vs deletes.** Se hash mudou, é "novo" — não capturamos
  rename real. Aceitar perda de continuidade no MVP.
- **Mono-repo com vários módulos.** Iteração por `go.mod`; cada um
  vira um Service.
- **Pacotes gerados (gen, mock).** Filtro padrão por `//go:generate`
  e diretórios `gen/`, `mocks/`.

## Notas de implementação

- Pacote em `internal/modules/code/collector/golang/`.
- Usar `go/parser` + `go/ast` da stdlib.
- Para handlers, percorrer chamadas para identificar registros de rota.
- Externalizar regras de filtro em config para iterar sem deploy.
