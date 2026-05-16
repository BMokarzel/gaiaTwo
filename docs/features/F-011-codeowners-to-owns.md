---
id: F-011
title: CODEOWNERS → Person.Owns(Service)
status: done
modules: [org, code, bridge]
depends_on: [F-007, F-010]
modeling_impact: no
adrs: []
epic: E-004
updated: 2026-05-14
---

# F-011 — CODEOWNERS → Person.Owns(Service)

## Problema

Ownership real fica em CODEOWNERS dos repos. Sem ler isso, a ligação
Person/Team ↔ Service é manual e diverge da fonte da verdade. Como
GitHub já consome esse arquivo para PR review, manter alinhado é
barato e legítimo.

**Para quem:** allocation engine (dimensão `team` quando tag não
existir), produto Teams ("o que meu time é dono?"), bridge.

**Dor sem ela:** time mapeado por planilha que envelhece.

## Escopo

**Inclui:**
- Comando `ce extract codeowners --repo=<path>`.
- Parser de `CODEOWNERS` (sintaxe GitHub).
- Mapeamento `@org/team` → `Team` no grafo (match por nome do team).
- Mapeamento `@user` → `Person` (match por github_handle → Person via
  property `github_handle` em F-010 follow-up, OU email se conhecido).
- Cria edges `Owns(Person|Team → Service)` (Service identificado pelo
  repo → service URN, convenção: 1 repo = 1 service, ou via
  `costengine.yaml`).

**NÃO inclui:**
- Múltiplos CODEOWNERS por path (suportar apenas linha "global"
  primeiro; refinar depois).
- Owner em arquivo individual (granularidade módulo no MVP).
- Sync bi-direcional (atualizar CODEOWNERS a partir do grafo).

**Precondições:**
- F-007 (Service existe).
- F-010 (Person/Team existem).
- Convenção `repo → service URN` definida (manifesto ou config).

## Toque no grafo

- **Lê:** `Service` (por nome), `Team`, `Person`.
- **Escreve:** edge `Owns(Person|Team → Service)`.
- **Novos Kinds/edges:** edge `Owns` modelado em `03-business.md`.
- **Bitemporal:** mudar owner no CODEOWNERS fecha edge e cria novo.

## Critérios de aceite

- [x] Dado `CODEOWNERS` com `* @org/payments-team`, quando
      `ce extract codeowners --repo=<path>` e Team `payments-team`
      existe, então edge `Owns(payments-team → Service)` é criado.
      *Verificado:* `resolver_test.go::TestResolver_TeamHandle`,
      `pipeline_test.go::TestPipeline_TwoPassCloseReopen`.
- [x] Dado handle `@alice` que mapeia para Person com
      `github_handle=alice`, edge `Owns(alice → Service)` é criado.
      *Verificado:* `resolver_test.go::TestResolver_PersonHandle`,
      `pipeline_test.go::TestPipeline_TwoPassCloseReopen`.
- [x] Owner desconhecido (handle não bate) vai para relatório de
      "não resolvidos" sem falhar.
      *Verificado:* `resolver_test.go::TestResolver_Unresolved`,
      `pipeline_test.go::TestPipeline_UnresolvedNonFatal`,
      `writer_test.go::TestWriter_UnresolvedCounted`.
- [x] Mudar dono no arquivo e re-extrair: edge antigo fechado, novo
      criado.
      *Verificado:* `writer_test.go::TestWriter_CloseRemovedOwner`,
      `pipeline_test.go::TestPipeline_TwoPassCloseReopen` +
      `TestPipeline_ReopenAfterClose`.
- [x] Owner duplicado (Team + Person) cria os dois edges.
      *Verificado:* `writer_test.go::TestWriter_DuplicateOwnerCreatesTwoEdges`,
      `pipeline_test.go::TestPipeline_DuplicateOwnerBothEdges`.

## Riscos / incerteza

- **github_handle em Person.** F-010 não captura por padrão. Solução:
  property opcional adicionável depois; F-011 trata ausente como
  "não resolvível".
- **Repo → Service.** Convenção 1:1 quebra para mono-repos. Manifesto
  `costengine.yaml` com lista de Services é o destino — mas pesa para
  o MVP. Aceitar 1:1 e tratar mono-repos como follow-up.
- **CODEOWNERS sintaxe ampla.** Wildcards, negação, paths complexos —
  MVP só pega regras globais.

## Notas de implementação

