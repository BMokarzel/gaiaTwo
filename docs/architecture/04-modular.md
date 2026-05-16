# Arquitetura Modular do CostEngine

> Documento de design da organização do código-fonte: módulos, dependências,
> superfícies de acoplamento, fronteiras de execução (CLI/API) e como isso
> funciona na prática.
>
> Complementa `modelagem.md` (modelo de domínio), `arquitetura-custos.md`
> (plano de custos) e `plano-implementacao.md` (fases).

---

## 1. Princípios

1. **Monólito modular por plano**, não por entidade. Três planos de primeira
   ordem — `infra`, `cost`, `code` — com fronteiras estáveis e contratos
   explícitos. Subdividir mais cedo é fricção; mais tarde é dívida.
2. **Núcleo compartilhado mínimo**. Só entra em `core/` o que é importado por
   **dois ou mais** módulos. Núcleo grande vira lixo; núcleo zero gera
   duplicação. A regra do "dois importadores" decide.
3. **Inversão de dependência nos limites**. Cada módulo expõe interfaces no
   seu próprio pacote; adapters (Neo4j, ClickHouse, AWS SDK) vivem em
   subpacotes do módulo, não no núcleo.
4. **Sem ciclos de import**. Direção única: `core/ ← modules/* ← app/ ← cmd/`.
   Nenhum módulo importa outro módulo diretamente — comunicação cruza por
   `core/` (tipos compartilhados) ou pela camada `app/` (orquestração).
5. **Fronteiras de execução são pacotes em `cmd/`**, não módulos. CLI e API
   são *consumidores* dos mesmos casos de uso; jamais reimplementam regra.
6. **Compatibilidade com o estilo gaia**, sem copiá-lo cegamente. Adota-se o
   padrão `Properties + Entity + New() + Repository` por entidade, mas
   *dentro* de cada módulo, agrupado por plano.

---

## 2. Árvore de pacotes

```
costEngine/
  cmd/
    cli/                       # binário "ce" — extração, ingestão, jobs
      main.go
    api/                       # binário "ce-api" — HTTP/gRPC público
      main.go

  internal/
    core/                      # shared kernel (importável por todos)
      urn/                     # URN tipada, ParseURN, builders
      bitemporal/              # Meta, ValidFrom/ValidTo, AsOf, Confidence
      kind/                    # node.Kind e edge.Type como strings tipadas
      edge/                    # interface Edge, adjacency matrix, Validate
      repository/              # interfaces NodeRepository/EdgeRepository
      errors/                  # ErrNotFound, ErrConflict, ErrInvalidArgument

    modules/
      infra/
        node/                  # Compute, Persistence, Network, Messaging,
                               #   Provider, Account, Region, Zone, Environment
        edge/                  # Contains, DeployedOn, AttachedTo, Routes,
                               #   Peers (edges *internos* a infra)
        collector/             # interfaces Collector + impls AWS/GCP/Azure
        repo/                  # adaptador Neo4j + memory para infra
        service/               # casos de uso: discoverInfra, reconcileInfra

      cost/
        domain/                # CURRow, BillingPeriod, Allocation, Projection
        ingest/                # bronze→silver (Parquet/Iceberg)
        allocate/              # engine de rateio (urn × period × dimension)
        rollup/                # gold (ClickHouse)
        repo/                  # adaptadores Parquet/Iceberg/ClickHouse
        service/               # casos de uso: ingestCUR, allocate, query

      code/
        node/                  # Service, Endpoint, Function, Method, Class,
                               #   Database, Table, Column, ExternalCall,
                               #   Middleware, Owner, Process, Telemetry, ...
        edge/                  # Calls, Imports, Reads, Writes (intra-code)
        extract/               # parsers AST, OpenAPI, Protobuf, schema dump
        repo/                  # adaptador Neo4j para code plane
        service/               # casos de uso: indexRepo, refreshTelemetry

      bridge/                  # *o único módulo que conhece dois planos*
        edge/                  # DeployedOn (code→infra), BackedBy (code→infra),
                               #   CommunicatesWith (code↔code via infra)
        service/               # casos de uso: resolveDeployment, linkSchema

    app/                       # orquestração de casos de uso transversais
      simulate/                # what-if: latência, custo, redundância
      query/                   # GraphQL/REST resolvers
      ingestpipeline/          # encadeia infra.discover + cost.ingest + code.index
      webhook/                 # handler para GitHub Actions

    platform/                  # adapters de infraestrutura técnica
      neo4j/                   # Client compartilhado (driver, sessions)
      clickhouse/              # client + migrations
      objectstore/             # S3/GCS
      httpserver/              # router, middleware, auth
      observability/           # logger, tracer, metrics
```

