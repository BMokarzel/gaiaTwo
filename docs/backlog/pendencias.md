# Pendências do backlog

> Lista única e priorizada do que ficou aberto. Cada item registra
> origem, escopo, dor/risco de adiar e gatilho para promover a feature.
>
> Atualizado: 2026-05-18 (sessão tarde: F-030 Writer completo, web scaffold,
> seed dev `--seed-code`, gaps remanescentes do sidecar TS).

## Visão geral

- **Sem features `refined`/`in_progress`.** Roadmap só tem slices
  dentro de features `done com backlog parcial` ou follow-ups
  técnicos.
- **Backlogs vivos** (slices dentro de features done): F-017, F-019,
  F-022, F-023, F-025, F-030.
- **Entity-layer só**: F-026 (Persona) — sem coletor, sem dor.
- **Follow-ups técnicos**: migração `entity→core`, OpenAPI sync,
  docs de módulo, controllers code/infra, BusinessArea, etc.

Para o painel de estado por feature/épico, ver
[refinement-status.md](refinement-status.md).

---

## 1. Backlogs vivos por feature

Slices que vivem dentro de uma feature já `done`. Promover a story
nova quando aparecer dor concreta (usuário real, integração nova,
ADR que mude o modelo).

### F-017 — Cross-language identity

- **Origem:** ADR-006/F-017. Go coletor pronto; TS agora coberto via
  F-030 (sidecar Node + ts-morph). Python ainda parcial — só emite
  Frameworks via `collector/python`, sem Services/Endpoints/Functions.
- **Pendente:**
  - Coletor Python full (`pyproject.toml`, `setup.py`) — emite
    Service + Module + Function + Endpoint (FastAPI/Flask/Django).
    Decidir entre sidecar Python (espelhar F-030) ou parser puro-Go.
  - Normalização de `ManifestType` ao indexar (já existe enum, falta
    coletores populá-lo).
- **Dor adiada:** usuários atuais são Go/TS. Sem demanda Python ainda.
- **Gatilho:** primeiro repo Python no escopo de algum tenant.

### F-019 — Call boundary

- **Origem:** ADR-007. Inbound + outbound subkinds entregues
  (HTTP/gRPC/SQS/SNS/Kinesis/cron + FunctionCall).
- **Pendente:** `EventSubscribe` (consumer Kafka/Pub-Sub) — requer
  type-resolve (depende de F-021 IMPLEMENTS).
- **Gatilho:** primeira app event-driven entrar no grafo.

### F-021 — Type/Variable

- **Origem:** F-021. `Type`, `Variable`, `EXTENDS`, `ALIASES` prontos.
- **Pendente:** `IMPLEMENTS` (Type→Type, struct↔interface em Go;
  `implements` explícito em TS/Java). Requer resolução estrutural.
- **Gatilho:** quando F-019 `EventSubscribe` ou type-aware allocation
  pedir.

### F-022 — Schema

- **Origem:** F-022. OpenAPI adapter pronto.
- **Pendente:**
  - JSON Schema standalone (sem OpenAPI ao redor).
  - Avro (Confluent Schema Registry).
  - GraphQL SDL.
  - `oneOf`/`allOf`/`anyOf` em FieldSlots — hoje achatado.
- **Gatilho:** primeiro contrato fora de OpenAPI/Proto.

### F-023 — Framework / License / SecurityAdvisory

- **Origem:** F-023. Coletores Go (go.mod) + npm prontos. Edges
  `LICENSED_UNDER`, `AFFECTED_BY`, `PATCHED_IN` modelados.
- **Pendente:**
  - Coletor `pyproject.toml` / `Cargo.toml`.
  - Lookup SPDX (mapping licença → ID canônico).
  - Sync OSV / GHSA (popula `SecurityAdvisory` + edges).
- **Gatilho:** primeira dor de compliance/security tracking.

### F-030 — TypeScript code collector (sidecar)

- **Origem:** F-030 / ADR-012. Pipeline base entregue (slices S-030.1
  a S-030.9): sidecar Node+ts-morph, NDJSON, embed bundle, dispatch
  CLI `--lang=auto|go|typescript`. Smoke test OK: 1 Service + 2
  Modules + 1 Function + 2 Endpoints Express + 1 Type + 3 Variables +
  1 Framework + 1 Call no fixture `ts-fixture`.
