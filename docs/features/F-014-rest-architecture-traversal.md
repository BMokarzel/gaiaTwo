---
id: F-014
title: REST /v1/architecture/* (traversal de grafo)
status: done
modules: [product, code, infra]
depends_on: [F-001, F-007]
modeling_impact: no
adrs: [ADR-001]
epic: E-006
updated: 2026-05-14
---

# F-014 — REST /v1/architecture/*

## Problema

Os produtos web (especialmente Arquitetura, Dashboard) e o agente
precisam consultar o grafo sem aprender Cypher e sem acoplamento ao
Neo4j. Faltam endpoints REST estáveis que cubram os casos comuns:
"qual a vizinhança deste nó?", "quem depende disso?", "qual o caminho
até o custo?".

**Para quem:** produto Arquitetura, produto Dashboard, agente (via
MCP wrapper), CLI.

**Dor sem ela:** todo consumer reescreve traversals e fica frágil a
mudança de schema.

## Escopo

**Inclui:**
- `GET /v1/architecture/nodes/{urn}` — nó corrente ou `?as_of=t`.
- `GET /v1/architecture/nodes/{urn}/neighbors?depth=N&edge_types=...`
  — vizinhança k-hop com filtros.
- `GET /v1/architecture/nodes/{urn}/history?limit=N` — versões em
  ordem cronológica.
- `GET /v1/architecture/search?q=...&kind=...` — busca textual
  (substring sobre URN + campos visíveis no MVP; full-text depois).
- `GET /v1/architecture/paths?from=urn1&to=urn2&max_hops=N` —
  caminhos entre dois URNs.
- Paginação cursor-based.
- Read-only.

**NÃO inclui:**
- Mutações (estão em F-012 para product; collectors para infra/code).
- GraphQL (avaliar quando consumers pedirem; REST primeiro).
- WebSocket de updates (futuro stream hub).

**Precondições:**
- Grafo populado (F-001 + F-007 já dão massa crítica para começar).
- Auth via middleware seam (ADR-001 semente).

## Toque no grafo

- **Lê:** todos os Kinds.
- **Escreve:** nada (read-only).
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** filtro `valid_to IS NULL` por padrão; `?as_of=t`
  habilita visão pontual.

## Critérios de aceite

- [x] `GET /v1/architecture/nodes/urn:ce:aws:1234:compute/i-0abc`
      retorna nó com propriedades e `version`, `valid_from`.
      *Cobertura:* `TestNodes_Get_*` em `internal/app/api/v1/nodes_test.go`.
- [x] Mesmo endpoint com `?as_of=2026-01-15T00:00:00Z` retorna versão
      vigente naquela data.
      *Cobertura:* `TestNodes_Get_AsOf` em `nodes_test.go`.
- [x] `GET .../neighbors?depth=2&edge_types=Contains,DeployedOn`
      retorna ≤ 100 vizinhos com edges incluídos.
      *Cobertura:* `TestNeighbors_*` em `nodes_test.go`; cap em
      `nodes.go` (`maxNeighborsResults=100`).
- [x] `GET .../history?limit=10` retorna versões em ordem cronológica
      decrescente com `valid_from/valid_to`.
      *Cobertura:* `TestHistory_*` em `nodes_test.go`.
- [x] `GET /v1/architecture/search?q=payments&kind=Service` retorna
      lista paginada (cursor).
      *Cobertura:* `TestSearch_*` em `search_paths_test.go`; cursor
      em `cursor.go`.
- [x] `GET /v1/architecture/paths?from=urn:A&to=urn:B&max_hops=5`
      retorna 0..N caminhos com URNs intermediários.
      *Cobertura:* `TestPaths_*` em `search_paths_test.go`; cap
      `maxPathsHops=5` em `search_paths.go`.
- [x] Auth ausente: 401 (em `--tenant-required`).
      *Cobertura:* `TestServer_TenantRequired_*` em `server_test.go`;
      `authMiddleware` em `middleware.go`.

## Decisões (implementação)

- **D1 — Routing por wildcard:** URNs contêm `/` e `:`, então registramos
  `GET /v1/architecture/nodes/{rest...}` e despachamos por sufixo
  (`/neighbors`, `/history`) dentro de `dispatchNode`. Mantém o stdlib
  `net/http` (Go 1.22) sem trazer chi/gorilla.
- **D2 — Cursor opaco base64+fingerprint:** `next_cursor` é
  `base64url(JSON{o, f})` com `f = sha256(q\x00kind)[:8]`. Wrapper sobre
  offset agora; troca por keyset depois sem mudança de contrato.
  Fingerprint rejeita reuso de cursor entre queries (400 em vez de
  retornar página silenciosamente errada).
- **D3 — Forward-compat de edge_types:** tipos desconhecidos são
  silenciosamente ignorados em `parseEdgeTypes`. Permite adicionar Kinds
  no core sem quebrar clients que ainda não atualizaram a whitelist.
- **D4 — Caps explícitos:** `depth ≤ 3` (neighbors), `max_hops ≤ 5`
  (paths), `≤ 100` vizinhos, `≤ 100` caminhos, `limit ≤ 200` (search),
  `limit ≤ 500` (history). Protege contra explosão acidental de
  traversal.
- **D5 — Tenant seam default:** em modo permissivo, ausência de
  `X-Tenant-ID` injeta `"default"`. Em `--tenant-required`, devolve 401.
  Honra ADR-001 sem amarrar a um IdP específico.
- **D6 — Repository extension dual:** `NodeRepository.Search` e
  `EdgeRepository.Paths` em **memory** (BFS determinístico, peers
  ordenados por URN) e **n4j** (Cypher `MATCH p=(a)-[:CE_EDGE*1..N]->(b)
  ORDER BY length(p) ASC, [n IN nodes(p) | n.urn] ASC LIMIT 100`).
- **D7 — RequestID middleware:** echo do header se válido
  (`hex/dash, ≤64 chars`), gera 8-byte hex caso contrário. Anexa em
  toda resposta e no envelope de erro.
- **D8 — Erros sem leak:** `writeRepoError` mapeia sentinelas do
  `repository` para HTTP. Apenas `ErrInvalidArgument` propaga `err.Error()`
  (mensagem já cuidadosamente formatada); demais classes usam mensagens
  genéricas para não vazar implementação.

## Verificação

```bash
# Suite completa do v1
go test ./internal/app/api/v1/...

# Suite do repository (memory + n4j integration)
go test ./internal/repository/...

# Build + smoke local (memory backend)
go build -o bin/ce-api ./cmd/api
./bin/ce-api --addr=:8080 &
curl -s localhost:8080/healthz
# {"status":"ok","version":"v1"}
```

OpenAPI publicada em `docs/api/v1/openapi.yaml` (3.0.3) — cobre os 5
endpoints + `/healthz`, security scheme `tenantHeader`, schemas
`NodeView`/`EdgeView`/`ErrorBody`, caps e convenções de cursor.

## Entregue em

- **S-001** — Extensão de `NodeRepository.Search` + `EdgeRepository.Paths`
  (memory BFS + n4j Cypher).
- **S-002** — Scaffold de `internal/app/api/v1`: server, routes,
  middlewares (RequestID/Recover/Auth), erros tipados, `/healthz`.
- **S-003** — `GET nodes/{urn}` + `neighbors` + `history` (dispatch por
  sufixo, BFS k-hop com caps, view helpers).
- **S-004** — `GET search` + `paths` (caps, parsers, view).
- **S-005** — Cursor opaco com fingerprint anti-cross-query.
- **S-006** — `cmd/api` binary: flags, signal handling, graceful
  shutdown 10s, slog JSON, escolha memory/neo4j.
- **S-007** — OpenAPI 3.0.3 em `docs/api/v1/openapi.yaml`.
- **S-008** — Status `done` + promoção de F-011 na priorities.

## Notas de implementação

- Pacote `internal/app/api/v1/`.
- Repository: usa `NodeRepository` + Cypher gerado de forma
  parametrizada (whitelist de edge_types).
- Não expor Cypher nem campos internos (`session_token`, `tenant_id`
  só nos lugares certos).