- Pacote `internal/modules/org/ingest/codeowners/` (alinhado com o layout
  já adotado por `org/ingest/hris`).
- Parser próprio: a sintaxe alvo do MVP (linha global + handles) é trivial
  e evita dep externa.

## Stories

### S-001 — Edge `Owns` + Person.github_handle
**Comportamento:** Dado o catálogo de edges atual, quando o módulo
declara `edge.TypeOwns` com adjacência `[Person, Team] → [Service]`,
então `Validate` aceita o par e a persistência (memory + n4j) sabe
ler/escrever a aresta. Person ganha campo opcional `GithubHandle`
preservando idempotência do hash.
**Camadas tocadas:** entity, repository, persistence.

### S-002 — Parser CODEOWNERS (regra global)
**Comportamento:** Dado um arquivo `CODEOWNERS` com linhas de regra
`<path> @owner...`, quando o parser roda, então devolve uma lista de
regras com `Pattern` + `Owners[]`. Comentários (`#`) e linhas em branco
são ignorados. Linhas inválidas viram `RowError` não-fatal.
**Camadas tocadas:** parser, errors.

### S-003 — Resolver de owners
**Comportamento:** Dado um conjunto de URNs Person/Team já no grafo,
quando o resolver recebe `@org/team` ou `@user`, então retorna a URN
canônica via lookup por nome de team / `github_handle` de pessoa.
Handles não-resolvidos viram entrada em `Unresolved[]` sem abortar.
**Camadas tocadas:** resolver, repository read.

### S-004 — Convenção repo → Service URN
**Comportamento:** Dado um caminho de repo no disco e o grafo de
Services do plano de código, quando o emitter precisa do destino do
`Owns`, então deriva a Service URN aplicando convenção 1:1
(`repo basename → Service.Repo`). Mono-repo é detectado e relatado
para follow-up sem falhar globalmente.
**Camadas tocadas:** resolver, repository read.

### S-005 — Emit + Writer (close-and-reopen)
**Comportamento:** Dado um conjunto de owners resolvidos e a Service
alvo, quando o engine aplica, então edges `Owns` correntes são
upsertados deterministicamente. Edges não-mais-presentes na nova
extração são fechados (`valid_to`). Reaparecer um owner cria novo
edge sem reusar `valid_to` antigo.
**Camadas tocadas:** engine, repository write, bitemporal.

### S-006 — CLI `ce extract codeowners`
**Comportamento:** Dado o binário `ce`, quando `ce extract codeowners
--repo=<path> --tenant=<t>` roda, então o comando lê o CODEOWNERS,
resolve, aplica e imprime um sumário JSON (`resolved`, `unresolved`,
`opened`, `closed`). `--dry-run` não escreve no grafo.
**Camadas tocadas:** cli, integration.

### S-007 — Integration test full-cycle
**Comportamento:** Dado um repo de fixture com CODEOWNERS, quando
extraímos duas vezes (segunda com owner trocado), então:
(a) primeira passada abre N edges; (b) segunda fecha o antigo e abre
novo; (c) handle desconhecido reportado mas não falha; (d) owner
duplicado (Team + Person) cria dois edges; (e) re-execução sem mudança
é idempotente.
**Camadas tocadas:** integration, cli, repository.

### S-008 — F-011 done + priorities
**Comportamento:** Dado F-011 entregue, quando atualizamos docs,
então `status: done`, todos os critérios marcados com referência a
testes, seções "Decisões"/"Verificação"/"Entregue em", e
`priorities.md` reflete F-011 concluída + próxima feature promovida.
**Camadas tocadas:** docs.

## Decisões (implementação)

- **D1 — Identidade lógica do `Owns` por (From, Target).** O
  `DeterministicID` inclui `validFrom`, então cada extração gera ID
  novo. O `Writer` compara o conjunto corrente por `(from→target)` via
  `Neighbors(target, DirIn, Types=[OWNS])` e decide open/close/unchanged
  na hora. Mantém idempotência sem deduplicar por hash do conteúdo.
- **D2 — Person via `github_handle`, Team via slug.** Person ganhou
  campo opcional `GithubHandle` (`person.go`) — Resolver constrói um
  índice em memória *uma vez* via `Nodes.List(Kind=Person)` e devolve
  por busca exata case-insensitive. Team usa `node.NewTeamURN(tenant,
  Slugify(slug))` direto (URN canônica). Evita escanear todo o grafo
  por handle em cada lookup.
