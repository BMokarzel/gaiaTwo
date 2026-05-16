---
id: ADR-001
title: Começar como monolito modular único, com sementes para extração futura
status: accepted
date: 2026-05-13
related_features: [F-001, F-002, F-003]
supersedes: []
superseded_by: []
---

# ADR-001 — Começar como monolito modular único

## Contexto

O CostEngine modela 8 planos (`infra`, `cost`, `code`, `org`, `product`,
`revenue`, `docs`, `bridge`) e atenderá 6 produtos web + 1 agente.
Documentos de arquitetura (`06-system.md`, `07-tiers.md`) descrevem
quatro topologias possíveis, de monolito modular único (A) até serviços
por produto com BFFs dedicados (D).

No momento desta decisão:
- 1 desenvolvedor ativo.
- Fase 0 e Fase 1 concluídas (foundation + repository layer).
- Nenhum consumidor externo produtivo ainda.
- Modelagem do grafo ainda em refinamento ativo (novos `Kind`s e edges
  surgem mês a mês).
- Tempo de feedback do ciclo de desenvolvimento é o gargalo dominante.

A escolha de topologia define onde o time pagará o "custo de coordenação"
nos próximos 12-18 meses.

## Decisão

Adotar **Topologia A** (monolito modular único — ver `06-system.md §3`):

- Um único binário `cmd/api` atendendo CLI, web (sem auth inicialmente)
  e futuros consumidores.
- CLI no mesmo repositório, binário separado para distribuição.
- Workers como goroutines in-process.
- Stream hub em memória.
- Bancos compartilhados (Neo4j single + Postgres + ClickHouse + S3/MinIO).
- Sem BFF, sem serviços por plano, sem fila externa.

**Três "sementes estruturais"** são adotadas desde o dia 1 para permitir
evolução barata para Topologia B/T3 sem reescrita:

1. **Versionamento de rota** desde o primeiro endpoint (`/v1/...`),
   mesmo sem consumidor externo.
2. **Middleware seam para auth e tenant**, mesmo com implementação
   `tenant = "default"` hard-coded inicialmente. Trocar a implementação
   do middleware não toca handlers.
3. **Outbox em Postgres** desde o primeiro `Upsert`. Eventos de mudança
   no grafo são gravados na mesma transação. Nenhum consumidor existe
   ainda, mas o contrato existe e tem histórico quando o primeiro
   chegar.

Acrescenta-se um quarto hábito:

4. **Pacote `pkg/client/`** encapsulando a chamada à API. CLI, testes
   de integração e, futuramente, agente e SDKs externos consomem o
   mesmo pacote.

## Alternativas consideradas

### B — Monolito + bordas extraídas (worker, stream hub, webhook separados)
Descartada para *agora*. Adiciona 3-4 processos para resolver problemas
que ainda não existem (workers competindo com API, WebSocket multi-instância).
Vira escolha natural quando ingest crescer ou WebSocket entrar em jogo.

### C — Serviços por plano (infra-svc, cost-svc, code-svc, ...)
Descartada. Pressupõe time grande (~10 devs) e ownership por plano.
Bloqueia refator cross-plano, que é exatamente o que mais acontece
durante refinamento de modelagem.

### T1 / T3 — write/read split com BFF + product services + graph services
Descartada para fase atual. Destino válido pós-MVP. Implementar agora
seria pagar custo de operação distribuída antes de ter consumidores
diferenciados.

### Microserviços por produto (D)
Descartada. Over-engineering para o estágio. Listada apenas para
completude.

## Consequências

### Positivas
- Refator cross-módulo é mecânico (mesma codebase, mesma transação,
  mesmo deploy).
- Iteração rápida: ciclo de feedback em segundos.
- Custo de infra baixíssimo (~$300-600/mês conforme `06-system.md`).
- Onboarding novo dev é "leia o `00-overview.md` e o `04-modular.md`".
- Toda a complexidade que existe é **modelagem de domínio**, não
  operação.

### Negativas / custo
- Workers competem por CPU com API. Job pesado degrada latência.
  Mitigação: jobs pesados rodam via CLI (`cmd/cli`) fora do processo
  da API.
- WebSocket exige sticky session ou *single instance*. Mitigação:
  começar single-instance; extrair stream hub quando necessário.
- Reiniciar deploy derruba tudo. Mitigação: aceitar janelas de
  manutenção até multi-instância valer a pena.
- Multi-tenant exige rigor desde o dia 1 (URN com tenant, middleware
  injetando `tenant_id`). Bug de leak aqui é catastrófico.

### Quando reabrir
Reabrir esta ADR quando *qualquer* uma das condições for verdadeira:
- Ingest de uma fonte (ex.: CUR mensal completo) degrada
  consistentemente p95 da API por mais de 30min.
- WebSocket entra em uso produtivo e há necessidade de mais de uma
  instância da API.
- Há 2+ times com necessidade de ciclos de release independentes.
- Há ≥2 tenants externos pagantes com SLAs distintos.

Caminho de migração recomendado: A → B → T3 → T1, conforme
`07-tiers.md §11`.