### O que mora onde — regra rápida

| Coisa | Lugar | Por quê |
|---|---|---|
| `Kind`, `URN`, `Meta`, `AsOf`, `Confidence` | `core/` | Importado por todos os 3 planos |
| `NodeRepository`, `EdgeRepository` (interfaces) | `core/repository/` | Contrato comum |
| `Compute`, `Persistence`, `Network`... | `modules/infra/node/` | Tipos do plano infra |
| `Service`, `Endpoint`, `Function`... | `modules/code/node/` | Tipos do plano code |
| `CURRow`, `Allocation` | `modules/cost/domain/` | Tipos do plano cost |
| `DeployedOn` (Service→Compute) | `modules/bridge/edge/` | Cruza code↔infra |
| Driver Neo4j, pool ClickHouse | `platform/` | Infra técnica, não domínio |
| Caso de uso "simular +2 req/s" | `app/simulate/` | Toca 3 planos |
| Comando `ce extract aws` | `cmd/cli/` | Entrypoint, não regra |

---

## 3. Grafo de dependências

```
                    cmd/cli        cmd/api
                       \             /
                        \           /
                         v         v
                          app/
                  /         |          \
                 v          v           v
        modules/infra  modules/cost  modules/code
                 \          |          /
                  \         |         /
                   v        v        v
                       core/  platform/
```

**Regras invioláveis:**

- `core/` **não importa nada** de `modules/`, `app/`, `cmd/`, `platform/`
  (pode importar `repository/` — `core/errs.Render` mapeia sentinelas
  como `repository.ErrNotFound` → HTTPProblem).
- `platform/` **não importa nada** de `modules/`, `app/`, `cmd/`.
- `repository/` **não importa nada** de `modules/`, `app/`, `cmd/`.
- `modules/<X>` **não importa** `modules/<Y>` diretamente. Comunicação só
  via `core/` (tipos) ou via `modules/bridge/` (integração cross-plano).
- `modules/<X>/controller` **não importa** `internal/repository`. Recebe
  por construtor uma instância de `module.<X>.Service` (port narrow,
  consumer-owned). O Service é quem fala com o repo.
- `modules/bridge/` é exceção cross-módulo: pode importar `modules/infra`
  e `modules/code` porque sua razão de existir é vincular os dois.
  Custo de manutenção concentrado em um lugar.
- `app/<X>` pode importar qualquer `modules/*` e `core/*`. É a camada de
  composição.
- `app/graph/controller` é exceção documentada (**ADR-005**): pode
  importar `internal/repository` porque é um *thin wrapper* cross-kind
  sobre `NodeRepository`/`EdgeRepository`, sem lógica acima do grafo.
- `app/search` segue o pattern "port-set": **não** importa
  `internal/repository`. Define `search.NodeSearcher` (port narrow,
  consumer-owned); o adapter de `NodeRepository` vive em
  `cmd/api/main.go`. Modelo de referência para futuros `app/*`.
- `cmd/*` pode importar `app/*` e `platform/*`. Nunca regra de negócio
  diretamente — sempre via `app/` ou `modules/<X>/service`.

Um lint em CI (`depguard` no golangci-lint, em `.golangci.yml`) trava
PR que viole essas regras — codifica todos os carve-outs acima. Sem
isso, o monólito vira pasta com nome chique.

---

## 4. Interfaces e desacoplamento — onde estão os "plugs"

### 4.0 Service-per-module (F-016)

Cada módulo de domínio expõe **uma única interface `Service`** no
seu package raiz — o inbound port narrow do bounded context. O
controller HTTP do módulo consome esse Service; nunca o repo.

```
modules/<X>/
  port.go     # type Service interface { ... } — consumer-owned
  errs.go     # erros tipados (ErrXxxNotFound, ...) implementam HTTPProblem
  types.go    # DTOs: Page[T], queries, summaries, details
  service/    # implementação(ões) do Service (reader, writer, ...)
  controller/ # HTTP handlers; dependem só de module.Service
```

Em código:

```go
// modules/org/port.go
type Service interface {
    GetTeam(ctx context.Context, urn node.URN, opts AsOfOptions) (TeamDetail, error)
    ListTeams(ctx context.Context, q ListTeamsQuery) (Page[TeamSummary], error)
    // ...
}

// modules/org/service/reader.go
func New(nodes repository.NodeRepository, edges repository.EdgeRepository) org.Service {
    return &reader{nodes: nodes, edges: edges}
}

// modules/org/controller/controller.go — NÃO importa repository
func New(svc org.Service) *Controller { return &Controller{svc: svc} }
```

Bounded context = módulo = um Service. Não há "OrgReaderService" +
"OrgWriterService" como portas separadas no controller; a interface
agrega o que o controller HTTP precisa expor.

### 4.1 Repository (já existe, parcial)

`core/repository/repository.go` define as interfaces. Cada módulo:

- **Consome** as interfaces para escrever sua lógica.
- **Implementa** as interfaces em `modules/<X>/repo/` (um adapter por
  storage: Neo4j, in-memory, eventualmente ClickHouse para `cost`).

Casos de uso recebem repos por construtor, nunca instanciam:

```go
// modules/infra/service/discover.go
type DiscoverService struct {
    nodes core.NodeRepository
    edges core.EdgeRepository
    aws   infra.AWSCollector
}

func NewDiscoverService(n core.NodeRepository, e core.EdgeRepository, a infra.AWSCollector) *DiscoverService { ... }
```

Em `cmd/cli/main.go` o wiring é explícito:

```go
client := platform.NewNeo4jClient(cfg)
nodeRepo := infrarepo.NewNodeRepo(client)
edgeRepo := infrarepo.NewEdgeRepo(client)
awsCol  := aws.New(cfg.AWS)
discover := infrasvc.NewDiscoverService(nodeRepo, edgeRepo, awsCol)
```

### 4.2 Collector (a criar — Fase 2)

`modules/infra/collector/` define:

```go
type Collector interface {
    Provider() node.ProviderID
    Discover(ctx context.Context, scope Scope) (<-chan Resource, <-chan error)
}
```

Implementações: `aws/`, `gcp/`, `azure/`, `k8s/`. Cada uma é um subpacote
independente. CLI escolhe a implementação por flag; nada acima depende da
escolha.

### 4.3 Extractor de código

`modules/code/extract/` define:

```go
type Extractor interface {
    Kind() string                   // "go-ast", "openapi", "protobuf", "sql-ddl"
    Extract(ctx context.Context, src Source) (Snapshot, error)
}
```

Mesmo padrão. Um `code` repo pode rodar múltiplos extractors em paralelo.

### 4.4 Bridge resolver

`modules/bridge/service/resolve.go` é o ponto único que sabe **fazer
ligações entre planos**:

```go
type Resolver struct {
    infraNodes core.NodeRepository    // lê Compute por external_id
    codeNodes  core.NodeRepository    // lê Service por id
    edges      core.EdgeRepository    // escreve DeployedOn, BackedBy
}

func (r *Resolver) LinkDeployment(ctx context.Context, svcURN, computeExtID string) error
```

Toda heurística "qual Compute é este Service?" mora aqui. Se um dia virar
microserviço, esse módulo é o candidato natural — porque já é um bounded
context próprio.

### 4.5 Pipelines de ingestão

`app/ingestpipeline/` orquestra:

```
infra.Discover → bridge.LinkInfra → cost.IngestCUR → bridge.LinkCost
```

Cada etapa é uma chamada a um service do módulo respectivo. Pipeline é
linha reta, sem regra própria.

---

## 5. Acoplamentos que sobrevivem (e por quê são aceitáveis)

| Acoplamento | Quem ↔ Quem | Aceitável porque |
|---|---|---|
| Tipos de `Kind` | `core/kind` ↔ todos | `Kind` é só string; cada módulo declara as suas constantes |
| Adjacency matrix | `core/edge` precisa conhecer pares válidos de `Kind` | É o **contrato** do grafo; mudança aqui é mudança de schema, deve ser visível |
| URN | `core/urn` parseia URNs de todos os planos | Esquema único é a feature, não bug |
| `bridge/` | importa `infra` e `code` | Existe para isso; isolado em um módulo |
| Driver Neo4j | `platform/neo4j` é usado por 3 repos | É infra técnica; cada repo só recebe `*Client` |

Acoplamentos que **NÃO** podemos aceitar:

- `infra` importar `code` ou vice-versa → quebra a fronteira.
- `core/` referenciar uma entidade concreta (ex.: `node.Compute`) → puxa
  todo o plano infra para o núcleo.
