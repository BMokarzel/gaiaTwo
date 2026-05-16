# Arquitetura em Camadas: BFF + Produtos + Grafo

> Refinamento da Topologia C/D de `arquitetura-sistema.md` com a divisão
> **write-side (Grafo) vs read-side (Produtos)** acoplada por um **BFF**
> de entrada (auth + roteamento). CLI ocupa papel dual: consome como
> humano e dispara escritas como operador.
>
> Este documento aprofunda essa topologia específica: variantes de
> granularidade, comunicação, ownership de dados.

---

## 1. Shape proposto

```
   Web                           CLI                       Agente
    │                             │                         │
    │   (HTTP)                    │  (HTTP, com creds         │ (HTTP / MCP)
    │                             │   privilegiadas)          │
    ▼                             ▼                          ▼
  ┌──────────────────────────────────────────────────────────────┐
  │                            BFF                                │
  │   auth · tenant resolution · rate-limit · roteamento · cache │
  └─────────────┬────────────────────────────────────┬───────────┘
                │                                     │
       (operações de leitura,                (operações de escrita,
        queries de UI)                        triggers, jobs, admin)
                │                                     │
                ▼                                     ▼
  ┌──────────────────────────────┐    ┌──────────────────────────┐
  │      PRODUCT SERVICES        │    │     GRAPH SERVICES       │
  │  (read-side, hot path)       │    │  (write-side, batch+sync)│
  │                              │    │                          │
  │  • arquitetura               │    │  • collectors (cloud)    │
  │  • teams                     │    │  • extractors (código)   │
  │  • observabilidade (proxy)   │    │  • ingestors (jira,      │
  │  • documentos                │    │     stripe, hris, cur)   │
  │  • dashboard                 │    │  • allocation engine     │
  │  • kanban                    │    │  • bridge resolver       │
  │                              │    │  • cron / scheduler      │
  └─────────────┬────────────────┘    └─────────────┬────────────┘
                │                                     │
       (read-only, joins,                  (read+write,
        agregações)                         transações)
                │                                     │
                └─────────────┬───────────────────────┘
                              ▼
  ┌──────────────────────────────────────────────────────────────┐
  │                        DATA TIER                              │
  │   Neo4j  ·  ClickHouse  ·  Postgres  ·  Meili  ·  S3/MinIO   │
  │                                                               │
  │   Bus interno (NATS/Redis): graph publica eventos de upsert; │
  │   product consome para invalidar cache / atualizar projeção. │
  └──────────────────────────────────────────────────────────────┘
                              ▲
                              │ (graph services emitem)
                              │
                       webhooks externos
                       (GitHub, Stripe, Jira)
```

**Três princípios estruturais:**

1. **Product services NÃO escrevem no grafo.** Só leem (+ escrevem em
   suas próprias projeções/caches). Toda mutação de domínio passa pelo
   Graph tier.
2. **Graph services NÃO atendem UI diretamente.** Eles operam sob
   demanda de jobs, webhooks ou da própria CLI. UIs não sabem que eles
   existem.
3. **BFF é trânsito, não regra.** Não tem lógica de domínio. Decide
   quem pode falar com o quê e em qual tenant.

---

## 2. Granularidade dos Product Services

### P1 — Um serviço por produto (6 serviços)

```
arquitetura-svc  teams-svc  obs-svc  docs-svc  dashboard-svc  kanban-svc
```

Cada produto tem cópia do esqueleto: HTTP handler, repo, cache,
projeção dedicada se necessária.

| Pró | Contra |
|---|---|
| Time de produto evolui sozinho | 6 binários para operar |
| Cache e escala por produto | Duplicação de plumbing |
| Falha isolada | Onboarding caro |
| Schema HTTP refletindo UI | Mudança transversal toca 6 lugares |

### P2 — Agrupados por storage (2-3 serviços)

```
graph-read-svc   →  arquitetura, teams, kanban (Neo4j-heavy)
analytics-svc    →  dashboard, observabilidade (ClickHouse + proxy)
docs-svc         →  documentos (Neo4j + Meili + S3)
```