- **D3 — Convenção `repo → Service`: 1 repo = 1 Service raiz (`.`).**
  `ServiceLookup` tenta primeiro `GetByURN(NewServiceURN(basename, "."))`;
  se faltar, faz fallback `List(Kind=Service) | filter by Repo`. 0
  matches → `ErrServiceNotFound` (não-fatal). >1 matches →
  `Ambiguous[]` (mono-repo detectado, emit pula). Aceita Linux+Windows
  paths via `filepath.ToSlash` + `LastIndex("/")`.
- **D4 — Apenas regra global `*` no MVP.** Parser devolve `[]Rule`
  completas, mas `GlobalRule(rules)` filtra e o CLI só consome essa.
  Regras de path-pattern viram backlog (F-011 follow-up) — sem
  branchear estrutura de dados agora.
- **D5 — RowError não-fatal, ciclos resolvíveis.** Linhas inválidas no
  CODEOWNERS (owner sem `@`, regra sem owners) viram `RowError` na
  stderr mas o pipeline segue. Handles não-resolvidos viram
  `Unresolved[]` no `Result` — o writer conta como `Stats.Unresolved`
  sem abortar. Honra o princípio "best-effort com observabilidade"
  que F-010 estabeleceu.
- **D6 — Sumário JSON no stdout, warnings na stderr.** CLI imprime
  `summary` (`resolved`, `unresolved`, `opened`, `closed`,
  `unchanged`, `service`, `repo`, `owners_path`) com `MarshalIndent`.
  `errPartial` (exit 5) marca casos de `service: not_found` /
  `ambiguous` — distingue de erros fatais.
- **D7 — Duplicate Person+Team com mesmo nome são edges distintos.**
  Resolver dedup por URN; como `Person:alice` e `Team:alice` têm URNs
  diferentes, ambos passam. Validado por
  `TestPipeline_DuplicateOwnerBothEdges`.

## Verificação

```bash
# Suite completa do pacote codeowners (unit + integration)
go test ./internal/modules/org/ingest/codeowners/...

# Smoke do CLI com fixtures mínimas
go test ./cmd/cli/... -run TestRunExtractCodeowners

# Build do binário
go build ./cmd/cli/...

# Dry-run local (in-memory; espera ErrServiceNotFound sem Service)
./ce extract codeowners --repo=/path/to/repo --tenant=acme --dry-run
```

Adjacência `Owns` registrada em `internal/entity/edge/registry.go`:
`[Person, Team] → [Service]`. Pipeline determinístico por close-and-reopen
(F-011 D3). Backend memory + n4j (mapper já cobre `TypeOwns`).

## Entregue em

- **S-001** — Edge `TypeOwns` em `entity/edge`, struct `Owns`,
  adjacência matrix update, `Person.GithubHandle` + normalização. Cover
  unit + persistência (memory close-edge, n4j decode).
- **S-002** — Parser `CODEOWNERS` (`parser.go`): `Rule{LineNum, Pattern,
  Owners}`, comentários inline, validação de prefixo `@`, RowError
  não-fatal, helper `GlobalRule`.
- **S-003** — `Resolver` com índice in-memory para Person via
  `github_handle` (case-insensitive), Team via `NewTeamURN(tenant,
  Slugify(slug))`, dedup por URN preservando ordem.
- **S-004** — `ServiceLookup.ResolveForRepo` com atalho módulo raiz +
  fallback `List+filter`. `ErrServiceNotFound` sentinela. Detecção de
  mono-repo via `Ambiguous[]`. Cross-platform `repoBasename`.
- **S-005** — `Build` (Emit) + `Writer.Apply` com semântica
  close-and-reopen: identidade lógica por (from, target); abrir/manter/
  fechar pelo conjunto corrente em `Neighbors(target, DirIn)`.
- **S-006** — CLI `ce extract codeowners --repo --tenant [--dry-run]`,
  pipeline localiza/parse/resolve/lookup/build/apply, sumário JSON,
  exit codes 2/5 para usage/partial.
- **S-007** — `pipeline_test.go` integration full-cycle:
  TwoPassCloseReopen, ReopenAfterClose, DuplicateOwnerBothEdges,
  UnresolvedNonFatal.
- **S-008** — Doc finalizada: status=done, critérios marcados com
  referência a testes, `priorities.md` movendo F-011 para concluídas.