- Repositório de um módulo conhecer entidade de outro módulo → o tipo
  passa por `core/` ou pela camada de aplicação.

---

## 6. Fronteiras de execução — CLI vs API

O CostEngine tem **dois binários** com responsabilidades distintas:

### 6.1 `cmd/cli` — o executor

Onde **trabalho pesado e periódico** acontece. Não atende usuário em tempo
real. Roda como cronjob, worker, GitHub Action runner, ou comando manual.

Sub-comandos previstos:

```
ce extract infra --provider=aws --account=123456789012
ce extract code  --repo=./services/checkout --kind=go-ast
ce ingest  cost  --period=2026-04 --source=s3://cur/...
ce link    bridge --since=24h
ce migrate --backend=neo4j
ce simulate --scenario=move-region --service=svc:checkout --to=us-east-1
```

A CLI:
- Lê config (`~/.costengine.yml`, env, flags).
- Instancia adapters (Neo4j client, AWS SDK, etc.) via `platform/`.
- Constrói services de `modules/*/service` injetando dependências.
- Chama `app/ingestpipeline` quando precisa de fluxo composto.
- Reporta progresso e erros via stdout/stderr (também emite eventos
  estruturados para observabilidade).

### 6.2 `cmd/api` — o receptor

Onde **input declarativo de humanos e CI** chega. Não faz extração; apenas
registra intenção e dispara jobs.

Endpoints previstos:

| Verbo | Rota | Quem chama | O que faz |
|---|---|---|---|
| POST | `/v1/services` | humano (form/curl) | registra novo `Service` no grafo de code |
| POST | `/v1/services/:id/deployments` | humano ou pipeline | declara `DeployedOn` |
| POST | `/v1/webhooks/github` | GitHub Action | recebe evento de merge/release, enfileira `ce extract code` |
| POST | `/v1/webhooks/billing` | GitHub Action / cron | dispara `ce ingest cost` |
| GET  | `/v1/query/...` | dashboards, humanos | leitura do grafo (custo, dependências, etc.) |
| POST | `/v1/simulate` | dashboards | executa simulação síncrona ou assíncrona |

A API:
- **Não chama collectors diretamente.** Recebe input, valida, persiste a
  intenção (ou um job pendente) e devolve 202.
- **Dispara CLI** via mecanismo escolhido: (a) executando o binário como
  subprocesso, (b) enfileirando em fila externa (NATS/Redis/SQS) que um
  worker baseado em `cmd/cli` consome, (c) chamando `app/*` diretamente
  para operações leves (registrar Service, query).
- **Reusa exatamente os mesmos services** de `modules/*/service` para
  operações leves. CLI e API são "dois rostos do mesmo cérebro".

### 6.3 Fluxo típico — GitHub Action atualiza o grafo

```
[gh action push main]
       │
       ▼
POST /v1/webhooks/github  ──► cmd/api
       │                       │
       │                       ├─ valida HMAC do GitHub
       │                       ├─ extrai {repo, sha, changed_paths}
       │                       ├─ enfileira job: "extract code --repo=X --sha=Y"
       │                       └─ 202 Accepted
       │
       ▼
[worker baseado em cmd/cli consome fila]
       │
       ├─ ce extract code --repo=X --sha=Y
       │       │
       │       ├─ code.extract.GoASTExtractor.Extract()
       │       ├─ code.repo.Upsert (versiona nodes/edges)
       │       └─ bridge.service.Resolver.LinkDeployment()
       │
       └─ emite evento "grafo atualizado" (opcional)
```

A API **nunca** roda o extractor inline. Mesmo que fosse rápido, o
isolamento da fila garante: (a) backpressure controlado, (b) retry barato,
(c) audit log natural.

### 6.4 Fluxo típico — humano registra novo serviço

```
POST /v1/services { id: "svc:reports", owner: "data-team", ... }
       │
       ▼
cmd/api → app.RegisterService(svc) → modules/code/service.Register()
       │                                      │
       │                                      └─ code.repo.Upsert (Service node)
       │
       └─ 201 Created
```

Operação síncrona, leve, sem fila. Quando o usuário em seguida fizer um
commit ao repositório do `svc:reports`, o webhook GitHub vai enriquecer o
nó com Endpoints, Functions etc.

---

## 7. Como funciona na prática — três cenários

### Cenário A: bootstrap completo de uma conta AWS nova

