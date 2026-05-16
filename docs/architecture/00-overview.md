# Overview da Arquitetura

> Síntese consolidada dos 7 documentos desta pasta. Lê-se em 5 minutos
> para ter o mapa mental; cada seção remete ao doc completo.

## O que é o CostEngine

Plataforma de **inteligência arquitetural** que mapeia infraestrutura,
código, organização e custos como um **grafo único bitemporal**, e
expõe esse grafo via APIs para 6 produtos web + 1 agente.

A unidade canônica é a **URN** (`urn:ce:<provider>:<account>:<kind>/<id>`).
Toda entidade tem versão temporal (`ValidFrom`/`ValidTo`) e
observabilidade (`ObservedAt`, `Confidence`, `Source`).

## Os planos do grafo

| Plano | O que representa | Onde detalhado |
|---|---|---|
| `infra` | Compute, Persistence, Network, Messaging, Provider, Account, Region | [01-modeling](./01-modeling.md) |
| `cost` | CUR rows, allocation, projections | [02-cost](./02-cost.md) |
| `code` | Service, Endpoint, Function, Database, Table, ExternalCall | [03-business](./03-business.md) §1 |
| `org` | Company, BusinessArea, Team, Squad, Person, Budget, OKR, Tool | [03-business](./03-business.md) §2.1 |
| `product` | Capability, Feature, Product, Offering, Contract | [03-business](./03-business.md) §2.2 |
| `revenue` | Receita por contrato/produto/período | [03-business](./03-business.md) §2.4 |
| `docs` | Document, DocumentVersion (knowledge base) | [05-platform](./05-platform.md) §7 |
| `bridge` | Edges cross-plane (DeployedOn, ImplementedBy, Owns, ...) | [04-modular](./04-modular.md) §3 |

## Como o código se organiza

Monólito modular por **plano**, não por entidade. Núcleo compartilhado
mínimo em `internal/core/`; cada plano em `internal/modules/<plano>/`;
edges cross-plane concentrados em `modules/bridge/`. Dependência
unidirecional: `cmd → app → modules → core/platform`.

Detalhes: [04-modular](./04-modular.md).

## Como o sistema se desdobra

Plataforma com 6 produtos consumindo APIs versionadas. Write-side
(coletores, ingestores, alocação, cron) separado do read-side
(produtos). BFF na entrada faz auth e roteamento.

Topologia atual: **monolito modular** (1 binário) servindo CLI + web
direto, com sementes estruturais para evoluir (versionamento de rota,
middleware de auth, outbox em Postgres, `pkg/client`).

Caminho: A → B (bordas extraídas) → T3 (write/read split) → T1 (services
por plano). Detalhes em [06-system](./06-system.md) e [07-tiers](./07-tiers.md).

## Stack alvo

| Camada | Tecnologia |
|---|---|
| Grafo principal | Neo4j |
| Fatos colunares (cost, revenue) | ClickHouse |
| Estado operacional + outbox + auth + projeções | Postgres |
| Object store (CUR, snapshots, docs) | S3 / MinIO |
| Busca textual (futura) | Postgres FTS → Meilisearch |
| Bus de eventos | Outbox (PG) → NATS JetStream (futuro) |
| Linguagem principal | Go 1.22+ |
| LLM do agente | Claude (via MCP) |

## Princípios não-negociáveis

1. **URN é canônica.** Cross-system references usam URN, nunca ID nativo.
2. **Bitemporal sempre.** Toda entidade carrega `ValidFrom/ValidTo`.
3. **Auth e tenant no middleware.** Nunca no handler ou produto.
4. **Idempotência de ingest.** Reprocessar 10× = mesmo grafo.
5. **Read models são descartáveis.** Reconstrução por replay.
6. **Edges cross-plane vivem em `bridge/`**, não nos planos donos.
7. **Product services não escrevem no grafo.** Só graph services
   escrevem; products consomem.

## Estado atual (2026-05-13)

- ✅ Fase 0: foundation (URN, Kind, Meta, ExternalID, testes)
- ✅ Fase 1: repository layer (interfaces + memory + Neo4j com
  bitemporal + adjacency matrix)
- ⏳ Fase 2: AWS Discoverer
- ⏳ Fase 3+: ver [roadmap](../backlog/roadmap.md)

## ADRs principais

(criadas conforme decisões emergem; lista em [decisions/](./decisions/))

- ADR-001: monolito modular inicial (pendente)
- ADR-002: bitemporal como invariante (pendente)
- ADR-003: BFF e CLI consomem mesma API (pendente)
