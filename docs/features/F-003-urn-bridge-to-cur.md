---
id: F-003
title: URN Bridge para AWS CUR
status: done
modules: [infra, cost, bridge]
depends_on: [F-001]
modeling_impact: no
adrs: [ADR-002]
epic: E-002
updated: 2026-05-14
---

# F-003 — URN Bridge para AWS CUR

## Problema
O AWS Cost and Usage Report (CUR) traz linhas por `resource_id` nativo
(ex.: `i-0abc1234`, `arn:aws:rds:...`). Para atribuir custo a recurso
do grafo, é necessário resolver `(provider, account, resource_id)` →
`URN` do nó correspondente. Sem essa ponte, o custo fica solto.

**Para quem:** ingestor de CUR (futuro `cost-ingest-svc`) e qualquer
allocation engine.

**Dor sem ela:** CUR vira tabela paralela que não conversa com o grafo.

## Escopo

**Inclui:**
- `NodeRepository.GetByExternalID(ctx, providerID, account, externalID) → URN`.
- Resolver "external_id" = ARN quando disponível, fallback para ID
  nativo (`i-...`, `vol-...`, etc.).
- Index Neo4j em `(provider, account, external_id)` para lookup O(log n).
- Tolerância a recurso ausente: retorna `ErrNotFound` (caller decide
  se fila como unresolved ou ignora).

**NÃO inclui:**
- Parser de CUR (vive em F-004 / `cost-ingest`).
- Allocation engine (vive em F-005).
- Resolução heurística de recursos sem external_id (ex.: data transfer
  charges — F-006).

**Precondições:**
- F-001 (discovery povoa external_id).
- Schema Neo4j com index `node_external_idx` (já existe em `schema.go`).

## Toque no grafo

- **Lê:** `CeNode WHERE provider=$p AND external_id=$eid AND account CONTAINS $acct AND valid_to IS NULL`.
- **Escreve:** nada (read-only feature).
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** retorna versão corrente; futuras versões com `AsOf(t)`.

## Critérios de aceite

- [x] Dado `Compute` com `external_id="i-0abc1234"` versão corrente,
      quando `GetByExternalID("aws", "1234", "i-0abc1234")`, então
      retorna URN correto.
- [x] Dado mesmo recurso após Upsert (versão 2), quando
      `GetByExternalID`, então retorna URN (mesma) com versão corrente.
- [x] Dado recurso deletado (`valid_to` preenchido), quando
      `GetByExternalID`, então retorna `ErrNotFound`.
- [x] Dado external_id que não existe, quando lookup, então
      `ErrNotFound`.
- [x] Performance: 10k lookups em 5s em grafo de 100k nós (index hit).
      Medido em `bench_test.go` — impl memory resolve em ~11 ms.

## Riscos / incerteza

- **ARN vs ID nativo no CUR.** Algumas linhas têm `lineItem/ResourceId`
  como ARN, outras como ID curto. Discoverer (F-001) precisa preencher
  ambos ou normalizar para um deles. **Decisão pendente.**
- **Recursos cross-account** (ex.: bucket S3 acessado de várias contas):
  CUR atribui a uma conta; grafo guarda o "dono". Casar pela conta do
  CUR.
- **Recursos "shared infrastructure"** (data transfer, support):
  external_id pode vir vazio. Tratar em feature dedicada.

## Notas de implementação

- Implementação atual em `internal/repository/n4j/node.go`:
  `GetByExternalID`.
- Resource interface tem `ExternalID()` (Fase 0).
- Query Cypher usa `account CONTAINS $acct` para suportar account em
  URN ou ID nativo — revisar se precisa de duas formas separadas.

## Impacto na modelagem

**Classe:** No impact. A ponte já é coberta pelos campos `provider`,
`account`, `external_id` flatten em props (ver `01-modeling.md` e
`02-cost.md §"Ponte URN"`).

## Stories

### S-001 — Scaffold do pacote bridge + contrato `GetByExternalID` ✅
**Comportamento:** Dado um caller que importa `internal/modules/bridge`, quando chama `bridge.ResolveURN(ctx, provider, account, externalID)`, então recebe `(URN, error)` — implementação inicial é wrapper sobre `NodeRepository.GetByExternalID` já existente, com assinatura estável.
**Camadas tocadas:** [modules/bridge, repository]
**Notas:** Story zero. Foca em estabelecer o ponto de entrada único que o futuro F-004 vai consumir. Sem mudança de comportamento — apenas wrapper.

**Entregue (2026-05-14):**
- `internal/modules/bridge/bridge.go` define `Resolver` interface +
  `repoResolver` que delega ao `NodeRepository`.
- Política estrito → fallback wildcard documentada no godoc do package.
- Testes em `bridge_test.go` cobrem match estrito de short ID.

### S-002 — Normalização de external_id (ARN ↔ ID curto) ✅
**Comportamento:**
- Dado nó `Compute` indexado por ARN completo, quando caller chama com ID curto (`i-0abc...`), então retorna URN correto.
- Dado nó indexado por ID curto, quando caller chama com ARN, então retorna URN correto.
**Camadas tocadas:** [modules/bridge, repository]
**Blocked by:** [S-001]
**Notas:** Resolve a decisão pendente listada em "Riscos". Padroniza para indexar **ambas** as formas (ARN + ID nativo) no upsert; bridge consulta a forma recebida. Pode exigir backfill em grafo existente (script único).

