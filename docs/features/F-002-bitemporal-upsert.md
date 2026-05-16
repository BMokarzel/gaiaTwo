---
id: F-002
title: Bitemporal Upsert no Neo4j
status: done
modules: [infra]
depends_on: []
modeling_impact: no
adrs: [ADR-002]
epic: E-001
updated: 2026-05-13
---

# F-002 — Bitemporal Upsert no Neo4j

## Problema
O grafo precisa preservar histórico: quando um recurso muda, a versão
anterior não desaparece — fica fechada com `valid_to`. Sem isso, queries
"como estava em 2026-03-01?" são impossíveis e atribuição de custo
retroativa quebra.

**Para quem:** repositório consumido por todos os outros módulos.

**Dor sem ela:** grafo perde história; CUR de mês passado não casa com
estado atual do grafo.

## Escopo

**Inclui:**
- `NodeRepository.Upsert(ctx, node)` em Neo4j que:
  - Fecha versão corrente (`valid_to = now` onde `valid_to IS NULL`).
  - Cria nova versão com `version++` e `valid_from = now`.
  - Tudo em transação única.
- `GetByURN(ctx, urn, AsOf)` retornando versão corrente ou as-of.
- `History(ctx, urn)` retornando todas as versões.
- `Delete(ctx, urn)` fechando versão corrente sem criar nova.
- Equivalente para edges (`EdgeRepository.Upsert/Delete`).

**NÃO inclui:**
- Compressão/arquivamento de versões antigas.
- Snapshots/views materializadas as-of.
- Conflict resolution multi-source (uma fonte por vez).

**Precondições:**
- Driver Neo4j configurado.
- Schema com índices em `(urn, version)` (existe em `schema.go`).

## Toque no grafo

- **Lê:** versão corrente do nó/edge alvo.
- **Escreve:** atualiza versão corrente + cria nova.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** é a feature em si.

## Critérios de aceite

- [x] Dado um nó novo, quando `Upsert`, então versão 1 é criada com
      `valid_from=now`, `valid_to=null`.
- [x] Dado nó com versão N, quando `Upsert`, então versão N tem
      `valid_to=now` e existe versão N+1.
- [x] Dado nó com 3 versões e `AsOf(t2)` entre v2 e v3, quando
      `GetByURN`, então retorna v2.
- [x] Dado `History`, então retorna versões em ordem cronológica.
- [x] Dado `Delete`, então versão corrente é fechada e Get current
      retorna ErrNotFound.
- [x] Operações concorrentes não criam versões duplicadas (constraint
      `(urn, version) UNIQUE`).

## Riscos / incerteza

- Performance de history em URNs com milhares de versões: paginar.
- Clock skew entre clientes: usar `nowFn()` injetável + UTC sempre.

## Notas de implementação

- Implementação em `internal/repository/n4j/node.go` e `edge.go`.
- Constraint Neo4j: `CREATE CONSTRAINT FOR (n:CeNode) REQUIRE (n.urn, n.version) IS UNIQUE`.
- `nowFn` em `mapper.go` é variável injetável para testes determinísticos.

## Impacto na modelagem

**Classe:** No impact (a feature *é* a modelagem bitemporal).
**ADR relacionada:** ADR-002 (a criar) — registrar bitemporal como
invariante de toda escrita.

## Stories

- [x] S-001 — Schema Neo4j com constraint (urn, version) UNIQUE
- [x] S-002 — Upsert de nó (close+create) em transação
- [x] S-003 — GetByURN current + as-of
- [x] S-004 — History
- [x] S-005 — Delete soft (close current)
- [x] S-006 — Equivalente para edges
- [x] S-007 — Testes de roundtrip + concorrência

## Verificação

Os ACs são exercitados em duas camadas:

- **Oráculo do contrato (memory):** `internal/repository/memory/memory_test.go`
  cobre todos os comportamentos bitemporais sem dependência externa.
- **Integration tests no Neo4j real (2026-05-14):**
  `internal/repository/n4j/integration_test.go` com `//go:build integration`
  cobre os 6 ACs ponta-a-ponta (Upsert versão, GetByURN AsOf, History,
  Delete, concorrência com constraint `(urn, version) UNIQUE`, edges) +
  os ACs de F-003 (GetByExternalID estrito, ARN↔short, AsOf, NotFound,
  ambiguous cross-account).

Para rodar:

```sh
docker run --rm -p 7687:7687 -e NEO4J_AUTH=neo4j/testpass neo4j:5
export NEO4J_TEST_URI=bolt://localhost:7687
export NEO4J_TEST_PASS=testpass
make test-integration
```

Sem `NEO4J_TEST_URI`, os testes skipam (não derrubam `make test`).