Cada serviço entende **uma família** de acesso a dados.

| Pró | Contra |
|---|---|
| 3 binários, manutenção viável | Produtos diferentes compartilham processo |
| Aproveita pool de conexões Neo4j/CH | Failure não isolada por produto |
| Onboarding por backend | Mudança em arquitetura pode afetar teams |

### P3 — Um único product-svc

```
products-svc  →  todos os 6 produtos como sub-rotas
```

Casca única servindo `/v1/architecture/...`, `/v1/teams/...`, etc.

| Pró | Contra |
|---|---|
| 1 binário, deploy simples | Não isola carga |
| Mudança trivial | Reinício derruba 6 produtos |
| Caching e rate-limit centralizado | Time grande disputa o mesmo repo |

### P4 — Híbrido (recomendado para o meio do caminho)

```
products-core-svc  →  arquitetura, teams, kanban, docs (graph-leaning)
analytics-svc      →  dashboard (ClickHouse rollups)
obs-proxy-svc      →  observabilidade (proxy + enrichment)
```

Justificativa: dashboard tem padrão de query muito diferente; observability é
proxy reverso (não compartilha código com leitura de grafo); o resto pode
viver junto até a dor justificar split.

**Por que P4 e não P2:**
- `obs-proxy-svc` tem padrão operacional próprio (timeouts curtos, cache
  agressivo do upstream, possibilidade de stack diferente — Rust, Go com
  cliente especializado de Prom).
- Dashboard cumulativo é trabalho de batch refresh, que polui o ciclo de
  release do resto se misturar.

---

## 3. Granularidade dos Graph Services

### G1 — Um serviço por fonte

```
aws-collector  gcp-collector  git-extractor  jira-ingestor
stripe-ingestor  hris-ingestor  cur-ingestor  ...
```

| Pró | Contra |
|---|---|
| Failure isolada por integração | 8-12 binários |
| Stack por fonte (SDK específico) | Bridge logic vira difícil |
| Time por integração | Operação pesada |

### G2 — Agrupados por plano (recomendado)

```
infra-ingest-svc   →  AWS, GCP, Azure, K8s discoverers + bridge p/ código
code-ingest-svc    →  Git, AST, OpenAPI, schemas
business-ingest-svc → Jira, ProductBoard, HRIS, OKR
cost-ingest-svc    →  CUR parser + allocation engine
revenue-ingest-svc →  Stripe + ERP (ou subpacote de cost)
```

Cada um sabe **um plano** end-to-end: coleta, normalização, escrita.

| Pró | Contra |
|---|---|
| 4-5 binários, manutenção viável | Job lento de uma fonte degrada o plano |
| Mapeia 1:1 com módulos do código | Bridge logic precisa de coordenação |
| Cron por plano | Stack uniforme (sem flexibilidade) |

### G3 — Um único ingest-svc + workers

```
ingest-svc  →  coordena, expõe API de trigger
worker-pool →  pega job da fila, identifica fonte, despacha handler
```

| Pró | Contra |
|---|---|
| 2 binários | Worker é grande "se" de handlers |
| Pool elástico, fácil escalar | Não isola entre planos |
| Cron unificado | Deploy do worker afeta tudo |

### G4 — Híbrido: serviços longevos + workers efêmeros

```
infra-ingest-svc      (longevo, conexão persistente com AWS)
code-ingest-worker    (efêmero por job, lançado por evento)
business-ingest-cron  (cron periódico)
allocation-svc        (longevo, em background)
bridge-resolver-svc   (longevo, reativo a eventos)
```

| Pró | Contra |
|---|---|
| Cada peça com o ciclo certo | Topologia menos uniforme |
| Workers efêmeros = custo zero ocioso | Mais variedade operacional |

**Recomendação inicial: G2.** Mapeia direto nos módulos
(`modules/infra/`, `modules/code/`, `modules/cost/`, etc.) e gera
quantidade tratável de processos. Evoluir para G4 quando houver carga
clara para justificar workers efêmeros.

