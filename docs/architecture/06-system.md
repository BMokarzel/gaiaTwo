# Arquitetura de Sistema: Topologias Possíveis

> Desenho da **topologia de execução** do CostEngine: quantos processos,
> quais bancos, onde mora cada coisa, como tudo conversa. Documento
> orientado a *trade-off*, não a prescrição.
>
> Complementa `arquitetura-modular.md` (organização do código-fonte) e
> `arquitetura-plataforma.md` (papel de plataforma). Aqui o foco é
> deployment, dados e fronteiras de processo.

---

## 1. O que precisa caber

Recapitulando as peças identificadas nos docs anteriores:

**Compute (o que roda):**
- Extração e ingestão (provedores cloud, CUR, código, HRIS, Jira, Stripe)
- API de leitura/escrita para 6 produtos web
- Stream hub (WebSocket para Kanban live, Dashboard refresh)
- Workers de jobs assíncronos (rebuild de projeção, alocação de custo)
- Webhooks (GitHub Action, Stripe, Jira, billing)
- Agente conversacional + MCP servers
- Proxy de observabilidade (passa-através enriquecido)
- CLI distribuível para operadores

**Dados (onde mora o quê):**
- Grafo (infra, code, org, product, docs, bridge)
- Fatos colunares (cost, revenue, telemetry agregada)
- Object store (CUR parquet, conteúdo de documentos, snapshots)
- Projeções operacionais (Kanban board, busca de documentos)
- Estado de jobs / agendamento
- Identidade, auth, secrets
- Memória vetorial do agente

**Tráfego (o que cruza fronteira):**
- Página web → API (REST + WebSocket)
- Página web → produtos individuais (se houver BFFs)
- Agente → produtos (via MCP)
- CLI → API (operações pontuais) **ou** CLI → DB direto (jobs)
- Webhooks externos → API
- API → providers cloud (descoberta, custo)

---

## 2. Variáveis de decisão

As topologias variam ao longo de **5 eixos**:

| Eixo | Extremos |
|---|---|
| **Granularidade de processo** | 1 binário ↔ N microserviços |
| **Granularidade de repo** | monorepo ↔ multi-repo |
| **Coabitação de DB** | um cluster compartilhado ↔ DB por serviço |
| **Acoplamento via fila** | tudo síncrono ↔ event-driven com bus |
| **Localização do agente** | dentro da plataforma ↔ totalmente externo |

Cada topologia abaixo é uma combinação coerente desses eixos.

---

## 3. Topologia A — Monólito Modular Único

```
┌──────────────────────────────────────────────┐
│              cmd/api  (1 binário)            │
│  ┌────────────────────────────────────────┐  │
│  │ HTTP/REST + WebSocket                  │  │
│  │ Workers (goroutines)                   │  │
│  │ Cron (gocron in-process)               │  │
│  │ Webhook handlers                       │  │
│  │ Stream hub (in-memory pub/sub)         │  │
│  └────────────────────────────────────────┘  │
└──────────────────────────────────────────────┘
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
     Neo4j        ClickHouse      Object store
   (single)       (single)         (S3/MinIO)
        │              │
        ▼              ▼
     Postgres       (CUR parquet)
   (auth, jobs,
    projeções,
    vetores via
    pgvector)

cmd/cli ─── (mesmo repo, binário separado) ───► API ou DB
```

**Quando vale:**
- Fase MVP, 1-3 desenvolvedores.
- 1 tenant ou poucos.
- Equipe sem cultura de operação distribuída.

**Pró:**
- Deploy trivial (1 binário + 3-4 bancos).
- Refatorar é seguro: tudo está num lugar.
- Latência mínima: tudo in-process.
- Custo de infra baixíssimo.

**Contra:**
- Workers competem com requests por CPU. Job pesado degrada API.
- Reiniciar para deploy derruba tudo (sem zero-downtime sem dança).
- WebSocket exige sticky session ou *single instance* (não escala
  horizontal sem extrair stream hub).
- Toda mudança em qualquer módulo recompila tudo.
- Multi-tenant exige rigor; data leak no schema é catastrófico.

**Custo operacional aproximado:**
- 1 processo de API (2-4 vCPU, 4-8 GB RAM)
- Neo4j single-node (4 vCPU, 16 GB RAM)
- ClickHouse single-node (4 vCPU, 16 GB RAM)
- Postgres pequeno (2 vCPU, 4 GB RAM)
- Object store gerenciado
- ~$300-600/mês em cloud para começar

