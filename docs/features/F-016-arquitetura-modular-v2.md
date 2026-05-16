---
id: F-016
title: Arquitetura modular v2 (controllers por módulo, httpserver compartilhado, erros tipados, port-sets)
status: done
modules: [platform, org, code, infra, bridge, app]
depends_on: [F-014, F-015]
modeling_impact: no
adrs: [ADR-001, ADR-005]
epic: E-006
updated: 2026-05-16
---

> **Status final (2026-05-16):** entregue. Desvios conscientes vs.
> plano original:
>
> - **S-006 / S-007 absorvidos por S-008.** `modules/code` e
>   `modules/infra` receberam `port.go` + `errs.go` + `types.go` +
>   `service/` (preparação completa do bounded context), mas **não
>   ganharam `controller/` próprio**: não existem endpoints REST
>   específicos `/v1/code/*` ou `/v1/infra/*` hoje. Os Kinds desses
>   módulos são acessados via `app/graph/controller` (cross-kind) e
>   `app/search` (port-set). Quando aparecer endpoint de domínio
>   próprio, criar `modules/<X>/controller/` seguindo o playbook de
>   `modules/org/controller/`.
> - **S-010 simplificado**: `.golangci.yml` ativo com `depguard`
>   cobrindo as fronteiras de §3 do `04-modular.md`. `internal/controller/`
>   (placeholder vazio) ainda existe — não bloqueia, pendente de
>   limpeza trivial.
> - Catálogo de erros entregue em `docs/api/v1/errors.md`.
> - `04-modular.md §9` atualizado: passos 4-6 marcados como executados;
>   passos 1-3 (mover `entity/*` → `core/*`) seguem pendentes em
>   feature separada registrada em `docs/backlog/pendencias.md`.

# F-016 — Arquitetura modular v2

## Problema