- **Entregue na sessão 2026-05-18 (tarde):**
  - `codesvc.Writer.Apply` agora persiste **todos** os Kinds (Modules,
    Types, Variables, Frameworks, Calls) e **todas** as edges
    estruturais (Contains, DependsOn, Invokes, Targets, Uses, Extends,
    Aliases, DefinedIn). `Stats` cobre o conjunto completo. Pré-valida
    `fromKind/toKind` antes de chamar `Edges.Upsert` — erro claro
    quando um endpoint da edge não foi emitido como nó.
  - URN canônica de Framework corrigida no sidecar
    (`extractors/frameworks.ts` e `extractors/calls.ts`):
    `urn:ce:code:_global:framework/npm!<name>` (espelha
    `node.NewFrameworkURN`). Antes vazava
    `urn:ce:code:_:framework/<name>`.
  - Go-side `appendCall` agora emite **`Invokes` Function→Call** (o
    `extractors/calls.ts` documentava "sempre" mas nunca emitia —
    URN do Call só existe Go-side).
  - Go-side `appendEndpoint` emite **`Contains` Module→Endpoint**
    (sidecar `endpoints_express.ts` não conhece o ModuleURN
    canônico). Aceita namespace vazio (módulo raiz).
  - Novo pós-pass `decoder.resolveTargets()` em
    `internal/modules/code/collector/typescript/resolve.go`: resolve
    **`Targets` Call→Function** por last-segment match
    (`this.repo.findById` → `(UsersRepository).findById`) restrito ao
    mesmo Service. Ambíguos (>1 match) são ignorados.
  - Seed dev `cmd/api --seed-code <repo> --seed-repo <name>` aplica
    `typescript.Collect` + `Writer.Apply` na subida. Sample em
    `C:/Users/User/AppData/Local/Temp/sample-api` produz 1 service /
    1 module / 3 endpoints / 9 functions / 4 types / 3 vars /
    2 frameworks / 10 calls / **38 edges** — fluxo do endpoint
    inteiramente navegável via BFS bidirecional `/flow`.
- **Pendente (high-impact):**
  - **Sidecar não capta calls dentro de arrow functions inline.** Só
    `FunctionDeclaration` e métodos de classe entram em `callers` no
    `extractors/calls.ts`. Handlers `app.get('/x', async (req, res) =>
    {...})` perdem todo o conteúdo. Workaround atual: refatorar
    sample-api para funções nomeadas top-level. Fix proper exige
    walker recursivo em arrow expressions passadas como argumento.
  - **Endpoint→Function (handler) sem edge direta no ontológico.**
    Hoje o link é implícito via `Module CONTAINS Endpoint` +
    `Module CONTAINS Function`, navegável só por BFS bidirecional.
    Quando o consumer precisar saber "qual Function é handler deste
    Endpoint" sem assumir Module pivot, criar edge nova
    (`HANDLED_BY` Endpoint→Function?) no registry e estender
    `appendEndpoint` para emitir via lookup `handler_symbol`.
  - **`resolveTargets` Call→Function é heurística.** Falha quando
    múltiplas Functions no mesmo service compartilham last-segment
    (ex.: dois `.find` distintos). Slice futura (F-026 escopo TS):
    symbol resolver com `ts-morph` type checker no sidecar — emitir
    `Targets` direto com URN resolvida.
  - **Resolução cross-module via imports não acontece.** Calls que
    cruzam arquivos via `import { x } from './y'` só conectam por
    coincidência de nome (heurística acima).
  - E2E test Go que spawn sidecar real (gated por env/build tag).
  - NestJS: heranças de controller, decorators `@Module` (para Module
    derivado do grafo Nest, não só do filesystem).
  - Express: `app.route('/x').get(...)`, routers em arrays, handlers
    fora do arquivo.
  - CI: build pipeline do bundle (`npm install && npm run build` antes
    de `go build`) ou checkin do bundle pronto (atual: stub no repo
    + rebuild local manual).
- **Dor adiada:** Coleta básica + persistência completos para
  Express típico com funções nomeadas. Arrow inline e cross-module
  ainda apagam parte do fluxo.
- **Gatilho:** primeira app real (não-fixture) entrar no grafo.

### F-025 — Epic / UserStory