---

## 4. CLI — papel dual

A CLI tem dois rostos:

### 4.1 CLI como humano (read)
```
ce show service svc:checkout
ce list compute --region=us-east-1
ce cost summary --period=2026-Q1
```

Vai pela **mesma API que os produtos**, autenticada como o usuário.
Sem privilégio especial. Toda query passa pelo BFF.

### 4.2 CLI como operador (write/trigger)
```
ce extract aws --account=...        # dispara graph service
ce ingest cur --period=2026-04      # idem
ce migrate                          # admin nos bancos
ce link bridge --since=24h          # roda bridge resolver
ce backfill projection kanban        # rebuild de read model
```

Aqui a CLI:
- Autentica com **credencial de operador** (chave de service account,
  não usuário humano).
- BFF reconhece e roteia para o **endpoint admin** dos graph services
  (não para product services).
- Para `migrate` e operações de manutenção em DB, há flag `--direct`
  que conecta direto ao banco usando credencial separada (não passa
  pelo BFF).

**Regra:**
- Operações de domínio = via BFF (auditável, rate-limited, tenant
  enforced).
- Operações de plataforma (migrate, backfill, debug) = direct,
  documentado, só para SREs com credencial separada.

### 4.3 CLI como SDK escondido

A CLI implementa as mesmas chamadas que um SDK Go faria. Empacotar a
camada de "client de API" como pacote reutilizável (`pkg/client/`)
permite usar de:
- A própria CLI
- Agente (no repo separado)
- Outros sistemas internos

---

## 5. BFF — o que faz, o que NÃO faz

### Faz
- **Auth**: valida token, identifica usuário e tenant.
- **Tenant injection**: anexa `tenant_id` em todo request downstream.
- **Roteamento**: `/v1/architecture/*` → `products-core-svc`,
  `/v1/admin/extract/*` → `graph services` correspondente.
- **Rate-limit**: por tenant, por usuário, por endpoint.
- **Cache de borda**: respostas com TTL curto (`Cache-Control`), com
  invalidação por evento do bus quando aplicável.
- **Negociação de protocolo**: REST out, eventualmente GraphQL na
  borda para alguns produtos (Kanban tipicamente).
- **WebSocket upgrade**: terminação WebSocket no BFF, fan-out interno
  via bus.
- **Audit log**: registra quem chamou o quê, quando, com que payload.

### NÃO faz
- Lógica de domínio (composição de resposta de múltiplos services é
  ok; transformação semântica não).
- Decisão sobre o grafo (qualquer escrita é responsabilidade do graph
  tier).
- Caching que sobreviva a invalidação dirigida (cache mal-invalidado
  vira bug subterrâneo).
- Cross-tenant joins (jamais — paranoia de vazamento).

### Granularidade do BFF

| Opção | Quando |
|---|---|
| **BFF único** | recomendado no início |
| **BFF por canal** (web BFF + cli BFF + agent BFF) | quando comportamento de cada canal divergir muito |
| **BFF por produto** | só se Topologia D for adotada (ver `arquitetura-sistema.md`) |

---

## 6. Webhooks e gatilhos externos

```
[GitHub Action] [Stripe] [Jira] [Cloud cost report]
       │           │       │            │
       └───────────┴───────┼────────────┘
                           ▼
                ┌──────────────────┐
                │  webhook-svc     │   (small, validates HMAC,
                │                  │    drops onto queue)
                └────────┬─────────┘
                         │
                         ▼
                  bus (NATS / outbox)
                         │
                         ▼
              graph services consomem
              (code-ingest, cost-ingest, ...)
```

`webhook-svc` é deliberadamente pequeno: valida assinatura, identifica
o tipo de evento, enfileira. Não faz negócio. Resposta 202 imediata.

**Por que separar do BFF:** webhooks têm requisitos próprios (idempotência
por delivery-id, retry exponencial, assinatura por provider, sem auth
de usuário). Misturar com BFF de UI gera código defensivo bagunçado.

---

