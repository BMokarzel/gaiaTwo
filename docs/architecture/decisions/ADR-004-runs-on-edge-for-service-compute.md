---
id: ADR-004
title: Service → Compute usa edge novo RUNS_ON, não overload de DEPLOYED_ON
status: accepted
date: 2026-05-14
related_features: [F-009]
supersedes: []
superseded_by: []
---

# ADR-004 — `RUNS_ON` como edge Service→Compute

## Contexto

F-009 (Bridge Service↔Compute) precisa materializar a relação
"este serviço de código roda neste recurso de infra" no grafo. A
spec original assumia `DEPLOYED_ON` reaproveitado para
`Service → Compute`.

Estado real ao iniciar F-009:

- `internal/entity/edge/edge.go` define
  `TypeDeployedOn Type = "DEPLOYED_ON" // compute → host/cluster`.
- `internal/entity/edge/registry.go` declara adjacency
  `From: [Compute], To: [Compute]` — semântica é
  **Compute→Compute** (Pod→Node, container→host).
- `docs/architecture/01-modeling.md` só usa `DEPLOYED_ON` num
  diagrama de exemplo, sem fixar adjacency.

Surgiu a escolha de modelagem: **estender `DEPLOYED_ON` para também
significar Service→Compute, ou criar um edge novo?**

## Decisão

**Criar edge `RUNS_ON` (Service → Compute)**, distinto de
`DEPLOYED_ON` (Compute → Compute). Concretamente:

1. **Constant nova:**
   `TypeServiceRunsOn Type = "RUNS_ON" // Service → Compute`.

2. **Adjacency:**
   `From: [Service], To: [Compute]`. `DEPLOYED_ON` permanece
   Compute→Compute, intocado.

3. **Confidence policy por source:**
   - `tag` (Service=name ou service.urn=urn) → `confidence=1.0`
   - `name_convention` (Name=svc-<name>-...) → `confidence=0.7`
   - `manifest` (futuro, costengine.yaml) → `confidence=1.0`

4. **Bitemporal:** edge tem `valid_from/valid_to`. Quando dono do
   Compute muda (tag mudou de `Service=A` para `Service=B`), o
   edge antigo é fechado (`valid_to = now`) e um novo é aberto.

5. **`01-modeling.md`** ganha entrada explícita de `RUNS_ON` na
   matriz de adjacency (até hoje só estava no exemplo).

## Alternativas consideradas

### A — Sobrecarregar `DEPLOYED_ON` (Compute → Compute + Service → Compute)
Descartada. Pró: zero tipos novos, adjacency aditiva. Contra: mesmo
edge type carregaria duas semânticas (Pod→Node determinístico vs
Service→Compute heurístico), com tabelas de confidence diferentes.
Queries de traversal teriam que filtrar por `fromKind` para
desambiguar — ruído que se paga em todo produto consumidor (F-014
REST, agente, custo por service). Dívida silenciosa.

### B — Novo `RUNS_ON` (escolhida)
Pró: cada edge type tem uma semântica única, uma regra de
confidence, um caso de teste. O próprio `01-modeling.md:241` já
trata `DEPLOYED_ON` e `RUNS_ON` como conceitos separados na
traversal de exemplo (`:DEPLOYED_ON|CONTAINS|RUNS_ON*1..4`) — a
intenção original era separar. Contra: novo Kind de aresta exige
ADR e update na matriz oficial de adjacency.

### C — `IMPLEMENTED_BY` ou `RUNS_AS` invertido
Descartada. `RUNS_ON` é a direção natural (Service depende de
Compute para existir, não o contrário) e casa com a metáfora de
deployment. Inverter complica traversals típicas
("custo do service" parte do Service, não do Compute).

## Consequências

### Positivas
- **Semântica única por edge.** Sem ramificação por kind em
  consumers.
- **F-009 ganha confidence policy clara** sem misturar com
  invariantes de discovery (F-001) que sempre emite confidence=1.0.
- **Traversal de custo** (`Service → Compute → Cost`) usa o tipo
  certo desde o início. F-014 expõe sem mascarar.
- **Reversível em direção contrária:** se aparecer caso de uso
  para unificar, basta um ADR superseding e migration aditiva
  (renomear `RUNS_ON` para `DEPLOYED_ON` no registry).

### Negativas / custo
- **Novo Kind de aresta.** Mais um tipo na matriz, mais um
  caminho em queries de adjacency.
- **Update em `01-modeling.md`.** F-009 deixa de ser
  `modeling_impact: no` — corrigido em frontmatter.
- **Coletores futuros precisam saber a distinção** (não emitir
  `DEPLOYED_ON` para Service→Compute por engano). Mitigação:
  registry valida adjacency e rejeita.

### Quando reabrir
- Se aparecerem 3+ edge types similares (`DEPLOYED_ON`,
  `RUNS_ON`, e.g. `EXECUTED_BY`) com semântica próxima, talvez
  valha consolidar via property em vez de tipo.
- Se a confidence policy de `RUNS_ON` convergir para sempre 1.0
  (manifestos universais), distinção heurística vs determinística
  deixa de existir e overload barateia.

## Implementação atual

- Constant: `internal/entity/edge/edge.go` →
  `TypeServiceRunsOn = "RUNS_ON"`.
- Adjacency: `internal/entity/edge/registry.go` → entry
  `TypeServiceRunsOn: {From: [Service], To: [Compute]}`.
- Pacote: `internal/modules/bridge/service_compute/` (F-009).
- Doc: `docs/architecture/01-modeling.md` ganha linha
  `RUNS_ON: Service → Compute (bridge, F-009)`.