- **Origem:** F-025. Entities + edges prontos, CRUD via `ce gov`.
- **Pendente:** integrações ALM (Jira / Linear / GitHub Issues) que
  ingestam epic/userstory automaticamente em vez de CRUD manual.
- **Gatilho:** primeiro produto/feature owner pedir
  "quanto custou entregar epic X?".

---

## 2. Features sem coletor (entity-layer only)

### F-026 — Persona

- **Origem:** F-026 / ADR-009.
- **Estado:** entity `Persona` + edge `Serves(UserStory→Persona)`
  existem; sem coletor, sem CLI, sem CRUD.
- **Decisão atual:** manter em backlog. Sem dor concreta — ADR-009
  proíbe denormalização em código (Persona só chega via UserStory).
- **Gatilho:** primeiro dashboard pedir agregação por Persona.

---

## 3. Follow-up F-013 (Feature→Service via PR)

Backlog explícito documentado em `docs/features/F-013-*.md`:

- **Fila de pendentes** para Features ausentes (label `feature:<urn>`
  aponta para URN inexistente — hoje 400). Replay automático quando a
  URN aparece.
- **Hidratação de `files[]`** via GitHub REST API. Hoje o payload
  assume que um proxy/CI enriquece o evento com a lista de arquivos
  (o evento nativo `pull_request` não traz). Implementar fetch lazy
  com token de instalação.
- **Outros providers** (GitLab MR, Bitbucket PR) — feature dedicada
  futura, não slice de F-013.
- **Decay de confidence** ao longo do tempo (edge antiga perde força
  se nenhum PR recente reforça).

**Gatilho:** primeiro consumer real do endpoint
`POST /v1/webhooks/github` em produção.

---

## 4. Follow-ups técnicos / dívida

### 4.1 Migração `internal/entity/*` → `internal/core/*`

- **Origem:** `docs/architecture/04-modular.md §9`, passos 1-3. F-016
  executou passos 4-6 (controllers por módulo, httpserver, depguard).
- **Escopo:**
  - `internal/entity/node/urn*.go` → `internal/core/urn/`
  - `internal/entity/node/{meta,base,source,lineage,confidence}` →
    `internal/core/bitemporal/`
  - `internal/entity/node/kind.go` → `internal/core/kind/`
  - `internal/entity/edge/{edge.go,registry.go}` → `internal/core/edge/`
  - `internal/repository/repository.go` → `internal/core/repository/`
  - Structs concretos de `entity/node` → `modules/<plane>/node/`
  - Imports em massa.
- **Risco de adiar:** baixo. `depguard` cobre fronteiras críticas;
  imports atuais são longos mas estáveis. Cada novo Kind concreto em
  `entity/node` adiciona ~5 min de migração futura.
- **Gatilho:** transformar em F-NNN quando alguém precisar evoluir
  `core/` (novo Kind transversal, novo tipo de URN).

### 4.2 OpenAPI v1 (sync com shape RFC 7807) — ✅ feito 2026-05-18

- **Origem:** F-016 trocou o shape de erro para
  `application/problem+json` (`docs/api/v1/errors.md`).
- **Entregue:**
  - `ErrorBody` removido, schema `Problem` (RFC 7807) com
    `type/title/status/code/detail/trace_id` + `additionalProperties:
    true` (extras top-level).
  - Todas as responses 4xx/5xx servem `application/problem+json`.
  - Adicionados `Conflict` (409), `Unprocessable` (422), `Internal`
    (500) ao bloco `components.responses`.
  - `code` documentado com o conjunto conhecido (org.*, code.*,
    infra.*, gov.*, fallbacks); descrição reforça que códigos novos
    podem aparecer em minor — clientes devem tratar desconhecidos
    pelo `status`.
  - `errors.md` ganhou seção `gov` e o campo `trace_id`.
- **Resíduo:** consistência por endpoint (cada path declara só algumas
  das respostas possíveis — falta uniformizar 401 e 409 onde aplicável,
  e adicionar paths `/v1/governance/*` e `/v1/webhooks/github` que
  hoje existem em código mas não no openapi). Tratar como sub-item
  novo (§4.8) quando alguém precisar do contrato completo.

### 4.3 Docs de módulo faltantes

Template em `docs/modules/infra.md`. Faltam:

- `docs/modules/cost.md`
- `docs/modules/code.md`
- `docs/modules/org.md`
- `docs/modules/gov.md` (renomear `product.md` planejado)
- ~~`docs/modules/bridge.md`~~ — ✅ feito 2026-05-18 (cobre `Resolver`,
  `service_compute`, `github_webhook`).

**Gatilho:** quando o módulo ganhar a próxima feature `in_progress` —
evita doc preditivo que envelhece.

### 4.4 Controllers de `code` e `infra`

- **Origem:** F-016 deixou `modules/code` e `modules/infra` com
  `port.go` + `service/` + `errs.go` + `types.go`, mas sem
  `controller/` próprio (S-006/S-007 absorvidos por S-008 — Kinds
  acessíveis via `app/graph` e `app/search`).
- **Gatilho:** demanda por endpoint REST específico
  (`/v1/code/services/{urn}`, `/v1/infra/computes/{urn}` com lógica
  acima do grafo cru). Replicar playbook de `modules/org/controller/`.

### 4.5b `ce extract aws` escreve só em memory (efêmero)

- **Origem:** `cmd/cli/main.go:145-147` tem TODO explícito desde
  S-008: *"Backend memory por enquanto (ephemeral); wiring de Neo4j
  vem em story dedicada — basta trocar o repo aqui"*. F-001 fechou
  com este débito.
- **Sintoma:** pipeline end-to-end AWS → API está quebrado em
  produção. O collector roda, descobre EC2/EBS/S3/RDS/VPC/SG/LB
  corretamente, mas o `memory.New()` hardcoded faz tudo sumir ao
  final do processo. Endpoints `/v1/architecture/*` nunca veem
  nó de infra AWS real.
- **Comparação:** todos os outros 8 comandos do CLI
  (`extract code`, `extract codeowners`, `ingest hris`,
  `ingest openapi`, `bridge service-compute`, `gov *`, `allocate`,
  `allocate-shared`) já aceitam `--neo4j` e persistem. AWS é o
  único exemplo restante.
- **Trabalho estimado:** trivial. Replicar o padrão `buildRepos` de
  `cmd/api/main.go:147-168` em `runExtractAWS`. Sem mudança de
  contrato no collector (já recebe `repository.NodeRepository`).
- **Gatilho:** **agora** — primeiro uso real do CostEngine para
  visualizar infra extraída. Promover a F-NNN ou tratar como hotfix
  diretamente.

### 4.5 `BusinessArea` collector

- **Origem:** F-015 recortou o critério "GET /v1/business-areas com
  `team_count`/`person_count`" do MVP. Entity `BusinessArea` existe
  (E-008/F-024), mas:
  - HRIS (F-010) não popula BusinessArea — só Team/Squad/Person.
  - Sem endpoint `/v1/business-areas/{urn}/teams` agregador.
- **Gatilho:** consumer real pedir agrupamento acima de `Team`.

### 4.6 Web app — scaffold inicial (`costEngine/web/`)

- **Origem:** sessão 2026-05-18. SPA React + Vite + react-flow +
  react-query + Zustand para visualizar Services / Endpoints e o
  subgrafo de cada Endpoint (`/flow`).
- **Entregue:**
  - Routes `/services`, `/endpoints`, `/endpoints/:urn/*` (splat
    preserva `:` e `/` da URN).
  - TopBar com toggle de tema (Zustand + `localStorage`); LeftRail
    vertical (Services / Endpoints).
  - Páginas-lista com filtro client-side via `react-query`.
  - Endpoint detail renderiza `react-flow` com `MiniMap` + `Controls`;
    custom `CeNode` com header colorido por `kind`; layout
    determinístico por colunas-por-kind (`flow/layout.ts`).
  - API client `api/graph.ts` consome o novo
    `GET /v1/architecture/nodes?kind=...&limit=...` (registrado em
    `controller.go`, com 3 testes em `list_test.go`).