## 7. Comunicação entre tiers

### 7.1 BFF → Product (sync HTTP)
Latência <50ms para a maioria. Cache de borda quando aplicável.

### 7.2 BFF → Graph (sync para trigger, async para resultado)
```
POST /v1/admin/extract/aws { account, region }
  → BFF valida, encaminha para infra-ingest-svc
  → infra-ingest-svc enfileira job, retorna 202 + job_id
  → CLI/UI polla GET /v1/admin/jobs/:id  ou  recebe via WebSocket
```

### 7.3 Graph → Product (async, via bus)
Quando graph escreve, **emite evento**:
```
event: NodeUpserted
  urn: urn:ce:aws:1:compute/i-1
  version: 7
  ts: ...
```
Product services subscritos:
- Invalidam cache local
- Atualizam projeção (Kanban board, etc.)
- Publicam refresh via WebSocket para UIs conectadas

### 7.4 Product → Graph (não permitido)
Se um product service "precisa" escrever, é sinal de modelagem errada.
A escrita deve vir de fora (UI → BFF → graph) **ou** de um evento de
domínio que graph processa. Product permanece read-only.

### 7.5 Bus — outbox em Postgres ou NATS

Recomendação:
- **Outbox em Postgres** quando graph services e product services
  estiverem em pouca quantidade (≤8 processos). Outbox table escrita
  na mesma transação que o grafo; relay process publica no bus interno.
- **NATS JetStream** quando o número de subscritores ou volume crescer.
  Mesma API de eventos; o relay é trocado.

---

## 8. Bancos — onde cada um conecta

| Banco | Graph svcs | Product svcs | BFF | Webhook |
|---|---|---|---|---|
| Neo4j | RW | RO | — | — |
| ClickHouse | W (rollups, facts) | RO | — | — |
| Postgres (auth) | — | — | RW | — |
| Postgres (outbox) | RW | RO (consume) | — | RW (enqueue) |
| Postgres (projeções) | W (refresh) | RW (own state) | — | — |
| Meili / OpenSearch | W (refresh) | RO | — | — |
| S3 / MinIO | RW (CUR, snapshots, docs body) | RO (docs body fetch) | — | — |
| Cache (Redis) | — | RW | RW (edge) | — |

**Princípios:**
- Cada banco tem **um único service "dono"** que escreve.
- Outros leem só.
- Excepção: projeções podem ter dono single (graph que atualiza) +
  leitor (product). Read-Modify-Write **no product** só sobre seu
  estado próprio (ex.: cache, sessão), nunca sobre dado de domínio.

---

## 9. Três variantes combinadas

### Variante T1 — Enxuta (recomendada para começar este split)

```
1 × bff-svc
1 × products-core-svc   (arch, teams, kanban, docs)
1 × analytics-svc       (dashboard)
1 × obs-proxy-svc       (observabilidade)
1 × infra-ingest-svc
1 × code-ingest-svc
1 × business-ingest-svc (org + product + revenue)
1 × cost-ingest-svc
1 × webhook-svc
1 × allocation-svc
```

**10 processos.** Mapeamento claro 1:1 com módulos. Custo operacional
~$1500-2500/mês com clusters compartilhados.

### Variante T2 — Granular (quando T1 doer)

Quebrar `products-core-svc` em `arquitetura-svc` + `teams-svc` +
`kanban-svc` + `docs-svc` quando algum desses tiver carga ou time
próprio.

Quebrar `business-ingest-svc` em `org-ingest-svc` + `product-ingest-svc`
+ `revenue-ingest-svc` se os adapters virarem complexos.

**14-16 processos.** Time precisa ter SRE dedicado.

### Variante T3 — Mínima (transitória, saindo de monolito)

```
1 × bff-svc
1 × products-svc       (todos os 6)
1 × graph-svc          (todos os ingest + alocação + bridge)
1 × webhook-svc
```

**4 processos.** Útil como **passo intermediário** entre Topologia B do
doc anterior (monolito + bordas) e T1 acima. Já tem a separação
write/read, sem o custo de fragmentar mais.