---

## 4. Topologia B — Monólito + Bordas Extraídas

```
                 cmd/api (núcleo)
                       │
   ┌───────────────────┼─────────────────────┐
   │                   │                     │
   ▼                   ▼                     ▼
 Bancos        cmd/worker             cmd/streamhub
 (idem A)      (jobs pesados,         (WebSocket fan-out,
                ingest, ETL)           Redis-backed pub/sub)
                       │
                       ▼
                  fila (NATS / Redis Streams)
                       ▲
                       │
   ┌───────────────────┴────────────────────┐
   │                                         │
 cmd/api emite eventos              cmd/webhook
 (na borda da escrita)              (handlers de Stripe,
                                     GitHub, Jira)


repos externos:
- repo: agent           (cliente MCP, fora deste repo)
- repo: observability-proxy (Rust ou Go fino, opcionalmente fora)
```

**Diferenças vs A:**
- Workers em **processo próprio** (mesmo repo, binário diferente).
  API não compete com eles por recursos.
- Stream hub extraído: usa Redis Streams ou NATS, escala horizontal,
  resolve WebSocket multi-instância.
- Webhooks em handler dedicado (tolerância a falha; backoff próprio
  não afeta API principal).
- Agente em repo separado consumindo só APIs públicas.

**Quando vale:**
- ~5+ tenants ou tráfego subindo.
- Equipe de 3-8 devs.
- Ingest começa a competir com leitura.
- Precisa de zero-downtime na API.

**Pró:**
- API enxuta, com perfil de latência previsível.
- Workers escalam independente.
- WebSocket funciona com múltiplas instâncias de API.
- Mantém vantagem do monorepo: tipos compartilhados, refactor seguro.
- Caminho para Topologia C sem reescrever — só extrair mais processos.

**Contra:**
- 3-4 processos para operar (API, worker, stream hub, webhook).
- Bus (NATS/Redis) é nova peça crítica.
- Debug fica mais difícil: rastreio entre processos exige tracing.
- Cron precisa de coordenação (leader election) para não duplicar jobs.

**Custo operacional:**
- 2-3× Topologia A (mais processos, mas mesmos bancos)
- ~$600-1200/mês

---

## 5. Topologia C — Serviços por Plano

```
                       ┌─────────────────┐
                       │  cmd/gateway    │  ◄── produtos web entram aqui
                       │  (auth, route)  │
                       └────────┬────────┘
                                │
   ┌──────────┬───────────┬─────┴─────┬───────────┬───────────┐
   ▼          ▼           ▼           ▼           ▼           ▼
 infra-svc cost-svc   code-svc   org+prod-svc docs-svc  projection-svc
   │          │           │           │           │           │
   ▼          ▼           ▼           ▼           ▼           ▼
 Neo4j     ClickHouse  Neo4j      Neo4j      Neo4j       Postgres
 (infra)   (facts)     (code)     (org+prod) (docs)      + Meili
                                                          + ClickHouse
                                                            cubes

         bus (NATS) ────────────────────────────────────────────►
         (eventos de upsert publicados por todos)
```

**Estrutura:**
- Cada plano vira processo + DB próprio.
- Gateway na entrada faz auth e roteia.
- Comunicação cross-plane via API interna (bridge-svc opcional) ou via
  bus (eventos).
- Projeções viram serviço dedicado (alimenta Kanban, Documents, Dashboard).

**Quando vale:**
- ~10+ devs em times distintos.
- Múltiplos tenants com SLAs diferentes.
- Um plano tem padrão de carga muito distinto dos outros.
- Equipes querem ciclos de release independentes.

**Pró:**
- Failure isolation: derrubar `cost-svc` não tira `infra-svc` do ar.
- Stacks por serviço (cost pode ser Python por causa de ML; resto Go).
- Times podem deployar sem coordenação.
- Storage por serviço — fácil escalar onde dói.

**Contra:**
- **Edges cross-plane são dor.** `DeployedOn` (code → infra) vive onde?
  Em `bridge-svc` separado? Em qualquer um dos dois? Consistência
  entre processos exige saga ou eventual.