Hoje `internal/app/api/v1/` é um *god server*: um único `Server` recebe
`NodeRepository`/`EdgeRepository` direto, monta todas as rotas, e
concentra ~1.350 linhas em `nodes.go` + `org.go` + `search_paths.go`. O
módulo `internal/modules/org/service` existe mas a API **bypassa** essa
camada — fala com o repositório direto. Há *drift* entre o que
`docs/architecture/04-modular.md §6` promete ("CLI e API são dois rostos
do mesmo cérebro, ambos consomem `modules/*/service`") e o código real.

Três sintomas concretos:

1. **Erros são `if errors.Is(...)` espalhados.** `writeRepoError` em
   `app/api/v1/errors.go` mapeia 4 sentinelas para HTTP. Não há lugar
   para erros tipados de domínio (`ErrCycleDetected{Path}`,
   `ErrInvalidURN{URN, Reason}`) com body específico — quem precisa
   adiciona um `switch` ad hoc no handler.
2. **Testes de handler arrastam o grafo inteiro.** `org_test.go` monta
   `repository.NodeRepository` + `EdgeRepository` para validar um GET
   simples. Não dá pra mockar só "o que team faz" — não existe essa
   abstração.
3. **Features cross-plane futuras (`app/search`, `app/simulate`) não
   têm padrão.** Hoje cada uma teria que importar repositórios e
   reinventar fan-out, partial failure, paginação federada.

**Para quem:** time de plataforma (mantém a base), futuros consumidores
de features transversais, e o próprio projeto no dia da extração de
microserviços.

**Dor sem ela:** cada feature nova em `app/api/v1` aumenta o custo da
reorganização. F-014 + F-015 já entregaram ~1.350 LOC nessa pasta;
F-017+ tendem a piorar.

## Escopo

**Inclui:**

- `platform/httpserver/` — pacote comum com `Server`, `Registrar`
  interface, middlewares (RequestID/Recover/Auth) migrados, helpers
  `WriteJSON`/`WriteError`/`ParseAsOf`/`ParseCursor`.
- `core/errs/` — `HTTPProblem` interface, `Problem` (RFC 7807-like),
  `Render(err) (int, Problem)` com fallback para sentinelas do
  `repository` existentes.
- **Erros tipados por módulo** — cada módulo declara seus erros como
  structs concretas implementando `HTTPProblem`, com `Unwrap` para
  sentinel correspondente. Service deixa de retornar sentinel cru.
- **Uma interface `Service` por módulo** (port única, agrega entidades).
  Implementação única em `modules/<X>/service/`.
- **Controller único por módulo** em `modules/<X>/controller/`,
  recebendo a `Service` por construtor. Arquivos por path para
  legibilidade, mas mesmo pacote, mesma struct.
- **Migração dos handlers atuais** de `internal/app/api/v1/` para os
  respectivos `modules/<X>/controller/` (org, code, infra) e
  `app/graph/controller/` (endpoints cross-kind).
- **Port-set pattern em `app/search/`** como referência viva: inbound
  port `Engine`, outbound ports estreitas por plano
  (`CodeSearcher`/`InfraSearcher`/`OrgSearcher`), adapters in-proc em
  `cmd/api/main.go`, partial failure no contrato.
- **Lint `depguard`** travando regressão das regras de fronteira do
  `04-modular.md §3`.
- **ADR-005** documentando o padrão (controllers, ports, erros).

**NÃO inclui:**

- Mover `internal/entity/*` para `internal/core/*` (passos 1-3 do plano
  do `04-modular.md §9`). Trabalho paralelo, fora deste escopo.
- Criar módulos novos (`cost`, `code/extract` além do que já existe).
- Substituir `net/http` mux por chi/gorilla.
- Introduzir fila/eventos. Cooperação cross-plane permanece síncrona
  via outbound ports.
- Migrar `cmd/cli` (oportunidade em S-010, não exigência).
- GraphQL/gRPC. REST se mantém como única interface externa.

**Precondições:**

- F-014 e F-015 `done` (são os handlers a migrar).
- `internal/modules/org/service`, `code/service`, `infra/service` já
  existem (verificado em 2026-05-15).
- Pasta `internal/controller/` está vazia (placeholder a remover).

## Toque no grafo

- **Lê:** nada novo. Mantém os reads atuais via repositórios.
- **Escreve:** nada novo.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** sem mudança. `?as_of=t` continua suportado pelos
  endpoints que já tinham.
- **Modelagem:** sem impacto. É refactor estrutural puro.

## Critérios de aceite

- [ ] `internal/app/api/v1/` não existe ao fim.
- [ ] Todo handler HTTP mora em `modules/<X>/controller/` ou
      `app/<feature>/controller/`.
- [ ] Toda resposta 4xx/5xx tem `Content-Type: application/problem+json`
      e `code` namespaced (`org.team.not_found`, `infra.resource.not_found`,
      etc.).
- [ ] Suite atual (`go test ./...`) verde sem mudança *funcional* nos
      asserts; apenas adaptação de localização e shape de erro.
- [ ] `cmd/api` builda; smoke `curl /v1/teams`, `curl /v1/architecture/nodes/...`,
      `curl /v1/search?q=foo` retornam payload esperado.
- [ ] Existe ≥ 1 feature em `app/` (search) consumindo outbound ports
      como referência viva do padrão.
- [ ] `depguard` está verde no CI e bloqueia:
      - `core/*` importar `modules/*`/`app/*`/`cmd/*`/`platform/*`;
      - `platform/*` importar `modules/*`/`app/*`/`cmd/*`;
      - `modules/<X>/*` importar `modules/<Y>/*` (exceto `bridge`);
      - `modules/*/controller/*` importar `repository` direto.
- [ ] ADR-005 mergeado.
- [ ] `docs/architecture/04-modular.md §9` atualizado refletindo a
      execução parcial deste plano (passos 4–6).

## Decisões (implementação)

- **D1 — Uma `Service` por módulo, não uma por entidade.** O bounded
  context é o módulo (`org`). Entities (`Team`, `Squad`, `Person`) são
  value types. A interface `org.Service` agrega todas as operações; a
  segregação vem dos *métodos*, não de múltiplas interfaces. Razão:
  operações que cruzam entidades (`PromoteToSquadLead`, `MoveSquadToTeam`)
  precisam de lugar natural; transações entre entidades exigem unidade
  única. Interface Segregation aplica-se ao **consumidor** (outbound
  ports, narrow), não ao produtor.

- **D2 — Entities são structs, não interfaces.** Sem `TeamInterface`,
  `TeamNode`, `IPerson`. Campos exportados são o "contrato"; getters
  artificiais não desacoplam nada em Go. Polimorfismo entre Kinds já
  existe em `core/node.Node` — não duplicar por kind.

- **D3 — Escola A vs Escola B por tamanho.** Módulos pequenos (`org` ~3
  entities) usam **flat** (`modules/org/types.go`). Módulos grandes
  (`code` 10+ kinds, `infra` 8+ kinds) usam **sub-pacote `node/`**
  conforme `04-modular.md §2`. Critério: ≥ 5 tipos → sub-pacote.

- **D4 — `HTTPProblem` interface + `errors.As` na borda.** Erros são
  tipados por módulo (`org.ErrTeamNotFound{URN}`), implementam
  `HTTPProblem` (`HTTPStatus()`, `Code()`, `Details()`), e fazem
  `Unwrap()` para o sentinel correspondente do `repository`. O
  `httpserver.WriteError` chama `errs.Render`, que faz `errors.As` na
  cadeia. Service nunca menciona HTTP; controller é dumb.

- **D5 — Shape RFC 7807-ish (breaking change controlado).** Body de
  erro vira `{type, title, status, code, detail, ...extras}`. Diferente
  do atual `{error: {code, message}}`. Como não há clients externos
  versionados conhecidos (MVP), aplicar direto a partir de S-005. Se
  aparecer cliente antes, flag `httpserver.LegacyErrorBody` na config
  (default false) — remover em S-011.

- **D6 — `Registrar` aceita `*http.ServeMux` concreto.** Não abstraímos
  o roteador. Trocar `net/http` para chi seria *outra* feature (sem
  demanda hoje). `Server` registra healthz + catch-all 404 **depois**
  dos `Registrar.Register` para preservar ordem.

- **D7 — Controllers consumem `Service` do próprio módulo.** Nada de
  controller importando outro módulo. Operação cross-plane vive em
  `app/<feature>/controller/`, não em `modules/<X>/controller/`.

- **D8 — Outbound ports são owned pelo caller.** `app/search` declara
  `CodeSearcher`, `InfraSearcher`, `OrgSearcher` em `app/search/port_out.go`.
  Os módulos NÃO conhecem `app/search`; satisfazem as ports via
  structural typing (a `Service` do módulo tem `Search(ctx, q)` que
  bate). Adapters in-proc moram em `cmd/api/main.go` quando os tipos
  divergem.

- **D9 — Partial failure no contrato desde o dia 1.** `PlaneResults
  { Hits []Hit; Degraded bool; Reason string }`. Hoje `Degraded` é
  sempre `false`. Amanhã (microservice), timeout do `code` retorna
  `{Hits: nil, Degraded: true, Reason: "code: timeout"}`. Search
  decide entre 200-com-aviso e 503. Custo zero hoje, evita reescrita.

- **D10 — Context em todo método de port + sem ponteiros para tipos
  internos de outro módulo.** Preparação para gRPC sem retoque: timeout
  por RPC funciona, e tipos de domínio cruzando módulo via outbound
  port são serializáveis (value types, sem ponteiro a `*node.Node` do
  outro pacote).

- **D11 — Slice piloto ANTES de replicar.** S-004/S-005 fazem `org`
  inteiro. Só depois S-006/S-007 replicam para `code`/`infra`. Se o
  piloto sentir amargo (ergonomia, boilerplate), parar e iterar no
  design **antes** de replicar — risco de criar três versões ruins.

- **D12 — Re-exports temporários permitidos em S-003.** Para não
  quebrar imports de teste durante a transição, `app/api/v1/cursor.go`
  e correlatos viram aliases finos para `platform/httpserver`. Removidos
  em S-010.

- **D13 — `app/search` é referência, não exigência funcional.** A
  feature de busca textual federada já é desejável (`F-014` faz busca
  por módulo, mas não cross-plane), mas seu valor primário aqui é
  **estabelecer o template** que `app/simulate` e `app/cost-of-service`
  vão seguir. Sem ele, cada feature transversal futura reinventa o
  fan-out.

- **D14 — Lint trava regressão.** Sem `depguard` em CI, as fronteiras
  desenhadas em ADR-001/ADR-005 viram convenção social — duram 6 meses.
  S-010 inclui regra ativa que falha o build.

## Verificação

```bash
# Build + suite completa
go build ./...
go test ./...

# Lint de fronteira
golangci-lint run

# Smoke local (memory backend)
go build -o bin/ce-api ./cmd/api
./bin/ce-api --addr=:8080 &

# Endpoint de cada módulo
curl -s localhost:8080/v1/teams
curl -s localhost:8080/v1/architecture/nodes/urn:ce:aws:1:compute/i-0abc

# Endpoint cross-plane (S-009)
curl -s "localhost:8080/v1/search?q=payments&kind=Service"

# Shape de erro RFC 7807
curl -s -i localhost:8080/v1/teams/urn:ce:org:team:inexistente
# HTTP/1.1 404 Not Found
# Content-Type: application/problem+json
# {"type":"ce:err:org.team.not_found","title":"Team not found",
#  "status":404,"code":"org.team.not_found",
#  "urn":"urn:ce:org:team:inexistente"}

# Partial failure (mockando code lento em test)
curl -s "localhost:8080/v1/search?q=foo"
# {"hits":[...],"planes":{"code":{"degraded":true,"reason":"timeout"}}}
```

## Slices

### S-001 — `core/errs` (fundação de erros)

**Objetivo:** introduzir `HTTPProblem`, `Problem`, `Render` sem consumir
em lugar nenhum ainda.

**Entregas:**
- `internal/core/errs/problem.go` — struct `Problem` com `MarshalJSON`
  que achata `Extras` em campos top-level.
- `internal/core/errs/http_problem.go` — interface `HTTPProblem`.
- `internal/core/errs/render.go` — `Render(err) (status int, body Problem)`
  com:
  - primeiro tenta `errors.As(err, &HTTPProblem)` na cadeia inteira;
  - fallback via `errors.Is` para sentinelas conhecidos do `repository`
    (`ErrNotFound`→404, `ErrInvalidArgument`→400, `ErrConflict`→409,
    `ErrAmbiguous`→409);
  - último fallback: 500 + `code: "internal"`, mensagem genérica.
- Testes: erro tipado → status correto; sentinel embrulhado com
  `fmt.Errorf("...: %w", ...)` é detectado; múltiplos `HTTPProblem` na
  cadeia → o mais interno vence; `nil` panic-protegido.

**Critério de pronto:**
- [ ] `go test ./internal/core/errs/...` verde, cobertura ≥ 85%.
- [ ] Nenhum outro pacote importa `core/errs` ainda.
- [ ] `go vet`, `gofmt` limpos.

**Não inclui:** uso em handler real (vem em S-005).

---

### S-002 — `platform/httpserver` (server compartilhado)

**Objetivo:** extrair o `Server` genérico e migrar middlewares.

**Entregas:**
- `internal/platform/httpserver/server.go`:
  ```go
  type Config struct {
      TenantRequired bool
      Logger         *slog.Logger
  }
  type Registrar interface{ Register(mux *http.ServeMux) }
  func New(cfg Config, regs ...Registrar) *Server
  func (s *Server) Routes() http.Handler
  ```
- `internal/platform/httpserver/middleware.go` — RequestID, Recover,
  Auth portados de `app/api/v1/middleware.go`.
- `internal/platform/httpserver/render.go` — `WriteJSON`,
  `WriteError(w, r, err)` (delega para `errs.Render`).
- `internal/platform/httpserver/cursor.go` — cursor opaco
  base64+fingerprint portado.
- `internal/platform/httpserver/context.go` — `ParseAsOf`, `TraceID`,
  helpers de tenant.
- `Server` registra `/healthz` e catch-all `/` (404 JSON) **depois** dos
  `Registrar.Register` para preservar precedência.
- Testes: server vazio → 404 JSON; healthz OK; middlewares em ordem;
  cadeia preserva trace ID; cursor roundtrip; cursor de query diferente
  é rejeitado.

**Critério de pronto:**
- [ ] `go test ./internal/platform/httpserver/...` verde.
- [ ] Nenhum import vindo de `internal/app/...` ou `internal/modules/...`.
- [ ] `app/api/v1` continua compilando e testando sem mudança (S-003
      ainda não tocou).

**Não inclui:** plug no v1 atual (S-003).

---

### S-003 — Plug `httpserver` no `app/api/v1` atual

**Objetivo:** `app/api/v1` passa a delegar para `platform/httpserver`,
sem mover handlers. Zero mudança de comportamento.

**Entregas:**
- `internal/app/api/v1/server.go` reescrito: `NewServer` cria um
  `httpserver.Server` e registra um Registrar único que adapta o
  `registerNodeRoutes`/`registerSearchPathsRoutes`/`registerOrgRoutes`
  existentes.
- Middlewares deletados de `app/api/v1/middleware.go` — passam a vir
  de `httpserver`.
- `app/api/v1/cursor.go`, `errors.go`, `context.go`: viram aliases finos
  (re-exports) apontando para `httpserver`. **Não deletar ainda** —
  testes podem importar.
- Pre-existing `writeRepoError` mantém comportamento; mas agora chama
  `httpserver.WriteError` internamente, que chama `errs.Render`. Para
  preservar shape antigo neste slice, `errs.Render` cai no fallback
  de sentinels — body fica `application/problem+json` mas com fields
  semanticamente equivalentes. **Aceitar essa mudança de shape aqui**
  (consistente com D5) ou guardar atrás de `cfg.LegacyErrorBody=true`
  (default false). Decisão do PR.

**Critério de pronto:**
- [ ] `go test ./internal/app/api/v1/...` verde com alterações **apenas**
      em snapshots/asserts de erro (se D5 aplicada agora) ou zero
      alteração (se LegacyErrorBody=true).
- [ ] `cmd/api` builda e responde `curl /healthz`, `curl /v1/teams`.
- [ ] Nenhum middleware duplicado entre `app/api/v1` e `httpserver`.

**Risco:** ordem de registro do catch-all. Mitigação: teste já existente
em `server_test.go` para 404 deve passar inalterado.

---

### S-004 — Piloto `org` (parte 1): port + errs + service adapter

**Objetivo:** preparar `modules/org` para receber o controller, **sem
mover o controller ainda**. Permite revisar a interface antes do move
grande.

**Entregas:**
- `internal/modules/org/port.go`:
  ```go
  type Service interface {
      ListTeams(ctx, ListTeamsQuery) (Page[Team], error)
      GetTeam(ctx, urn) (Team, error)
      ListSquadsOfTeam(ctx, teamURN, ListSquadsQuery) (Page[Squad], error)
      GetSquad(ctx, urn) (Squad, error)
      ListSquadMembers(ctx, squadURN, ListMembersQuery) (Page[Person], error)
      GetPerson(ctx, urn) (Person, error)
      ListReports(ctx, personURN, depth) (ReportsTree, error)
      Search(ctx, SearchQuery) (SearchResults, error)
  }
  ```
- `internal/modules/org/types.go` (Escola A — flat, ver D3):
  `Team`, `Squad`, `Person`, queries, results, `Page[T]` se ainda não
  existir em `core/`.
- `internal/modules/org/errs.go`: `ErrTeamNotFound{URN}`,
  `ErrSquadNotFound{URN}`, `ErrPersonNotFound{URN}`,
  `ErrInvalidURN{URN, Reason}`, `ErrCycleDetected{Path}`. Cada um:
  `Error()`, `HTTPStatus()`, `Code()`, `Details()`, `Unwrap()`.
- `internal/modules/org/service/service.go` adaptada:
  - Métodos renomeados/reorganizados para satisfazer `org.Service`.
  - Retorna erros tipados em vez de sentinels.
  - Mantém testes unitários verdes (adaptar asserts para `errors.As`).
- Controller **ainda não migrado** — `app/api/v1/org.go` continua no
  lugar. Esta slice é puramente preparatória.

**Critério de pronto:**
- [ ] `go test ./internal/modules/org/...` verde.
- [ ] `org.Service` é a única interface exportada em `modules/org/port.go`.
- [ ] Nenhum método retorna `repository.ErrXxx` cru.
- [ ] `app/api/v1/org.go` segue funcionando (continua consumindo repos).

---

### S-005 — Piloto `org` (parte 2): controller move + testes relocados

**Objetivo:** completar o piloto. Aqui é onde o padrão paga ou não paga.

**Entregas:**
- `internal/modules/org/controller/controller.go`:
  ```go
  type Controller struct{ svc org.Service }
  func New(svc org.Service) *Controller
  func (c *Controller) Register(mux *http.ServeMux) { ... }
  ```
- Arquivos por path para legibilidade, mesmo pacote: `teams.go`,
  `squads.go`, `people.go`. Mesma struct, mesma `Service` injetada.
- Handlers usam `httpserver.WriteError(w, r, err)` — sem `switch` de
  erro.
- `cmd/api/main.go` instancia `org.NewService(nodes, edges)` e
  `orgctrl.New(orgSvc)`, passa para `httpserver.New(cfg, orgCtrl, ...)`.
- `internal/app/api/v1/org.go` **deletado**.
- Testes movidos: `org_test.go`, `org_integration_test.go`,
  `teams_test.go`, `squads_test.go`, `people_test.go`, `reports_test.go`
  → `internal/modules/org/controller/`. Adaptação mínima:
  - Substituir `NewServer(nodes, edges, cfg)` por
    `httpserver.New(cfg, orgctrl.New(org.NewService(nodes, edges)))`.
  - Asserts de erro: `error.code` agora é `org.team.not_found` (era
    `not_found`). Atualizar snapshots.

**Critério de pronto:**
- [ ] Todos os testes de org verdes no novo local.
- [ ] `internal/app/api/v1/org.go` não existe.
- [ ] `curl /v1/teams/urn:ce:org:team:inexistente` retorna
      `application/problem+json` com `code: org.team.not_found` e `urn`
      no body.
- [ ] Diff de resposta JSON revisado em PR (esperado: `code`
      namespaced; `urn` virou top-level extra; demais campos idênticos).

**Gate de decisão pós-S-005:** se a ergonomia do padrão estiver ruim
(testes ficaram piores, controller ficou inchado, etc.), **parar e
iterar** antes de S-006/S-007. Documentar problemas no próprio PR.

---

### S-006 — Replicar para `code`

**Objetivo:** aplicar o playbook validado em S-004/S-005 para o módulo
`code`.

**Entregas:**
- `internal/modules/code/{port.go, errs.go, types.go}` (ou
  `node/` se ≥ 5 tipos — Escola B).
- `internal/modules/code/service/` adaptado para satisfazer `code.Service`.
- `internal/modules/code/controller/` com handlers de `nodes.go`
  específicos a Kinds de code (`Service`, `Endpoint`, `Function`,
  `Method`, `Class`, etc.).
- Wiring em `cmd/api/main.go`.
- Testes de `nodes_test.go` referentes a Kinds de code → relocados.

**Critério de pronto:**
- [ ] Suite de code verde no novo local.
- [ ] Endpoints `GET /v1/architecture/nodes/{urn-code}` respondem.

---

### S-007 — Replicar para `infra`

**Objetivo:** idem para `infra`.

**Entregas:**
- `internal/modules/infra/{port.go, errs.go, node/}` (Escola B esperada
  — `Compute`, `Persistence`, `Network`, `Messaging`, `Provider`,
  `Account`, `Region`, `Zone`, `Environment` ≥ 5 tipos).
- `internal/modules/infra/service/` satisfaz `infra.Service`.
- `internal/modules/infra/controller/` absorve handlers de `nodes.go`
  para Kinds de infra.
- Testes relocados.

**Critério de pronto:**
- [ ] Suite de infra verde no novo local.
- [ ] Endpoints `GET /v1/architecture/nodes/{urn-infra}` respondem.

---

### S-008 — `app/graph/controller` (handlers cross-kind)

**Objetivo:** mover endpoints que **não pertencem a um plano só** para
`app/graph/`. São endpoints de "grafo cru" (`/v1/architecture/*`) que
operam sobre qualquer Kind.

**Entregas:**
- `internal/app/graph/controller/` com:
  - `nodes.go` — `GET /v1/architecture/nodes/{urn}` polimórfico (faz
    dispatch para `code.Service` ou `infra.Service` ou `org.Service`
    conforme Kind extraído do URN, ou usa `NodeRepository` direto se
    for puro "leia o nó").
  - `neighbors.go` — `GET /v1/architecture/nodes/{urn}/neighbors`.
  - `history.go` — `GET /v1/architecture/nodes/{urn}/history`.
  - `paths.go` — `GET /v1/architecture/paths` (movido de
    `search_paths.go`).
- Decisão: graph consome `repository.NodeRepository`/`EdgeRepository`
  diretamente (é o que ele precisa — não tem lógica de negócio acima do
  grafo). **Exceção explícita à regra "controller não importa
  repository"** — documentar no ADR-005: graph é "thin wrapper sobre
  repo", os endpoints de domínio é que vão pelos modules' services.
- Restante de `app/api/v1/nodes.go` e `search_paths.go` deletado.
- Testes relocados: `nodes_test.go` (parte cross-kind),
  `search_paths_test.go`, `bitemporal_test.go` → `app/graph/controller/`.

**Critério de pronto:**
- [ ] `internal/app/api/v1/` contém apenas `doc.go` (ou está vazio).
- [ ] Suite de graph verde.
- [ ] Endpoints de `F-014` (`/v1/architecture/*`) continuam funcionando.

---

### S-009 — `app/search` com port-set (referência viva)

**Objetivo:** introduzir o padrão de port-set cross-plane. Esta é a
feature transversal-template.

**Entregas:**
- `internal/app/search/`:
  - `port_in.go`:
    ```go
    type Engine interface {
        Search(ctx, Query) (Results, error)
    }
    ```
  - `port_out.go`:
    ```go
    type CodeSearcher  interface { SearchCode (ctx, PlaneQuery) (PlaneResults, error) }
    type InfraSearcher interface { SearchInfra(ctx, PlaneQuery) (PlaneResults, error) }
    type OrgSearcher   interface { SearchOrg  (ctx, PlaneQuery) (PlaneResults, error) }
    ```
  - `types.go` — `Query`, `Results`, `Hit`, `PlaneResults{Hits, Degraded, Reason}`.
  - `service.go` — fan-out paralelo com `errgroup`, timeout por plano
    (default 2s), merge por score, paginação federada com cursor opaco
    que codifica offsets por plano internamente.
  - `controller/controller.go` — endpoint `GET /v1/search?q=...&kind=...`.
- Cada módulo (`org`, `code`, `infra`) ganha um método `Search` na sua
  `Service`. `org.Service.Search` já foi declarado em S-004; `code` e
  `infra` ganham em S-006/S-007 ou neste slice se preferir.
- Adapters in-proc em `cmd/api/main.go`:
  ```go
  type inProcCodeSearcher  struct{ s code.Service  }
  type inProcInfraSearcher struct{ s infra.Service }
  type inProcOrgSearcher   struct{ s org.Service   }
  // Implementam SearchCode/SearchInfra/SearchOrg traduzindo PlaneQuery
  // → query nativa do módulo, e Results nativos → PlaneResults.
  ```
- Testes:
  - Cada outbound port com fake; service compõe e retorna resultados
    mergeados.
  - Cenário "code lento" → timeout → `PlaneResults{Degraded: true, Reason: "timeout"}`
    para code, demais planos OK, status 200 com aviso.
  - Cenário "todos os planos falharam" → 503.
  - Cursor federado: paginar → segunda página → fingerprint consistente.

**Critério de pronto:**
- [ ] `GET /v1/search?q=foo` retorna hits dos 3 planos.
- [ ] Teste de partial failure verde.
- [ ] `app/search` não importa nenhum `modules/*`.
- [ ] Adapter mora em `cmd/api/main.go` (ou em `cmd/api/wiring/` se
      ficar grande).

---

### S-010 — Lint `depguard` + cleanup

**Objetivo:** travar as fronteiras desenhadas.

**Entregas:**
- `.golangci.yml` (criar ou estender) com `depguard`:
  ```yaml
  linters-settings:
    depguard:
      rules:
        core-isolation:
          list-mode: lax
          files: ["**/internal/core/**"]
          deny:
            - pkg: "costEngine/internal/modules"
            - pkg: "costEngine/internal/app"
            - pkg: "costEngine/internal/cmd"
            - pkg: "costEngine/internal/platform"
        platform-isolation:
          files: ["**/internal/platform/**"]
          deny:
            - pkg: "costEngine/internal/modules"
            - pkg: "costEngine/internal/app"
            - pkg: "costEngine/internal/cmd"
        module-cross-plane:
          files: ["**/internal/modules/!(bridge)/**"]
          deny:
            - pkg: "costEngine/internal/modules/(?!bridge)"
              # regex: permite o próprio módulo, bloqueia outros
        controller-no-repo:
          files: ["**/internal/modules/*/controller/**"]
          deny:
            - pkg: "costEngine/internal/repository"
  ```
- Re-exports temporários de S-003 (`app/api/v1/cursor.go`, etc.)
  deletados se nenhum import remanescente.
- `internal/controller/` (vazia) deletada.
- `internal/app/api/v1/` deletada se vazia.
- `cmd/cli/main.go` revisado: confirmar que consome `modules/*/service`
  e não `repository` direto (oportunidade — não bloqueia merge se
  precisar de migração separada).

**Critério de pronto:**
- [ ] `golangci-lint run` verde.
- [ ] Tentativa de import proibido (PR de teste local) falha o lint.
- [ ] Árvore final corresponde à seção §2 deste documento.

---

### S-011 — ADR-005 + atualização do `04-modular.md`

**Objetivo:** documentar o padrão para o futuro.

**Entregas:**
- `docs/architecture/decisions/ADR-005-controllers-ports-erros-tipados.md`:
  contexto, decisão (uma Service por módulo, controller por módulo,
  HTTPProblem, port-sets em `app/`), consequências, alternativas
  consideradas (3 services por módulo descartado — D1), status
  `accepted`.
- `docs/architecture/04-modular.md`:
  - §2: ajustar árvore-alvo refletindo `platform/httpserver`, `core/errs`,
    `modules/<X>/controller`, `app/search`.
  - §3: adicionar regra `modules/*/controller` → `repository` direto
    proibido.
  - §9: marcar passos 4-6 como executados; passos 1-3 (mover `entity/*`
    para `core/*`) seguem pendentes em feature separada.
- `docs/api/v1/errors.md` (novo): catálogo de códigos de erro
  namespaced (`org.team.not_found`, etc.), shape RFC 7807, exemplos.
- `docs/backlog/refinement-status.md`: marcar F-016 como `done`.
- `docs/backlog/pendencias.md`: adicionar feature de "Migração
  `entity/*` → `core/*`" (passos 1-3 do `04-modular.md §9`) como
  pendência futura.

**Critério de pronto:**
- [ ] ADR-005 mergeado, status `accepted`.
- [ ] `04-modular.md` reflete o estado real.
- [ ] `errors.md` lista todos os códigos introduzidos em S-005..S-009.

---

## Sequenciamento e dependências entre slices

```
S-001 ─┐
       ├─► S-003 ─► S-004 ─► S-005 ─┬─► S-006 ─┐
S-002 ─┘                            │          ├─► S-008 ─► S-010 ─► S-011
                                    └─► S-007 ─┤
                                               └─► S-009 ─┘
```

- S-001 e S-002 são independentes; pode-se mergear em qualquer ordem.
- S-003 depende de ambos.
- S-004 e S-005 são piloto sequencial.
- **Gate após S-005:** validar padrão antes de S-006/S-007.
- S-006, S-007, S-009 podem rodar em paralelo (módulos diferentes).
- S-008 espera S-006/S-007 (precisa dos módulos prontos para dispatch
  polimórfico).
- S-010 espera tudo estar nos lugares finais.
- S-011 fecha.

## Riscos

| Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|
| Piloto S-005 expõe que o padrão é ergonomicamente ruim | Média | Alta | Gate explícito pós-S-005; iterar antes de replicar |
| Middleware reordenado em S-003 vaza tenant | Baixa | Alta | Teste de integração novo em S-002 cobrindo cadeia |
| Move de testes quebra fixtures path-relative | Baixa | Baixa | Grep por `testdata/` antes de cada move |
| Lint S-010 acusa violações em `bridge` (que é exceção) | Média | Baixa | Allowlist explícita de `bridge` no `depguard` |
| `app/search` (S-009) vira boilerplate sem 2ª feature transversal real | Média | Média | Aceitar como template; o custo é justificado pelo `app/simulate` futuro |
| Breaking change de shape de erro (D5) afeta cliente externo desconhecido | Baixa | Média | Flag `LegacyErrorBody` disponível em S-003 se aparecer cliente |
| `nodes.go` em S-006/S-007/S-008 é difícil de fatiar limpamente | Média | Média | Estratégia: separar primeiro por Kind (S-006/S-007), depois resto para graph (S-008); revisar em S-006 |

## Rollback

Cada slice é um PR atômico — rollback via `git revert` mantém base
estável. Pontos socialmente irreversíveis:

- **S-005**: depois de deletar `app/api/v1/org.go`, voltar é caro
  (histórico tem o arquivo, mas refazer integração de testes dá
  trabalho). Por isso o gate de decisão antes de S-006.
- **S-010**: o lint, uma vez no CI, torna-se contrato. Relaxar gera
  regressão silenciosa.

## Entregue em

(slices listadas acima, S-001..S-011)

## Notas de implementação

- Pacote raiz dos novos componentes: `internal/platform/httpserver/`,
  `internal/core/errs/`, `internal/modules/<X>/{port, errs, controller,
  types|node}`, `internal/app/{graph, search}/`.
- Convenções:
  - Sem `Interface` sufixado, sem `I` prefixado, sem `Impl` sufixado.
  - Service expõe `Service interface` + `service` struct (lowercase,
    construído via `New(...) Service`).
  - Erros: prefixo `Err` (`ErrTeamNotFound`).
  - Códigos de erro: `<modulo>.<entidade>.<situacao>` lowercase
    com pontos.
- OpenAPI (`docs/api/v1/openapi.yaml`) precisa atualização em S-005
  (shape de erro RFC 7807) e em S-009 (endpoint `/v1/search`). Listar
  como sub-tarefa do PR correspondente.
- F-016 não toca em `internal/entity/*` — essa migração (passos 1-3
  do `04-modular.md §9`) é feature separada, registrada em
  `pendencias.md` por S-011.