---

## 10. Trade-offs vs Topologia C original

| Critério | C (serviços por plano) | T1 (BFF + products + graph) |
|---|---|---|
| Foco da separação | por domínio | por leitura/escrita |
| Reuso de modelos | duplicado (DTO por serviço) | menos duplicação (graph→bus→product) |
| Cache de leitura | difícil cross-domain | natural no product tier |
| Mudança em ingest | toca apenas graph tier | idem |
| Mudança em UI | toca product + BFF | idem |
| Edges cross-plane | dolorosos | concentrados no graph tier |
| Onboarding | "qual serviço cuida disso?" | "leitura ou escrita?" + plano |
| Cron / scheduler | espalhado | concentrado em graph |

**A diferença real:** C divide *por o quê* (infra/cost/code/...), T1
divide *por quando* (lê em tempo de UI vs escreve em ingest/cron).
Para um sistema cujo ponto-chave é "alimentar 6 produtos rápidos com
dados que vêm de fontes lentas", T1 é mais honesto.

---

## 11. Caminho de evolução

```
[hoje: Topologia A planejada]
        │
        ▼
[Topologia B: monolito + bordas (worker extraído)]
        │
        ▼
[T3: 4 processos, write/read separados]
        │  ← aqui já se ganha cache decente, ingest não atrapalha UI
        ▼
[T1: 10 processos, products+graph granulares]
        │  ← aqui já se ganha time-per-plano + scaling per-product
        ▼
[T2: 14-16 processos, granular]
```

Cada passo é deployável e mantém o anterior funcionando.

**Sinais para subir um nível:**
- A→B: ingest começou a degradar latência da API.
- B→T3: chegou um 2º produto que precisa de cache de leitura forte.
- T3→T1: time cresceu e há ownership por plano emergindo.
- T1→T2: um serviço específico virou gargalo de release ou de carga.

**Sinais para NÃO subir:**
- "Vai escalar melhor" sem medição = ignorar.
- "Best practice em empresa X" sem mapear contexto = ignorar.
- "Vai facilitar contratar gente" = ruim, complexidade não atrai talento.

---

## 12. Riscos específicos desta topologia

| Risco | Onde aparece | Mitigação |
|---|---|---|
| Product "precisa escrever" e alguém abre exceção | T1, T2 | revisão de PR sêmpre rejeita; documentar como hard rule |
| Bus de eventos pula tópicos no boot | T1+ | replay de outbox; events idempotentes |
| BFF vira monstro com regra de negócio | qualquer | code review impede; mover qualquer transformação para svc downstream |
| Cron duplicado em múltiplas réplicas de graph svc | T1+ | leader election (file lock em Postgres ou DistributedLock) |
| CLI direct-DB vira hábito | qualquer | só com flag explícita; logar e alertar quando usado |
| Webhook drop silencioso | qualquer | idempotency key + DLQ + alerta de DLQ não vazia |
| Cache de borda fica stale | T1+ | TTL curto + invalidação por evento; nunca cache de mais de 5min sem invalidation |

---

## 13. Resumo da escolha

Para o estado atual (Fase 0-1 concluída), a sequência:

1. **Continuar em Topologia A** (monolito modular) até existirem 2
   consumidores produtivos.
2. **Pular direto para T3** quando começar a integrar UI real e CLI
   estiver em uso operacional. T3 já dá o split write/read e
   estabelece o contrato BFF↔products↔graph sem fragmentar demais.
3. **Promover para T1** conforme planos ficam ativos (cost, code, org
   sendo todos ingeridos) e produtos ganham donos.
4. **T2 só sob dor mensurada.**

O ganho concreto desta arquitetura sobre a Topologia C "pura" é
estrutural: a fronteira write/read alinha com o ciclo de vida natural
dos dados (fontes lentas → processamento → projeções → UI rápida).
Times de produto raramente precisam pensar em ingest; times de
plataforma raramente precisam pensar em UI. Cada grupo trabalha onde
manda.
