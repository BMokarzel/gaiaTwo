---
id: F-009
title: Bridge Service → Compute (RunsOn)
status: done
modules: [bridge, code, infra]
depends_on: [F-001, F-007]
modeling_impact: yes
adrs: [ADR-004]
epic: E-003
updated: 2026-05-14
---

# F-009 — Bridge Service → Compute (RunsOn)

## Problema

Sem ligar `Service` (código) a `Compute` (infra), o grafo tem duas
ilhas e o produto Arquitetura não consegue responder "onde roda esse
serviço?" ou "qual o custo deste serviço?". Edge `RUNS_ON` faz a
ponte e é a chave para tudo de E-003 em diante.

**Decisão de modelagem registrada em [ADR-004](../architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md):**
Service→Compute usa edge novo `RUNS_ON`, **não** overload de
`DEPLOYED_ON` (que continua significando Compute→Compute, p.ex.
Pod→Node).

**Para quem:** allocation engine (custo por service), produto
Arquitetura, agente.

**Dor sem ela:** custo até o `Compute` para, sem chegar em `Service`.

## Escopo

**Inclui:**
- Estratégias de resolução `Service → Compute` (em ordem):
  1. **Tag explícita** no recurso AWS: `Service=<service_name>` ou
     `service.urn=<urn>`.
  2. **Convenção de nome:** EC2 com `Name=svc-<service_name>-...`,
     EKS namespace = service.
  3. **Manifesto explícito** num arquivo `costengine.yaml` na raiz do
     repo do Service (futuro).
- Comando `ce bridge service-compute --account=<id>` que materializa
  os edges `RUNS_ON` com `confidence` e `source` (qual estratégia
  resolveu).
- Edge bitemporal: muda dono, versiona.

**NÃO inclui:**
- Lambda → Service (subkind compute distinto; futuro).
- Inferência por tráfego/logs.
- Bridge para Persistence/Network (futuro: edges `UsesPersistence`,
  `ExposedVia`).

**Precondições:**
- F-001 (`Compute` com tags preservadas).
- F-007 (`Service` existe).

## Toque no grafo

- **Lê:** `Compute` (tags), `Service` (existência por nome/URN).
- **Escreve:** edge `RUNS_ON(Service → Compute)` com
  `confidence`, `source`, `valid_from/valid_to`.
- **Novos Kinds/edges:** `RUNS_ON` adicionado ao registry
  (`internal/entity/edge/edge.go` + `registry.go`) e à matriz de
  adjacency em `docs/architecture/01-modeling.md` — ver
  [ADR-004](../architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md).
- **Bitemporal:** quando recurso muda de owner (tag muda), fecha edge
  e cria novo.

## Critérios de aceite

- [x] Dado Compute com tag `Service=payments` e Service de nome
      `payments` no grafo, quando `ce bridge service-compute`,
      então existe edge `RUNS_ON(payments → compute)` com
      `confidence=1.0` e `source=tag`.
      *(`TestApply_TagOpensEdge`, `TestAdapter_RoundTrip_TagOpensThenIdempotent`)*
- [x] Dado Compute com `Name=svc-payments-prod-01` e nenhum tag
      `Service`, quando bridge, então edge criado com `confidence=0.7`
      e `source=name_convention`.
      *(`TestApply_NameConventionFallback`, `TestResolveByNameConvention_Matches`)*
- [x] Dado Compute sem nenhum sinal, quando bridge, então não cria
      edge (não força).
      *(`TestApply_NoSignalNoEdge`)*
- [x] Mudar tag de `Service=payments` para `Service=billing`: edge
      antigo é fechado, novo criado para `billing`.
      *(`TestApply_TagChangeClosesOldOpensNew`, `TestAdapter_TagChange_ClosesAndOpens`, `TestIntegration_FullCycle`)*
- [x] Relatório final: total de Computes, quantos com edge, quantos
      órfãos, quantos ambíguos.
      *(CLI `runBridgeServiceCompute` imprime Stats; `TestIntegration_FullCycle` valida contagens)*
- [x] Service ambíguo (mesmo nome em 2+ URNs) NÃO emite edge novo nem
      fecha o corrente — registra em Output.Ambiguous para inspeção.
      *(`TestApply_AmbiguousServiceSkipsEverything`, `TestIntegration_AmbiguousServiceProtectsExisting`)*

## Riscos / incerteza

- **Service ambíguo.** Múltiplos Services com mesmo nome em domínios
  diferentes — exigir URN absoluta na tag, ou usar `Service.fqn` se
  modelado.
- **Falsos positivos por convenção.** `Name=svc-foo-...` pode bater
  serviço deletado. Filtrar para Services `valid_to IS NULL`.
- **Confidence policy.** Documentar tabela de scores para evitar
  números mágicos espalhados.
- **Drift.** Tags mudam silenciosamente; relatório precisa expor
  "edges fechados nesta run".

## Notas de implementação