- Transações distribuídas: upsert de Service+DeployedOn não é atômico.
- 6-8 processos em produção. Observabilidade vira investimento sério.
- Refactor cross-service é trabalho coordenado.
- Schema do grafo *fragmentado* — query "tudo conectado a esta URN"
  precisa de federação.

**Custo operacional:**
- 6-8 processos rodando.
- 3-4 clusters de Neo4j (ou 1 cluster com bases lógicas).
- ClickHouse cluster.
- Bus crítico (3 nós).
- ~$2000-4000/mês.

---

## 6. Topologia D — Serviços por Produto (BFF amplo)

```
   página web
   ├──► arquitetura-bff ──► core-graph-svc (Neo4j infra+code+bridge)
   ├──► teams-bff       ──► core-org-svc   (Neo4j org+bridge)
   ├──► obs-bff         ──► proxy + core-graph-svc (enrichment)
   ├──► docs-bff        ──► docs-svc (Neo4j docs + Meili + S3)
   ├──► dashboard-bff   ──► metrics-svc (ClickHouse + rollups)
   └──► kanban-bff      ──► kanban-projection-svc (Postgres) + product-svc

   agent ──► MCP servers ──► mesma topologia acima
```

**Estrutura:**
- 1 BFF por produto web.
- BFF é casca fina sobre serviços de domínio (que podem coincidir com
  os de Topologia C).
- BFF é o lugar onde produto-específico vive (caching, shape DTO,
  rate-limit por produto).

**Quando vale:**
- Cada produto tem time próprio.
- Produtos têm requisitos de UX muito divergentes.
- Já se sente dor de over/under-fetching com API genérica.

**Pró:**
- Produto evolui sem mexer no core.
- Time do produto domina o BFF — autonomia real.
- Cache por produto.
- Schema do BFF reflete a UI (GraphQL aqui faz mais sentido que no core).

**Contra:**
- 6 BFFs **+** os serviços de domínio = 10-14 processos.
- Lógica espalhada: regra de negócio escapa para BFF se não houver
  disciplina.
- Onboarding dev fica caro (precisa entender o BFF do seu produto + o
  core).
- Coordenação de mudança de contrato no core afeta 6 BFFs.

**Custo operacional:**
- ~$3000-6000/mês.

---

## 7. Comparação lado a lado

| Critério | A: Monolito | B: Monolito + bordas | C: Serviços/plano | D: BFF/produto |
|---|---|---|---|---|
| Processos em prod | 1 | 3-4 | 6-8 | 10-14 |
| Time mínimo viável | 1-3 | 3-8 | 8-15 | 15+ |
| Tempo até "hello world" | dias | dias | semanas | semanas |
| Refactor cross-domain | trivial | trivial | médio | difícil |
| Falha isolada | não | parcial | sim | sim |
| Deploy independente | não | parcial | sim | sim |
| Stack heterogênea | não | parcial | sim | sim |
| Custo cloud baseline | baixo | médio | alto | mais alto |
| Risco de over-engineering | nenhum | baixo | alto | crítico |
| Caminho de migração | A→B trivial | B→C planejado | C→D incremental | terminal |

---

## 8. Bancos — opções de organização

Independente da topologia de processos, os bancos têm decisões próprias.

### 8.1 Neo4j

| Opção | Como | Pró | Contra |
|---|---|---|---|
| **Single-node** | 1 instância | barato, simples | SPOF, downtime no upgrade |
| **Causal cluster** | 3+ nós | HA, leitura escala | mais caro, latência de escrita |
| **Por plano** | 1 cluster por plano | isolamento total | queries cross-plane viram impossíveis |
| **Single + database lógico** | Neo4j Enterprise tem multi-DB | isolamento lógico | exige Enterprise |

**Recomendação inicial:** single-node em dev/staging, causal cluster
3-node em prod. **Não** fragmentar por plano antes de virar problema —
o valor do grafo é a conexão.

### 8.2 ClickHouse

| Opção | Quando |
|---|---|
| **Single-shard** | até ~bilhão de linhas/mês de fatos |
| **Sharded** | acima disso |
| **Por workload** (cost vs telemetry) | se padrão de query brigar |

Tabelas com partição mensal + `ORDER BY (period, urn)`. Materialized
views para os dashboards mais quentes.

### 8.3 Postgres