1. Operador roda `ce migrate` → aplica schema Neo4j + ClickHouse.
2. `ce extract infra --provider=aws --account=...` → popula `infra/node`
   e `infra/edge` (Provider, Account, Region, Compute, Persistence...).
3. `ce ingest cost --period=2026-04` → popula tabelas ClickHouse, gera
   `Allocation` por URN. O linker resolve URN a partir do `external_id`
   no CUR via `infra.NodeRepository.GetByExternalID`.
4. Para cada repo Git registrado: `ce extract code --repo=...` → popula
   `code/node` e `code/edge`.
5. `ce link bridge` → cria `DeployedOn` (Service→Compute), `BackedBy`
   (Table→Persistence) usando heurísticas (tags, anotações, manifestos).

Estado final: grafo unificado consultável.

### Cenário B: simulação "mover svc:checkout para us-east-1"

1. Cliente HTTP: `POST /v1/simulate {scenario, service, to_region}`.
2. `cmd/api` chama `app.simulate.MoveRegion(ctx, ...)`.
3. `app/simulate` consulta:
   - `modules/code/repo` → `Service`, `ExternalCall`s, `Telemetry` (req/s).
   - `modules/infra/repo` → `Compute` atual + `Compute` equivalente em
     us-east-1 (preço, latência base).
   - `modules/cost/repo` → preço por hora, egress por GB.
   - `modules/bridge` → edges atuais e calcula diff.
4. Retorna projeção em JSON.

Note: simulação **não escreve** no grafo. É leitura + cálculo.

### Cenário C: novo serviço entra no monólito

1. Dev faz commit em `/services/notifications`, GitHub Action POSTa o
   webhook.
2. `cmd/api` enfileira `ce extract code --repo=... --sha=...`.
3. Worker roda extractor Go AST, descobre `Service`, `Endpoint`s,
   `Function`s, `ExternalCall`s, `Table`s referenciadas.
4. `code.repo.Upsert` versiona tudo (bitemporal).
5. `bridge.Resolver.LinkDeployment` tenta casar `svc:notifications` com
   um `Compute` existente — por tag, por path em manifesto Helm, por
   convenção. Se achar, cria `DeployedOn`. Se não, registra
   `pending_deployment_link` para humano resolver via API.

---

## 8. Trade-offs comparados

| Aspecto | Estrutura plana (atual) | Por entidade (estilo gaia) | **Modular por plano (proposto)** |
|---|---|---|---|
| Refatorar bitemporal | 1 lugar | 21 lugares | 1 lugar (`core/bitemporal`) |
| Ownership por equipe | ruim | bom mas pulverizado | bom e concentrado |
| Apagar feature | difícil | fácil | fácil (deleta `modules/<X>`) |
| Storage poliglota | força tudo no Neo4j | livre | livre (cada módulo escolhe) |
| Onboarding novo dev | precisa ler tudo | precisa mapear 21 módulos | lê o plano relevante |
| Lint de fronteira | n/a | difícil (muitas regras) | viável (3 regras) |
| Edges cross-plane | natural mas implícito | sofre | explícito em `bridge/` |
| Risco de virar microserviço | n/a | over-engineered | preparado sem custo |
| Boilerplate | mínimo | alto | médio |

---

## 9. Plano de migração a partir da estrutura atual

> Status (2026-05-16): passos 4-6 executados via F-016; passos 1-3
> (mover `entity/*` para `core/*`) seguem pendentes em feature separada
> registrada em `docs/backlog/pendencias.md`.

**Passos:**

1. ⏳ **Pendente** — Criar `internal/core/` e mover:
   - `internal/entity/node/urn*.go` → `internal/core/urn/`
   - `internal/entity/node/{meta, base, source, lineage, confidence}` →
     `internal/core/bitemporal/`
   - `internal/entity/node/kind.go` → `internal/core/kind/`
   - `internal/entity/edge/edge.go` (interface) + `registry.go` →
     `internal/core/edge/`
   - `internal/repository/repository.go` → `internal/core/repository/`
2. ⏳ **Pendente** — Criar `internal/modules/infra/` e mover:
   - Todos os structs concretos de `entity/node` → `modules/infra/node/`
   - Todos os structs de `entity/edge` que ficam dentro de infra →
     `modules/infra/edge/`
   - `repository/memory` e `repository/n4j` → `modules/infra/repo/`
3. ⏳ **Pendente** — Atualizar imports em massa (`go fmt` + script de
   substituição).