- Pacote `internal/modules/bridge/service_compute/`.
- Reuso da abordagem de F-003: lookup em batch via
  `BridgeRepo.ListServicesByName` (1 query → mapa nome→URN).
- Engine pura (`Apply`) é determinística: ordena Computes por URN
  antes de processar; mesmo Input ⇒ mesmo Output (idempotência
  garante reprocessamento seguro).

## Decisões (implementação)

- **D1 — `RUNS_ON` como edge novo, não overload de `DEPLOYED_ON`.**
  Registrada em [ADR-004](../architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md).
  Service→Compute tem semântica e confidence policy distintas de
  Compute→Compute; manter separado evita filtro por kind em
  consumers e barateia produtos futuros (F-014).

- **D2 — Tabela de confidence fixa.**
  `tag=1.0`, `name_convention=0.7`, `manifest=1.0` (futuro). Valores
  hardcoded em constantes para evitar "magic numbers" espalhados;
  evolução requer mudança de constante + ADR-004 update.

- **D3 — Precedência tag → name → manifest.**
  Resolver tenta em ordem e a primeira vencedora consome. Não há
  voto/merge: estratégia mais forte (tag) sobrescreve a mais fraca
  (name). Operador que quer name_convention removendo tag explícita
  precisa remover a tag.

- **D4 — `service.urn=<URN absoluta>` é escape de ambiguidade.**
  Quando há 2+ Services com mesmo short name, `Service=<name>` vira
  inerte (engine registra ambiguidade e não muta estado). Operador
  resolve usando a tag `service.urn=urn:ce:code:...`. Não há
  desempate automático — silencioso pior que loud.

- **D5 — Longest-match para name convention.**
  Dado `Name=svc-billing-repo-prod-01` e service `billing-repo` no
  mapa, o resolver testa do prefixo mais longo para o mais curto
  (`billing-repo-prod-01`, `billing-repo-prod`, `billing-repo`,
  `billing`) e devolve o primeiro existente. Sustenta nomes com
  hífen sem exigir convenção de escape.

- **D6 — Ambíguo NÃO fecha edge corrente.**
  Política conservadora: se ambiguidade aparece (segundo Service
  homônimo criado), o engine preserva o edge atual em vez de
  fechá-lo. Razão: fechar destrói informação de dono que pode estar
  correta. Operador desambigua via tag e segue.

- **D7 — Órfão (Compute sem sinal) fecha edge se existia.**
  Distinto de ambiguidade: ausência total de sinal é interpretada
  como "Compute deixou de pertencer a algum Service" — emite Close
  com `Reason="orphaned"`. Reabrir é fácil: bastar adicionar tag de
  volta.

- **D8 — `DeterministicID(from, type, to, validFrom)` para edges.**
  Mesmo (Service, Compute) reaberto em momentos diferentes gera IDs
  diferentes — histórico do grafo registra ambos. Reabertura
  acidental no mesmo momento (mesmo `now`) é idempotente: ID
  colide, edge é reescrito.

## Verificação

```sh
# Unit (resolver + engine) + adapter contra repo memory.
go test ./internal/modules/bridge/service_compute/...

# Integration (drift + idempotência + ambiguidade end-to-end).
go test -run TestIntegration_ ./internal/modules/bridge/service_compute/...

# CLI smoke
ce bridge service-compute --account=urn:ce:aws:111:account/111 --dry-run
```

**Resultado esperado:**
- `TestResolveBy*` — 3 estratégias funcionam isoladamente; precedência tag>name; service deletado rejeitado.
- `TestApply_*` — open/close/orphan/ambiguous/idempotente/determinístico.
- `TestAdapter_*` — round-trip pelo repo memory: open → close → ambiguidade não muta estado.
- `TestIntegration_FullCycle` — 4 Computes (3 resolvidos + 1 órfão), drift muda donos, run idempotente pós-drift.

## Entregue em

- **S-001..S-008** (slicing interno): ADR-004 + edge `RUNS_ON` no
  registry, resolvers (tag/service.urn/name_convention) com
  longest-match, lookup de Services em batch com ambiguidade
  explícita, engine determinístico open/close/orphan/ambiguous, repo
  adapter sobre `NodeRepository`+`EdgeRepository`, CLI
  `ce bridge service-compute`, integration test full-cycle, docs +
  priorities.

## Impacto na modelagem

**Classe:** Modeling impact (novo edge type `RUNS_ON` no core).
Registrado em [ADR-004](../architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md).

**Mudanças concluídas:**
- `internal/entity/edge/edge.go`: `TypeServiceRunsOn = "RUNS_ON"`.
- `internal/entity/edge/registry.go`: adjacency
  `RUNS_ON: Service → Compute`.
- `internal/entity/edge/runs_on.go`: struct concreto
  `ServiceRunsOn{Base}`.
- `internal/repository/memory/memory.go`: `closeEdge` cobre o novo
  tipo (bitemporal close funciona).
- `docs/architecture/01-modeling.md`: linha `RUNS_ON` na tabela 3.1.