Postgres é a "navalha suíça" — usar para tudo que não justifica banco
especializado:
- Auth, sessões, API keys
- Estado de jobs (queue table pattern, ou outbox)
- Projeção Kanban
- `pgvector` para memória do agente (até ~100M vetores; acima disso
  considerar Qdrant/Weaviate)
- Metadados de documentos

**Cuidado:** um Postgres compartilhado entre 6 funcionalidades é tentação.
Manter schemas separados (`auth`, `jobs`, `kanban`, ...) e tratar como
se fossem bancos lógicos. Em algum momento valerá dividir.

### 8.4 Object store

- **S3 (cloud) ou MinIO (self-hosted)**.
- Buckets por categoria: `cur-raw`, `cur-silver`, `documents`, `snapshots`.
- Lifecycle policy: bronze 90 dias, silver 1 ano, gold permanente.

### 8.5 Bus / fila

| Opção | Características |
|---|---|
| **Redis Streams** | leve, ok até centenas msg/s, persistência fraca |
| **NATS JetStream** | bom equilíbrio, mais durável, escala bem |
| **Kafka** | overkill no MVP; necessário se atingir ~10k msg/s sustentado |
| **Postgres outbox** | sem nova peça; bom até centenas msg/s |

**Recomendação:** outbox em Postgres no MVP (zero nova peça); NATS
JetStream quando crescer.

### 8.6 Search index

| Opção | Bom para |
|---|---|
| **Postgres FTS + pg_trgm** | até ~milhões de docs, busca simples |
| **Meilisearch** | typo-tolerant out of the box, configuração mínima |
| **OpenSearch / Elastic** | analytics + busca, mais peso operacional |

**Recomendação:** Postgres FTS no MVP, Meili quando precisar fuzzy.

---

## 9. Estratégia de repositórios

| Estratégia | Forma | Trade-off |
|---|---|---|
| **Monorepo total** | tudo num repo, inclusive frontend dos produtos e agente | refactor cross-tudo é fácil; CI fica complexa; perde-se "ownership por repo" |
| **Monorepo backend, repos separados para frontends** | costEngine repo + 6 repos web + 1 repo agent | equilíbrio que a maioria escolhe |
| **Repo por serviço** | dezenas de repos | autonomia máxima, custo de coordenação alto |
| **Híbrido em camadas** | `platform-monorepo` (core+modules+cmd) + `products-monorepo` (6 produtos web) + `agent-repo` | melhor para times pequeno-médio |

**Recomendação:** monorepo backend (este repo cresce com tudo do servidor),
repos separados para cada produto web (ciclos próprios, stacks
possivelmente diferentes), repo separado para agente (consumidor da API
pública).

---

## 10. CLI — distribuição

A CLI é parte do mesmo repo, mas tem ciclo de distribuição próprio:

| Modo | Quando |
|---|---|
| `go install costEngine/cmd/cli` | devs internos |
| Binário em release GitHub | operadores em outras empresas |
| Container `ghcr.io/.../ce:tag` | execução em CI/Action |
| Homebrew tap / Scoop manifest | usuários finais |
| GitHub Action published | uso direto em workflows |

A CLI deve:
- **Não exigir conexão direta com Neo4j/ClickHouse** em produção (cliente
  do usuário não tem essas credenciais). Toda operação passa pela API.
- **Em dev/admin**, pode acessar bancos direto via flag (`--direct-db`)
  para operações de migração e recuperação.

---

## 11. Recomendação por fase

```
[FASE 0–1: Foundation atual]
      Topologia A  +  Postgres outbox  +  Neo4j single
      Sem agente, sem produtos ainda.

           ▼  (quando aparecer 2º consumidor sério)

[FASE 2–4: Plataforma básica]
      Topologia A → B  (extrair worker)
      Adicionar bus quando outbox virar gargalo.
      Stream hub extraído quando WebSocket entrar em jogo.
      Agente em repo separado consumindo APIs.

           ▼  (quando time crescer ou um plano destacar carga)

[FASE 5+: Crescimento]
      Topologia B → C  (extrair os planos mais "ativos" primeiro)
      Mantém o resto em monolito.
      C é destino, não obrigação.

           ▼  (apenas se houver dor real)

[FASE 6+: Especialização por produto]
      Topologia D introduzida produto a produto, conforme dor.
      Pode coexistir com C — alguns produtos têm BFF, outros não.
```