- **Pendente:**
  - Detail view de **Function** / **Service** / **Module** /
    **Framework** (hoje só Endpoint tem detail; outros ficam em 404
    se acessados por URL direta).
  - **Edge labels** no react-flow (hoje só tipo no `data`, sem render
    visual do label).
  - **Filtros no flow**: toggle por edge-type (esconder CONTAINS p/
    ver só o call chain, etc.).
  - **As-of selector** (timestamp picker) — backend já aceita
    `?as_of=`, falta UI.
  - **Estado vazio + erro**: tratamento amigável quando lista vier
    vazia, ou `/flow` falhar (hoje exibe spinner travado).
  - **CI / build**: web hoje só roda em dev (`npm run dev`). Definir
    deploy (servir estático via Go embed? deploy separado?).
  - **Auth**: zero. API hoje aceita header `X-Tenant` opcional; UI
    não envia nada.
- **Gatilho:** validação end-to-end com a sample-api foi suficiente
  para abrir o caminho. Próximas slices só quando consumer real
  pedir um detail-view ou filtro específico.

### 4.7 Seed dev `--seed-code` em `cmd/api`

- **Origem:** sessão 2026-05-18. Para abrir a web sem precisar rodar
  CLI separado, `cmd/api` ganhou `--seed-code <repoPath>` +
  `--seed-repo <repoName>` que aplicam `typescript.Collect` +
  `codesvc.Writer.Apply` na subida do servidor.
- **Risco:** scope-creep do `cmd/api` (devia ser só HTTP). Hoje só
  TS — extensão natural seria `--seed-go`, `--seed-aws`, virando uma
  matriz de flags.
- **Sugestão:** migrar para sub-comando do `ce` CLI
  (`ce dev serve --seed-code <path>`) quando aparecer a 2ª flag de
  seed. Por ora o flag atual é suficiente.
- **Gatilho:** segunda demanda de seed automático (Go, AWS, etc.).

### 4.8 OpenAPI — completude de paths e responses

- **Origem:** §4.2 (RFC 7807) tratou só o shape de erro. Sobrou:
  - Endpoints reais não documentados no `openapi.yaml`:
    `POST /v1/governance/*` (gov controller, F-024/F-025/F-026/F-027)
    e `POST /v1/webhooks/github` (F-013).
  - Cada path declara apenas um subconjunto de respostas; falta
    uniformizar (401 quando `--tenant-required`, 409 onde houver
    conflict, 422 para `org.reports.cycle`).
- **Risco de adiar:** baixo — clients fora do repo ainda não existem.
- **Gatilho:** primeiro consumer externo gerar SDK a partir do
  openapi e reclamar de path faltando.

---

## 5. Decisões diferidas (não bloqueiam)

- ~~Manifesto `costengine.yaml`~~ — ✅ decidido em
  [ADR-011](../architecture/decisions/ADR-011-costengine-yaml-manifest.md)
  (2026-05-18). Arquivo único, opcional, na raiz; absorve a seção
  `features` de ADR-009. Implementação concreta fica em feature
  dedicada quando a primeira dor aparecer (F-009 `manifest` strategy
  ou primeiro mono-repo em F-011/F-013).
- **PII expanded view** (F-015): endpoint que devolve email pleno
  exige ADR sobre política de privacidade. Adiar até primeiro
  consumer real pedir.
- **GraphQL vs REST** (F-014): manter REST até primeiro consumer
  pedir o contrário.
- **Shape `application/problem+json`** já é padrão da API v1 desde
  F-016 — não há flag de compat hoje. Eventuais clients existentes
  precisam adaptar.

---

## 6. Próximas ações sugeridas (sem fila firme)

Como não há feature `refined`, qualquer próxima sessão deve começar
por **escolher onde investir**. Sugestões (não-ordenadas):

1. **Promover backlog vivo a F-NNN** apenas se dor concreta apareceu.
   Ex.: usuário Python real → F-017 Python collector; primeiro
   mono-repo → F-011/F-013 com `costengine.yaml` (ADR-011 decidiu o
   formato, falta implementar parser + consumers).
2. **Docs de módulo (§4.3)** — falta `cost.md`, `code.md`, `org.md`,
   `gov.md`. Clonagem mecânica do template.
3. **OpenAPI — paths e responses completos (§4.8)** — documentar
   `/v1/governance/*` e `/v1/webhooks/github`, uniformizar respostas
   por endpoint. Só quando houver consumer externo.
4. **Migração `entity→core` (§4.1)** — só promover quando o custo de
   adiar passar do custo de migrar (~hoje ainda vale adiar).

Para retomar: ler este doc + `refinement-status.md`, escolher um
item, criar branch/PR e atualizar ambos os docs ao fechar.
