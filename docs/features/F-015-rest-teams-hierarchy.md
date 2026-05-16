---
id: F-015
title: REST /v1/teams/* (hierarquia org)
status: done
modules: [org]
depends_on: [F-010]
modeling_impact: no
adrs: [ADR-001]
epic: E-006
updated: 2026-05-14
---

# F-015 — REST /v1/teams/*

## Problema

Produto Teams e Dashboard precisam consultar hierarquia organizacional
(BusinessArea → Team → Squad → Person) com filtros úteis: "quem está
no squad X?", "quais squads dentro do time Y?", "qual a árvore de
report de Z?". F-014 cobre o grafo "como grafo"; este endpoint dá
projeção amigável.

**Para quem:** produto Teams, Dashboard, agente.

**Dor sem ela:** consumidor reconstrói hierarquia via N chamadas a
F-014, perdendo performance e ergonomia.

## Escopo

**Inclui:**
- `GET /v1/business-areas` — listagem com counts.
- `GET /v1/business-areas/{urn}/teams` — teams da business area.
- `GET /v1/teams/{urn}` — detalhe (members count, squads).
- `GET /v1/teams/{urn}/squads` — squads do time.
- `GET /v1/squads/{urn}/members` — pessoas (com PII ofuscada conforme
  F-010).
- `GET /v1/people/{urn}` — perfil (ofuscado se solicitante não tem
  permissão).
- `GET /v1/people/{urn}/reports` — quem reporta a essa pessoa
  (transitivamente com `?depth=N`).
- Paginação cursor-based; suporte a `?as_of=t`.

**NÃO inclui:**
- Mutações (F-010 ingestion + ADR para CRUD futuro).
- Permissionamento granular por team (todos os consumers autenticados
  veem hierarquia ofuscada).
- Ligação com salário / capacity.

**Precondições:**
- F-010 (HRIS ingest) populou Person/Team/Squad.

## Toque no grafo

- **Lê:** `BusinessArea, Team, Squad, Person` + edges
  `PartOf/MemberOf/ReportsTo`.
- **Escreve:** nada.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** `?as_of=t` retorna estrutura no instante; default
  = corrente.

## Critérios de aceite

- [~] `GET /v1/business-areas` retorna lista paginada com
      `team_count` e `person_count` para cada.
      *Status:* **fora do MVP** — `BusinessArea` exige novo Kind +
      ingest. Registrado como follow-up em
      [pendencias.md](../backlog/pendencias.md). MVP entrega
      hierarquia a partir de `Team`.
- [x] `GET /v1/teams/{urn}` retorna time com lista de squads e
      contagem total de membros.
      *Cobertura:* `TestGetTeam_ReturnsDetailWithSquads` em
      `internal/app/api/v1/teams_test.go`;
      `TestOrgHierarchy_EndToEnd` em `org_integration_test.go`.
- [x] `GET /v1/squads/{urn}/members` retorna pessoas com `email`
      ofuscado (formato `a***@d***.com`) por padrão.
      *Cobertura:* `TestSquadMembers_ReturnsMaskedEmails` em
      `people_test.go`; integration verifica PII em
      `TestOrgHierarchy_EndToEnd`.
- [x] `GET /v1/people/{urn}/reports?depth=2` retorna árvore com até
      2 níveis.
      *Cobertura:* `TestPersonReports_DepthTwo_BuildsTree` +
      `TestPersonReports_DepthCap_400` + `TestPersonReports_Cycle_Truncated`
      em `reports_test.go`.
- [x] `?as_of=2026-01-01` em qualquer endpoint retorna estrutura
      vigente naquela data.
      *Cobertura:* `TestAsOf_SnapshotsHistoricalState` +
      `TestAsOf_InvalidValue_400` em `bitemporal_test.go`;
      `TestOrgHierarchy_CursorPagination_Stable` em
      `org_integration_test.go` valida invalidação de cursor entre
      `as_of` diferentes.
- [x] Pessoas com `valid_to` preenchido (ex-funcionários) não aparecem
      por padrão; aparecem se `?include_inactive=true` (e ainda
      com PII ofuscada).
      *Cobertura:* `TestIncludeInactive_HidesByDefault`,
      `TestIncludeInactive_RevealsTerminated`,
      `TestIncludeInactive_PersonDetail`,
      `TestIncludeInactive_InvalidValue_400`,
      `TestIncludeInactive_ReportsTreeShowsClosed` em
      `bitemporal_test.go`.
- [x] Auth ausente: 401.
      *Cobertura:* herdado de F-014 via `authMiddleware` em
      `middleware.go`; suite `TestServer_TenantRequired_*` em
      `server_test.go` cobre todas as rotas v1 — `/v1/teams*`,
      `/v1/squads*`, `/v1/people*` passam pela mesma cadeia.

## Decisões (implementação)

- **D1 — `BusinessArea` punted para follow-up.** O Kind não existe
  em F-010 (que só modelou `Team/Squad/Person`); introduzi-lo exige
  nova entrada em `node.Kind`, registry de adjacência, mapper n4j
  e ingest. Custo fora da promessa "no modeling impact". MVP entrega
  hierarquia a partir de `Team`. Follow-up registrado em
  `pendencias.md`.
- **D2 — PII uniforme via `EmailHint` pré-mascarado.** F-010 já
  persiste apenas `EmailHint` (`a***@d***.com`) e o hash SHA-256; o
  email pleno nunca chega ao repositório. Os endpoints expõem
  `email = EmailHint` direto, garantindo que `include_inactive=true`
  não cria caminho de vazamento. Endpoint "ver email pleno"
  continua punted por ADR separada.
- **D3 — Routing por wildcard reaproveitando F-014.** As rotas
  `/v1/teams/{rest...}`, `/v1/squads/{rest...}`, `/v1/people/{rest...}`
  despacham por sufixo em `dispatchTeam`/`dispatchSquad`/
  `dispatchPerson`. URNs com `/` e `:` não cabem em routers que
  esperam path-segments planos; stdlib `net/http` continua sem chi.
- **D4 — `fetchNode` com fallback via `History`.** `GetByURN(urn, AsOf)`
  retorna `ErrNotFound` para nó fechado quando `AsOf` é zero; com
  `?include_inactive=true` precisamos da última versão. Como
  `GetByURN` não aceita filtro, criei helper na camada de API que cai
  para `History(urn)` e retorna o último elemento. Mantém repositórios
  com contrato estreito; localiza a regra de "ressurreição" no único
  lugar que precisa dela.
- **D5 — Flag `IncludeInactive` em `NodeFilter`/`EdgeFilter`.** Para
  listas (`List`/`Between`/`Neighbors`), plumear via filter é mais
  limpo que adicionar parâmetro em cada método. Memory respeita via
  `pickEdgeWith` (retorna última versão quando corrente é nil); n4j
  remove `n.valid_to IS NULL` do WHERE quando flag ativa.
- **D6 — Subtree vazio é `[]`, não `null`.** `buildReportsSubtree`
  retorna `[]map[string]any{}` em base case (depth=0 ou sem
  subordinados); `nil` slice marshalava para `null` e quebrava
  consumers que iteram. Defensivo desde o início porque a árvore
  pode ser arbitrariamente rasa.
- **D7 — Detecção de ciclo via visited compartilhado.** `ReportsTo`
  é DAG por convenção mas o grafo não impõe; passamos um
  `map[node.URN]bool` por toda a recursão e setamos
  `truncated: true` quando detectamos ciclo (em vez de panic ou
  500). Cap de depth = 3 mitiga explosão mesmo sem ciclo.
- **D8 — Cursor com fingerprint de `as_of`.** O cursor opaco de F-014
  inclui o hash de `(q, kind, as_of)`; mudar `as_of` entre páginas
  rejeita o cursor com 400 ao invés de retornar página silenciosamente
  inconsistente. Aplicado em `/v1/teams` via reuso direto do helper
  de F-014.

## Verificação

```bash
# Suite completa do v1 (inclui org/bitemporal/integration)
go test ./internal/app/api/v1/...

# Suites dos repositórios (memory + n4j com IncludeInactive)
go test ./internal/repository/...

# Suite full
go test ./...
```

OpenAPI estendida em `docs/api/v1/openapi.yaml`: novos paths
`/v1/teams`, `/v1/teams/{urn}`, `/v1/teams/{urn}/squads`,
`/v1/squads/{urn}`, `/v1/squads/{urn}/members`, `/v1/people/{urn}`,
`/v1/people/{urn}/reports`; parâmetros reutilizáveis `Limit`,
`Cursor`, `IncludeInactive`; schemas `TeamView`, `SquadView`,
`PersonView`, `ReportNode`, `ReportTree`.

## Entregue em

- **S-001** — Scaffold do pacote org dentro de `internal/app/api/v1`:
  rotas `/v1/teams*`, `/v1/squads*`, `/v1/people*` com dispatch
  por sufixo, handlers stub 501.
- **S-002** — `GET /v1/teams` (paginado, counts) + `GET /v1/teams/{urn}`
  (detalhe com `squads[]`).
- **S-003** — `GET /v1/teams/{urn}/squads` + `GET /v1/squads/{urn}`
  (member_count + team_urn).
- **S-004** — `GET /v1/squads/{urn}/members` + `GET /v1/people/{urn}`
  com PII via `EmailHint` mascarado e team_urn transitivo.
- **S-005** — `GET /v1/people/{urn}/reports?depth=N`, cap depth=3,
  detecção de ciclo com `truncated`.
- **S-006** — `?as_of=t` + `?include_inactive=true` em todos os
  endpoints; `IncludeInactive` plumbed por `NodeFilter`/`EdgeFilter`
  em memory + n4j; helper `fetchNode` com fallback via `History`.
- **S-007** — OpenAPI 3.0.3 estendida + 2 integration tests
  end-to-end cobrindo hierarquia completa e estabilidade de cursor
  entre `as_of`.
- **S-008** — F-015 `status: done`, priorities atualizada,
  follow-up `BusinessArea` registrado.

## Riscos / incerteza

- **Performance em business areas grandes.** Counts via traversal
  pode ser caro; cache 5min razoável.
- **PII policy.** Política inicial: tudo ofuscado para todos os
  consumers desta versão. Endpoint "ver email pleno" requer ADR
  separada.
- **Depth em reports.** Recursão pode ser custosa; cap em `depth=3`.
- **Sincronia com F-010.** Re-ingestão pode invalidar caches em
  /v1/teams; aceitar TTL.

## Notas de implementação

- Pacote `internal/app/api/v1/teams/`.
- Compartilha middleware/auth seam com F-014.
- Resposta segue mesmo padrão de paginação cursor (consistência).

## Escopo MVP — recortes de slicing

- **BusinessArea fora do MVP.** F-010 modelou apenas `Team/Squad/Person`;
  introduzir `BusinessArea` exige nova `node.Kind`, adjacência em
  registry, mapper n4j e ingest. Custo fora da promessa de "sem
  modeling impact". `GET /v1/business-areas*` vira **follow-up
  feature** dedicada (a ser registrada na backlog). MVP entrega
  hierarquia a partir de `Team` (topo corrente do grafo org).
- **PII obfuscada para todos.** Política inicial é uniforme: email
  mascarado (`a***@d***.com`) via `node.MaskEmail` já existente.
  Endpoint "ver email pleno" continua punted por ADR separada
  (consistente com F-010).

## Stories

### S-001 — Scaffold pacote `teams` + repo helpers
**Comportamento:** Dado o server v1 existente, quando o pacote
`internal/app/api/v1/teams/` é montado, então o server registra as
rotas `/v1/teams*`, `/v1/squads*`, `/v1/people*` (handlers retornando
501 stub) e o serviço sobe sem regressão nos endpoints F-014. Repo
ganha helpers `MembersOf(squadURN, AsOf)` e `SquadsOf(teamURN, AsOf)`
se a forma atual de `Neighbors` não cobrir com ergonomia (decidir
durante implementação).
**Camadas tocadas:** api, repo, server.

### S-002 — GET /v1/teams + /v1/teams/{urn}
**Comportamento:** Dado `Team` nodes correntes, quando
`GET /v1/teams?limit=N&cursor=…` é chamado, então retorna lista
paginada (cursor opaco compartilhado com F-014) com `urn, name, slug,
squad_count, person_count` para cada. `GET /v1/teams/{urn}` retorna o
mesmo objeto + array `squads[]` (urns) e contagens transitivas.
**Camadas tocadas:** api, repo read.

### S-003 — GET /v1/teams/{urn}/squads + /v1/squads/{urn}
**Comportamento:** Dado `Squad` ligado a `Team` via `PartOf`, quando
`GET /v1/teams/{urn}/squads` é chamado, então retorna squads paginados
do team. `GET /v1/squads/{urn}` retorna detalhe com `member_count` e
team owner. Ordem determinística por URN.
**Camadas tocadas:** api, repo read.

### S-004 — GET /v1/squads/{urn}/members + /v1/people/{urn}
**Comportamento:** Dado `Person` ligadas a `Squad` via `MemberOf`,
quando `GET /v1/squads/{urn}/members` é chamado, então retorna pessoas
com `urn, name, email_obfuscated, role`. `GET /v1/people/{urn}`
devolve perfil com squad/team ancestrais e email mascarado por padrão.
PII via `node.MaskEmail` (formato `a***@d***.com`).
**Camadas tocadas:** api, repo read, privacy.

### S-005 — GET /v1/people/{urn}/reports?depth=N
**Comportamento:** Dado `ReportsTo` edges entre Persons, quando
`GET /v1/people/{urn}/reports?depth=2` é chamado, então retorna árvore
até 2 níveis com pessoas mascaradas. `depth` capado em 3; default 1.
Loop em cadeias circulares (defesa) interrompido com flag no
response.
**Camadas tocadas:** api, repo traversal.

### S-006 — `?as_of=t` + `?include_inactive=true`
**Comportamento:** Dado o grafo bitemporal, quando qualquer endpoint
`/v1/teams*|/squads*|/people*` recebe `?as_of=2026-01-01`, então as
queries propagam `AsOf` para `NodeRepository` e `EdgeRepository`.
`?include_inactive=true` inclui pessoas com `valid_to` preenchido
(ainda com PII ofuscada). Default: corrente + ativos.
**Camadas tocadas:** api, repo bitemporal.

### S-007 — OpenAPI + integration tests
**Comportamento:** Dado os endpoints implementados, quando rodamos
`go test ./internal/app/api/v1/...`, então existem integration tests
end-to-end cobrindo: lista vazia, paginação cursor, hierarquia
completa (team → squads → members → reports), PII mascarada,
`as_of` retornando snapshot histórico, `include_inactive` toggle.
`docs/api/v1/openapi.yaml` ganha schemas `Team`, `Squad`, `Person`,
`ReportTree` e os 7 endpoints novos.
**Camadas tocadas:** integration, docs.

### S-008 — F-015 done + priorities
**Comportamento:** Dado F-015 entregue, quando atualizamos docs, então
`status: done`, todos os critérios marcados com referência a testes,
seções Decisões/Verificação/Entregue em, `priorities.md` reflete
F-015 concluída + próxima feature promovida + follow-up
`BusinessArea` registrada na backlog.
**Camadas tocadas:** docs.