**Entregue (2026-05-14):**
- `shortIDFromExternal` helper em `memory` e `n4j` extrai o último
  segmento de um ARN (após `/` ou `:`). Não-ARN retorna `""`.
- `memory.Repo.Upsert` indexa o nó duas vezes: pelo `ExternalID()` cru e
  pelo short ID derivado, no mesmo set de URNs.
- `n4j.nodeProps` grava propriedade `short_id` quando o input é ARN;
  novo index `node_short_id_idx` em `(provider, short_id)`.
- `n4j.GetByExternalID` query usa `WHERE (n.external_id = $eid OR
  n.short_id = $eid)` — ambidirecional.
- Testes `TestResolveURN_ARNToShortID` e `TestResolveURN_S3BucketARN`
  validam ambos os sentidos.

### S-003 — Lookup respeita bitemporal: corrente vs `AsOf(t)` ✅
**Comportamento:**
- Dado recurso com versão 1 fechada (`valid_to` preenchido) e versão 2 corrente, quando `ResolveURN(externalID)` sem `t`, então retorna URN da versão corrente.
- Dado o mesmo recurso, quando `ResolveURN(externalID, AsOf=t1)` com `t1 < valid_to(v1)`, então retorna URN da versão 1.
- Dado recurso totalmente removido (todas as versões fechadas, hoje), quando `ResolveURN` sem `t`, então retorna `ErrNotFound`.
**Camadas tocadas:** [modules/bridge, repository]
**Blocked by:** [S-001]
**Notas:** Cobre os critérios 2 e 3. Requer adicionar parâmetro `AsOf` opcional na assinatura.

**Entregue (2026-05-14):**
- Interface `NodeRepository.GetByExternalID` ganhou parâmetro
  `as repository.AsOf` (5 args).
- Memory impl coleta candidatos e filtra por `versionAsOf` quando
  `!as.IsZero()`; senão usa `currentVersionIdx`.
- n4j impl adiciona cláusula `n.valid_from <= $at AND (n.valid_to IS
  NULL OR n.valid_to > $at)` quando AsOf não-zero.
- Recurso totalmente fechado: `Delete` marca `valid_to`, lookup corrente
  acha 0 candidatos → `ErrNotFound`.
- Testes: `TestResolveURN_AsOfPastVersion`,
  `TestResolveURN_DeletedReturnsNotFound`,
  `TestResolveURN_UnknownReturnsNotFound`.

### S-004 — Performance: lookup em lote + benchmark ✅
**Comportamento:** Dado grafo com 100k nós e 10k external_ids para resolver, quando executar `bridge.ResolveURNBatch(ids)`, então retorna mapa `externalID → URN` em ≤ 5s (com index hit).
**Camadas tocadas:** [modules/bridge, repository, benchmark]
**Blocked by:** [S-002]
**Notas:** Cobre o critério de performance. Benchmark fica em `internal/modules/bridge/bench_test.go`. Pode exigir tunning de Cypher (`UNWIND $ids` em vez de N queries).

**Entregue (2026-05-14):**
- `Resolver.ResolveURNBatch` retorna `(map[string]URN, []ResolveError,
  error)`: resultados parciais + falhas não-fatais por externalID + erro
  fatal (ctx).
- Benchmark `BenchmarkResolveURNBatch_10kIn100k` em `bench_test.go`.
- `TestResolveURNBatch_PerfBudget` falha o build se o tempo > 5s — corre
  como teste regular (sem `-bench`).
- Resultado atual no oráculo memory: ~11 ms para 10k lookups em 100k
  nós. Implementação n4j ainda terá que ser validada com Cypher
  `UNWIND` — fica para quando o backend Neo4j estiver wired.

### S-005 — Tratamento de external_id ambíguo / multi-conta ✅
**Comportamento:**
- Dado bucket S3 (recurso global) onde o CUR atribui à conta A, mas o grafo tem o nó sob conta B (dona), quando `ResolveURN("aws", "A", bucket_arn)`, então retorna URN do nó (independente de account no grafo) — desde que `external_id` bate.
- Dado dois recursos com mesmo `external_id` em contas diferentes (não deveria acontecer com ARN, mas pode com ID curto cross-account), quando lookup, então retorna `ErrAmbiguous` com lista de candidatos.
**Camadas tocadas:** [modules/bridge, repository]
**Blocked by:** [S-002]
**Notas:** Documentar política em ADR se decisão for não-trivial. Cobre o risco listado de "recursos cross-account".

**Entregue (2026-05-14):**
- Novo sentinel `repository.ErrAmbiguous`.
- Política de duas etapas no `bridge`: estrito por account → fallback
  para wildcard se `ErrNotFound`. `ErrAmbiguous` no estrito *não*
  cai pra fallback (caller decide).
- Repositories: account=="" → wildcard cross-account; >1 match retorna
  `ErrAmbiguous` com contagem. n4j usa `LIMIT 2` para detectar sem custo.
- Testes: `TestResolveURN_CrossAccountFallback`,
  `TestResolveURN_AmbiguousAcrossAccounts`.
- Decisão registrada no godoc de `bridge` (sem ADR — política é simples
  e revogável).