**A regra:** mover de A→B→C→D é fácil **na direção indicada**. Pular
estágios cria sistemas que ninguém entende ou opera bem.

---

## 12. Riscos transversais

| Risco | Onde aparece | Mitigação |
|---|---|---|
| Data leak entre tenants | qualquer topologia | URN com tenant + query rewriter + testes de fuzz |
| Pequeno DB-do-cara-X-virou-crítico | Postgres compartilhado | namespacing por schema desde o dia 1 |
| Stream hub é SPOF | A, B | redundância no bus (NATS cluster) ou aceitar polling como fallback |
| Drift de schema entre serviços | C, D | contrato versionado, schema registry |
| Saga sem disciplina | C, D | menos eventos cross-serviço, mais reconciliação batch |
| CLI vira "API alternativa" e bypassa regras | qualquer | CLI consome API igual produtos; só modo admin acessa DB |
| Agente consome API "mágica" privada | qualquer | tools = endpoints públicos, sem atalho |
| ClickHouse vira lago bagunçado | qualquer | tabelas particionadas, lifecycle, ownership por módulo |
| Neo4j cresce demais | C tarde | sharding lógico por tenant ou plano somente quando dói |
| Custo cloud silencioso | B em diante | dashboard interno consumindo o próprio CostEngine 😉 |

---

## 13. Decisões que **NÃO** dependem da topologia

Independente de A/B/C/D, alguns padrões valem desde o início:

1. **URN é canônica.** Toda referência cruza por URN, nunca por ID
   nativo. Migração entre topologias é trocar processos, não dados.
2. **Bitemporal sempre.** Não há "topologia que dispensa versionamento".
3. **Auth e ACL no gateway** (ou middleware do API). Nunca no
   client/produto.
4. **Idempotência de ingest.** Reprocessar a mesma fonte 10 vezes dá o
   mesmo grafo. Sem isso, qualquer topologia distribuída quebra.
5. **Observabilidade tripla:** logs estruturados + métricas + traces.
   Em A é simples; em C é vida ou morte.
6. **Migrations versionadas.** Cada serviço/módulo tem suas migrations
   (existe `n4j/schema.go` para Neo4j; replicar para ClickHouse e
   Postgres).
7. **Read models são descartáveis.** Em qualquer topologia, projeção
   deve poder ser rebuildada do grafo + facts. Se não pode, virou SoT
   acidental.

---

## 14. Pergunta a ser respondida antes de escolher

Para decidir entre A, B, C, D — responder antes:

1. **Quantas pessoas vão trabalhar no backend nos próximos 12 meses?**
   - 1-3 → A. 3-8 → A ou B. 8+ → B começa a apertar.
2. **Há previsão de múltiplos tenants externos pagantes?**
   - Sim → multi-tenant disciplinado desde o dia 1 (vale em qualquer).
3. **Algum produto tem padrão de tráfego radicalmente diferente?**
   - Sim → forte sinal para extrair aquele caminho (B parcial → C).
4. **O agente é diferenciador estratégico ou commodity?**
   - Diferenciador → repo próprio, evolução rápida, time dedicado.
   - Commodity → pode até ficar embarcado no API por mais tempo.
5. **Quem é o operador?** SRE dedicado, dev rodando ops, ou empresa
   cliente self-hosting?
   - Self-hosting de outras empresas → **prefira A** o quanto puder.
     Cada processo extra é fricção para o cliente.

---

## 15. Recomendação opinativa

Para o estado atual (Fase 0-1 concluída, planos `cost`/`code`/`org`/
`product`/`docs` ainda pendentes):

**Topologia A** com **3 ressalvas estruturais** que pagam dividendos
quando migrar para B:

1. **Cron e workers já isolados em pacote** (`internal/jobs/`), mesmo
   rodando como goroutine. Extrair para processo separado vira mudança
   de `main`, não de código.
2. **Outbox em Postgres** desde o dia 1. Eventos de upsert são gravados
   na mesma transação que o grafo, e um worker dispatch publica. Quando
   houver consumidor externo (stream hub ou serviço extraído), o
   contrato já existe.
3. **API pública versionada** (`/v1/...`) desde o primeiro endpoint.
   Mesmo que o único consumidor seja a CLI no início, o contrato é
   formal.

Com isso, A→B é refator mecânico de algumas horas. A→C é projeto, mas
nunca volta a zero.
