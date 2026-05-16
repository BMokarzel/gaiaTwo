---
id: ADR-002
title: Bitemporalidade é invariante de toda escrita no grafo
status: accepted
date: 2026-05-13
related_features: [F-001, F-002, F-003]
supersedes: []
superseded_by: []
---

# ADR-002 — Bitemporalidade é invariante

## Contexto

O CostEngine precisa responder perguntas como:
- "Quanto custou o serviço X em março, considerando a infra como
  estava naquele mês?"
- "Quando este recurso mudou de tamanho?"
- "Mostre o grafo como estava antes do incidente de 15/abril."

CUR fecha em M+1 e é frequentemente reprocessado. Coletores rodam em
cadência variável (horária, diária). Ingest de fontes externas (HRIS,
Jira, Stripe) chegam atrasados, fora de ordem ou são corrigidos
retroativamente.

Sem versionamento temporal:
- Atribuir custo de CUR de março ao grafo de hoje produz erro
  sistemático.
- Auditoria de mudança vira impossível.
- Simulação "como teria sido se..." perde base.

A decisão sobre **como** versionar precisa ser tomada antes que
qualquer dado real seja ingerido — caso contrário, migração futura é
catastrófica.

## Decisão

**Toda entidade do grafo é bitemporal por contrato.** Não há entidade
"sem versão". Concretamente:

1. **Schema:** todo nó e toda aresta carregam:
   - `version` (int, incremental por URN/ID)
   - `valid_from` (timestamp UTC, obrigatório)
   - `valid_to` (timestamp UTC nullable — null = corrente)
   - `observed_at` (timestamp UTC — quando a fonte observou)
   - `source` (collector, run_id, method)
   - `confidence` (0..1)

2. **Toda escrita versiona:**
   - Fecha versão corrente: `UPDATE WHERE valid_to IS NULL SET valid_to = now`.
   - Cria nova versão: `version = anterior + 1`, `valid_from = now`,
     `valid_to = null`.
   - Operações ocorrem em **transação única** por URN.

3. **Toda leitura é temporal:**
   - Default = `current` (`valid_to IS NULL`).
   - `AsOf(t)` retorna versão vigente em `t` (`valid_from <= t AND (valid_to IS NULL OR valid_to > t)`).
   - `History(urn)` retorna todas as versões em ordem cronológica.

4. **Delete é soft:** fecha versão corrente (`valid_to = now`) sem
   criar nova. Não há remoção física de versões antigas no MVP.

5. **Constraint de unicidade:** `(urn, version)` é único globalmente
   por label. Isso impede duplicação em escritas concorrentes.

6. **Reprocessamento idempotente:** se uma escrita produz exatamente
   os mesmos valores da versão corrente, **não** cria versão nova —
   apenas atualiza `observed_at`. Hash determinístico dos campos
   significativos antes do diff.

## Alternativas consideradas

### Sem versionamento (overwrite)
Descartada. Reprocessar CUR de mês anterior contaminaria o estado
atual. Auditoria impossível.

### Versionamento apenas em entidades "importantes"
Descartada. Define-se "importante" caso a caso e a regra escapa.
Manter invariante uniforme é mais barato do que policiar exceções.

### Versionamento por snapshot (snapshot do grafo inteiro por período)
Descartada. Storage explosivo; queries pontuais ineficientes;
versionamento por entidade é mais granular.

### Append-only event log + projeção
Considerada. É essencialmente o que fazemos, mas com a projeção
*sendo* o grafo no Neo4j em vez de uma view materializada separada. O
outbox em Postgres (ver ADR-001 seção "sementes") já carrega o log de
eventos para consumidores externos. Adotar event sourcing puro adiciona
complexidade desproporcional no estágio atual.

### Bitemporal completo (transaction time + valid time separados)
Descartada para o MVP. A literatura distingue *valid time* (quando o
fato é verdadeiro no mundo real) de *transaction time* (quando o
sistema registrou). Aqui, `valid_from/valid_to` cobrem ambos com
suficiente fidelidade para os casos de uso. Promover para
verdadeiramente bitemporal (4 timestamps por entidade) só se o domínio
exigir reconstrução de "o que o sistema acreditava saber em data X".

## Consequências

### Positivas
- Auditoria nativa: toda mudança é rastreável.
- Reprocessamento de CUR antigo não corrompe estado atual.
- Simulação as-of é query, não esforço.
- Conflitos de concorrência rejeitados pelo constraint
  `(urn, version)`.
- Lineage (`Source.RunID`) por versão habilita debug "quem fez essa
  mudança?".

### Negativas / custo
- Toda query precisa filtrar por `valid_to IS NULL` (ou `AsOf`). Bug
  comum: esquecer o filtro e ver versões fechadas. Mitigação: encapsular
  no Repository; nunca expor Cypher cru para consumers.
- Storage cresce com versões. Mitigação: arquivamento de versões
  antigas em ClickHouse após N períodos (feature futura, não MVP).
- Performance de `History()` em URNs com muitas versões: paginação +
  index em `(urn, valid_from)`.
- Idempotência depende de hash determinístico — adicionar campo novo
  ao Kind exige cuidado para não fazer todo o grafo "mudar" de versão
  no próximo run.

### Quando reabrir
- Caso o domínio exija distinção entre *valid time* e *transaction time*
  (ex.: regulatório, "qual era nossa visão da realidade em X?"
  diferente de "o que era verdade em X?").
- Caso o custo de storage de versões antigas vire material.
- Caso surja uma fonte sem timestamp confiável e impossível inferir.

## Implementação atual

- Modelagem em `architecture/01-modeling.md §"Bitemporal Meta"`.
- Schema Neo4j em `internal/repository/n4j/schema.go` (constraint
  `(urn, version) UNIQUE` + index `node_urn_idx`).
- Upsert bitemporal em `internal/repository/n4j/node.go` e `edge.go`
  (close current + create new em transação).
- Idempotência (hash de campos significativos) é trabalho de cada
  collector — pendente, parte de F-001.