4. ✅ **Feito (F-016 S-010)** — `depguard` ativo em `.golangci.yml`
   travando as fronteiras de §3 (core/platform/repository/modules cross
   e controller→repository).
5. ✅ **Feito** — `cost/`, `code/`, `org/`, `bridge/` criados como
   pastas próprias sem tocar em `infra/`. Estrutura final em §4.
6. ✅ **Feito** — `bridge/` existe (`modules/bridge`,
   `modules/bridge/service_compute`) com edges cross-plane reais
   (`Service → Compute`).

**Adições não previstas no plano original, executadas em F-016:**

- ✅ `internal/platform/httpserver/` — server compartilhado, middlewares
  (RequestID/Recover/Auth), helpers `WriteJSON`/`WriteError`/`ParseAsOf`/
  cursor opaco.
- ✅ `internal/core/errs/` — `HTTPProblem` interface, `Problem` (RFC 7807-like),
  `Render(err) (status, Problem)` com fallback para sentinelas.
- ✅ Padrão **service-per-module**: cada `modules/<X>/` ganhou
  `port.go` (Service interface, consumer-owned), `errs.go` (erros
  tipados implementando `HTTPProblem`), `types.go` (DTOs).
- ✅ Padrão **controller-per-module**: `modules/org/controller/` migrado
  de `internal/app/api/v1/`. `internal/app/api/v1/` deletado.
- ✅ Decisão: `code` e `infra` **não ganharam controller próprio** —
  não havia endpoints REST específicos desses domínios; os Kinds são
  acessados via `app/graph/controller` (dispatch cross-kind sobre o
  grafo) e via `app/search` (port-set). Controllers de módulo serão
  criados quando aparecer endpoint `/v1/code/*` ou `/v1/infra/*` de
  domínio próprio.
- ✅ `app/graph/controller/` — handlers cross-kind
  (`/v1/architecture/nodes/{urn}`, `/neighbors`, `/history`, `/paths`).
  Exceção documentada em ADR-005: pode importar `repository` direto.
- ✅ `app/search/` — port-set como referência viva (inbound `Engine`,
  outbound `NodeSearcher`); adapter in-proc em `cmd/api/main.go`.
- ✅ ADR-005 mergeado; catálogo de erros em `docs/api/v1/errors.md`.

---

## 10. Checklist de "está bem modular?"

Use isto antes de aceitar PRs grandes:

- [ ] Nenhum arquivo em `core/` referencia tipo concreto de `modules/*`.
- [ ] Nenhum `modules/<X>` importa `modules/<Y>` (exceto `bridge/`).
- [ ] Nenhum `modules/<X>/controller` importa `internal/repository`
  (consome `module.<X>.Service`).
- [ ] Nenhum arquivo em `cmd/` contém regra de negócio (só wiring +
  parsing de flags + serialização HTTP).
- [ ] Toda dependência externa (Neo4j, AWS, Clickhouse) é injetada via
  interface; testes usam fake/memory.
- [ ] Edge cross-plane mora em `modules/bridge/edge/`, não nos planos.
- [ ] Adicionar um novo `Kind` toca: (a) o módulo dono, (b) a matriz de
  adjacência em `core/edge/`. Nada mais.
- [ ] `cmd/cli` e `cmd/api` chamam apenas `app/*` e `modules/*/service`,
  nunca `*/repo` direto.
- [ ] Lint de import (`depguard` em `.golangci.yml`) está verde no CI.
- [ ] Exceções `app/graph` (importa repository) e `app/search` (não
  importa) seguem documentadas em ADR-005.

---

## 11. O que isto NÃO é

- **Não é DDD ortodoxo.** Não há "agregados", "eventos de domínio",
  "value objects" obrigatórios. Pegamos só o que serve: bounded contexts
  via módulos, repositories como interfaces.
- **Não é hexagonal puro.** Não há separação rígida "domain/application/
  infrastructure" dentro de cada módulo — seria boilerplate sem retorno
  para o tamanho do projeto. A separação é entre `module/*` (lógica) e
  `platform/*` + `module/*/repo` (adapters).
- **Não é preparação para microserviços.** É preparação para *ser
  capaz* de virar microserviço se um dia for necessário — sem pagar o
  custo de já ser um agora.
- **Não é gaia.** Gaia é "um módulo por entidade"; aqui é "um módulo por
  plano". As lições do gaia (padrão `Properties + Entity + New() +
  Repository`, ID tipado por prefixo) são adotadas *dentro* de cada
  módulo.
