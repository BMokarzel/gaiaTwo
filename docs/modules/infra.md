---
id: M-infra
title: Módulo de Infra
status: stable
plane: infra
package: internal/entity/node + internal/entity/edge + internal/repository
updated: 2026-05-13
---

# Módulo `infra`

> Modela recursos de infraestrutura (cloud, on-prem, K8s) como nós do
> grafo. É o plano mais maduro do CostEngine — referência para os
> outros módulos.

## Propósito

Representar **o que existe** em termos de infraestrutura observável:
contas, regiões, instâncias de compute, bancos, redes, filas. Mantém
versão temporal de cada recurso e bridge para CUR via `ExternalID()`.

## Escopo

**Inclui:**
- Nós: `Provider`, `Account`, `Region`, `Zone`, `Environment`,
  `Compute`, `Persistence`, `Messaging`, `Network`.
- Edges intra-plano: `Contains` (hierarquia provider→account→region→zone),
  `DeployedOn`, `AttachedTo`, `Routes`, `Peers`.
- Adjacency matrix em `internal/entity/edge/registry.go`.
- Repository com bitemporal upsert (memory + Neo4j).

**NÃO inclui:**
- Lógica de coleta (vai para `internal/modules/infra/collector/` quando
  Fase 2 entrar).
- Linkagem com código (vive em `bridge/`).
- Custo associado (vive em `cost/` + alocação via `bridge/`).

## Entidades (Kinds)

| Kind | URN | Resource? | Sub-types |
|---|---|---|---|
| `Provider` | `urn:ce:<p>:0:provider/<id>` | sim | AWS, GCP, Azure, K8s, OnPrem |
| `Account` | `urn:ce:<p>:<acct>:account/<id>` | sim | — |
| `Region` | `urn:ce:<p>:<acct>:region/<id>` | sim | — |
| `Zone` | `urn:ce:<p>:<acct>:zone/<id>` | sim | — |
| `Environment` | `urn:ce:<p>:<acct>:environment/<id>` | não | dev, staging, prod |
| `Compute` | `urn:ce:<p>:<acct>:compute/<id>` | sim | VM, container, function, cluster, batch |
| `Persistence` | `urn:ce:<p>:<acct>:persistence/<id>` | sim | block, object, file, rdbms, nosql, cache, warehouse |
| `Messaging` | `urn:ce:<p>:<acct>:messaging/<id>` | sim | queue, topic, stream, broker |
| `Network` | `urn:ce:<p>:<acct>:network/<id>` | sim | vpc, subnet, lb, gateway, peering, endpoint, eni |

## Edges intra-plano

| Edge | From → To | Significado |
|---|---|---|
| `Contains` | Provider → Account → Region → Zone | hierarquia geográfica/contábil |
| `Contains` | Account → Environment | ambientes lógicos |
| `DeployedOn` | Compute (container) → Compute (cluster) | workload sobre host |
| `AttachedTo` | Persistence (block) → Compute | volume montado em VM |
| `AttachedTo` | Network (eni) → Compute | ENI atrelado a instância |
| `Routes` | Network (lb/gateway) → Compute/Persistence | tráfego direcionado |
| `Peers` | Network (vpc) ↔ Network (vpc) | peering simétrico |

Adjacency matrix completa em `internal/entity/edge/registry.go`.

## Edges cross-plane (vivem em `bridge/`)

| Edge | From → To | Documentado em |
|---|---|---|
| `DeployedOn` | Service (code) → Compute | módulo code + bridge |
| `BackedBy` | Table (code) → Persistence | módulo code + bridge |
| `Maintains` | Squad (org) → Compute | módulo org + bridge |
| `BudgetCovers` | Budget (org) → Account/Region | módulo org + bridge |

## Storage

- **Neo4j** (principal). Schema: label `:CeNode` com `kind` property +
  propriedades flattened em `nodeProps`; JSON em `type_specific_json`
  para roundtrip do tipo concreto. Edges: `:CE_EDGE` com `type`
  property.
- **Memory** (testes/oracle). `internal/repository/memory/memory.go`.

## Contratos públicos

Interfaces em `internal/repository/repository.go`:

- `NodeRepository`: Upsert, GetByURN, GetByExternalID, List, History, Delete
- `EdgeRepository`: Upsert, Between, Neighbors, Traverse, Delete

Filtros: `NodeFilter{Kind, Provider, AccountURN, RegionURN, AsOf, Limit, Offset}`,
`EdgeFilter{Types, AsOf, Limit, Offset}`.

Erros: `ErrNotFound`, `ErrInvalidArgument`, `ErrConflict` em `internal/repository/`.

## Bitemporal

Toda escrita versiona:
1. Fecha versão corrente (`valid_to = now`).
2. Cria nova versão com `version++` e `valid_from = now`.
Operações ocorrem em transação única.

Leitura:
- Default = current view (`valid_to IS NULL`).
- `AsOf(t)` = versão vigente em `t`.
- `History()` = todas as versões em ordem cronológica.

## Dependências

- `core/urn` — URN parser/builder
- `core/edge` — interface Edge + adjacency matrix
- `core/bitemporal` — Meta, ValidFrom/ValidTo
- `core/kind` — Kind enum

(Estes pacotes ainda vivem em `internal/entity/node` no estado atual;
migrar para `core/` está em `04-modular.md §9`.)

## Features relacionadas

- (a popular conforme features forem criadas)

## ADRs relacionadas

- ADR-001 (pendente): monolito modular inicial
- ADR-002 (pendente): bitemporal como invariante

## Open questions

1. Quando promover label dinâmico no Neo4j (atualmente todos os nós são
   `:CeNode` + `kind` property). Migração via APOC, mas adiada até
   provar valor.
2. `Environment` é entidade ou label? Tratado como entidade hoje, mas é
   discutível — pode virar Label em `Meta.Labels`.
3. Hierarquia de Network (VPC → Subnet → ENI) está implícita; vale
   formalizar `Contains` para esses pares?

## Estado de implementação

- ✅ Structs concretos com Resource interface
- ✅ Adjacency matrix com validação
- ✅ Repository in-memory e Neo4j
- ✅ Testes de roundtrip e bitemporal
- ⏳ Collectors (Fase 2)
- ⏳ Migração para `internal/core/` + `internal/modules/infra/`
  (Fase 4-5)
