---
id: ADR-005
title: Controllers de módulo não importam `repository`; exceção documentada para `app/graph/controller`
status: accepted
date: 2026-05-15
related_features: [F-016]
supersedes: []
superseded_by: []
---

# ADR-005 — Controllers não importam `repository`; `app/graph` é a única exceção

## Contexto

F-016 (Arquitetura modular v2) consolida o estilo de cada módulo do
monolito modular em três camadas:

```
modules/<X>/
  port.go     # Service interface (inbound port, consumer-owned)
  errs.go     # erros tipados que implementam HTTPProblem
  types.go    # DTOs do módulo (Page[T], queries, summaries, details)
  service/    # implementação do port (reader, writer, ...)
  controller/ # HTTP: registra rotas, parseia query, delega ao Service
```

A separação consumer/producer surge em três pontos da pilha:

1. **REST → módulo:** controller fala com `module.Service`, não com
   `repository.NodeRepository`. O port de cada módulo é narrow,
   consumer-owned, expressa só o vocabulário do bounded context.
2. **Módulo → grafo:** o service de cada módulo recebe
   `NodeRepository` e `EdgeRepository` via construtor — é o ponto
   onde a abstração se realiza.
3. **App-level (search) → grafo:** o package `app/search` define
   sua *própria* porta narrow (`NodeSearcher`), o adapter para
   `repository.NodeRepository` vive em `cmd/api/main.go` (port-set).

Quando S-008 migrou os handlers cross-kind (`/v1/architecture/nodes/{urn}`,
`/history`, `/neighbors`, `/paths`) de `app/api/v1/` para
`internal/app/graph/controller/`, o controller resultante importa
diretamente `internal/repository`. Isso viola a regra "controller
não importa repository" que foi adotada para `modules/<X>/controller`.

Surgiu a escolha: **forçar `app/graph/controller` a passar por um
Service como os demais, ou aceitar a exceção?**

## Decisão

**Aceitar `app/graph/controller` como exceção documentada à regra
"controllers não importam `repository`".** Concretamente:

1. **Regra geral.** Controllers de módulo
   (`internal/modules/<X>/controller/`) **não** importam
   `internal/repository`. Recebem por construtor uma instância de
   `module.<X>.Service` (port narrow do módulo) e operam só sobre
   ela. Lint `depguard` em `.golangci.yml` impõe.

2. **Exceção `app/graph/controller`.** `internal/app/graph/controller/`
   pode importar `internal/repository`. Razão: o "graph plane" é
   um wrapper *thin* e cross-kind sobre `NodeRepository` +
   `EdgeRepository`. Não há lógica de negócio acima do grafo, só
   tradução HTTP → repo (parsing, dispatch por sufixo, serialização
   de view). Inserir um Service intermediário criaria boilerplate
   sem benefício — todas as operações são read-only diretas do repo.

3. **Exceção `app/search`.** Direção oposta da exceção do graph:
   `app/search` **não** importa `repository`. Usa o pattern
   "port-set" — define `search.NodeSearcher` (uma única chamada,
   consumer-owned) no próprio package; o adapter de `NodeRepository`
   → `NodeSearcher` é construído em `cmd/api/main.go`. Modelo de
   referência para futuros packages `app/*` que precisem de uma
   fatia narrow do mundo de leitura.

4. **Lint enforcement.** `.golangci.yml` codifica os três casos:
   - `module_<X>` rules: denyam `internal/repository`.
   - `app_graph`: permite `internal/repository`.
   - `app_search`: nega `internal/repository`.

## Alternativas consideradas

### A — Forçar `app/graph/controller` a passar por um Service
Descartada. Criaria um `graph.Service` que reexporta literalmente
`NodeRepository.GetByURN`, `History`, `Neighbors`, `EdgeRepository.Paths`
sem agregar nada. Cada método seria um one-liner delegando ao repo.
Custo: dobra os tipos (port + impl) sem fechar um bounded context —
graph não é um bounded context, é um *aspect* polimórfico do grafo.

### B — Apagar `app/graph/controller` e distribuir os endpoints
nos módulos
Descartada. `/v1/architecture/nodes/{urn}` precisa funcionar para
qualquer Kind (Compute, Service, Team, Account, ...). Espalhar
isso em N controllers de módulo significa que cada novo Kind
exige um novo handler. O endpoint é genuinamente cross-kind;
deve viver onde é cross-kind.

### C — Aceitar a exceção (escolhida)
Pró: a regra "controllers não importam repository" continua válida
em 99% dos casos (todos os módulos de domínio). A única exceção é
o controller cuja razão de existir é ser polimórfico sobre o repo.
Documentar a exceção custa um ADR; impedir essa exceção custa
boilerplate em cada endpoint. Contra: lint `depguard` precisa de
uma cláusula explícita carve-out — feito.

## Consequências

### Positivas
- **Boilerplate zero** para handlers cross-kind. Adicionar um Kind
  novo não exige mexer em controllers.
- **Regra forte permanece forte** onde importa: módulos de domínio
  (org, code, infra) seguem sendo bounded contexts fechados, sem
  vazamento de `repository.AsOf`/`repository.SearchQuery`/etc.
- **Port-set documentado** para o caso oposto (`app/search`) —
  serve como modelo para futuros packages `app/*` que tenham
  superfície mais específica que o graph plane.

### Negativas / custo
- **Duas regras precisam ser lembradas** em vez de uma única
  "controllers não importam repo". Mitigação: `depguard` é a
  documentação executável; PR review não precisa lembrar.
- **`app/graph/controller` não tem um Service para testar em
  isolamento.** Os testes batem no `httpserver.Handler` real com
  `memory.Repo` por baixo. Aceitável: graph não tem lógica para
  testar separadamente — é só HTTP↔repo.

### Quando reabrir
- Se `app/graph/controller` começar a acumular lógica não-trivial
  (cache, agregação, autorização cross-kind), o thin-wrapper deixa
  de ser thin: vira um Service candidato. ADR superseding move
  a lógica para `app/graph/service/` e o controller passa a
  consumir um port.
- Se aparecer um terceiro caso "cross-X, sem bounded context"
  além de graph, vale extrair um padrão (talvez `app/*` ganhe
  uma camada `app/<X>/service/` opcional).

## Implementação atual

- Lint: `.golangci.yml` linhas `module_<X>`, `app_graph`, `app_search`.
- Doc executável: comentário de package em
  `internal/app/graph/controller/controller.go` cita esta ADR.
- Doc executável: package comment de `internal/app/search/port.go`
  explica o port-set.
- Atualização em `docs/architecture/04-modular.md` §3: regras
  invioláveis ganham nota sobre `app/graph` e `app/search`.
