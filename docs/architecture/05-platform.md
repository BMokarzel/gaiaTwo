# CostEngine como Plataforma: 6 Produtos + Agente

> Reposicionamento: o CostEngine deixa de ser "uma ferramenta de custo" e
> passa a ser a **backbone de dados** que viabiliza 6 produtos verticais
> consumidos por uma página web, somados a um agente conversacional que
> opera transversalmente.
>
> Complementa `arquitetura-modular.md` (organização do código),
> `arquitetura-negocio.md` (planos org/product/revenue) e
> `arquitetura-custos.md` (plano cost).

---

## 1. Mudança de papel

| Antes | Depois |
|---|---|
| Sistema único com CLI + API leve | Plataforma com **N consumidores externos** |
| Output principal: relatórios de custo | Output principal: **APIs e read models** |
| Usuário final: FinOps/Plataforma | Usuário final: 6 produtos + 1 agente |
| Modo: batch + queries pontuais | Modo: misto (batch, transacional, streaming) |

O grafo continua sendo a fonte de verdade. O que muda é que ele precisa
**servir múltiplos formatos** para múltiplos consumidores com SLAs
distintos. Isso não é evolução cosmética — afeta API design,
performance, isolamento de mudanças e estratégia de cache.

---

## 2. Os 6 produtos — perfil de consumo

| Produto | Leitura dominante | Escrita? | Tempo real? | Fonte primária no CostEngine |
|---|---|---|---|---|
| **Arquitetura** | grafo (traversal, simulação) | não | não | `infra` + `code` + `bridge` |
| **Teams** | hierárquica (org) | sim (registrar/mover) | não | `org` |
| **Observabilidade** | séries temporais + traces | não | **sim** | `code/telemetry` (pointers) + tools externos |
| **Documentos** | full-text + grafo | sim | não | `docs` (NOVO) |
| **Dashboard** | agregações | não | semi (refresh 1-5min) | `cost` + `revenue` + rollups |
| **Kanban** | feature/story por sprint | sim | semi (WebSocket) | `product` + `bridge` |
| **Agente** | tudo, sob demanda | sim (via tools) | conversacional | todos |

### O que cada produto realmente precisa

**Arquitetura** — vista de grafo navegável: clicar num Service e ver
dependências, Compute onde roda, custo associado, pessoas responsáveis,
simulação what-if. Caminho de query: traversal Cypher, latência <1s,
pode cachear agressivo.

**Teams** — vista hierárquica: árvore Company→BusinessArea→Team→Squad→
Person, com filtro por capability/serviço. Pouco volume, alto valor
informacional. Caminho: query direta no `org/repo`, cache curto.

**Observabilidade** — passa-através enriquecido. O CostEngine **não
substitui** Prometheus/Datadog/Tempo — ele enriquece os dados com
contexto do grafo ("este alerta no service-X é dono do squad-Y, parte da
capability-Z, último deploy foi PR #1234"). Caminho: proxy reverso +
join com grafo.

**Documentos** — knowledge base ligada ao grafo. Cada documento vive
preso a um ou mais nós (Service, Capability, Compute, Person). Caminho:
full-text search + grafo de referências. Storage: índice (Meilisearch/
OpenSearch/Postgres FTS) + conteúdo em object store.

**Dashboard** — números agregados, frescos o suficiente. Caminho:
ClickHouse com rollups pré-computados, refresh por job; UI faz polling.
Não precisa ser instantâneo, precisa ser **rápido e consistente**.

**Kanban** — espelho de status de Features/UserStories. Caminho:
projeção relacional (Postgres ou ClickHouse) com índice por sprint,
squad, status. Updates de drag-and-drop via WebSocket. Conexão de saída
para Jira/Linear se eles forem source of truth.

**Agente** — orquestra tudo. Não tem dados próprios; usa **tools** que
batem nas mesmas APIs dos produtos.

---

## 3. Plataforma, não tool — implicações arquiteturais

Ser plataforma significa:

1. **Contratos públicos versionados.** APIs externas ganham versão
   (`/v1/...`), schema documentado (OpenAPI/GraphQL SDL/gRPC proto), e
   política de deprecation. Mudança de schema vira evento, não detalhe
   de implementação.
2. **Isolamento de read paths.** Produtos não tocam Neo4j/ClickHouse
   diretamente. Vão pela API (ou por projeção, ver §4). Isso permite
   trocar o storage sem coordenar com 6 consumidores.
3. **Observabilidade de quem chama o quê.** Cada produto consome via
   chave/identidade própria. Métricas por consumidor revelam quem usa o
   quê, quem é candidato a cache, quem precisa de novo endpoint.
4. **Multi-tenant pronto desde cedo.** Se a página web atende uma
   empresa hoje, atenderá várias amanhã. URN já tem `account/<tenant>`
   embutido — basta enforcement na API.

---

## 4. Arquitetura de API — opções e recomendação

### Opção A — API única, produtos compõem
- Um `cmd/api` expõe endpoints por **domínio** (não por produto).
- Cada produto chama os endpoints que precisar.
- Pró: simples, um auth, um versionamento.
- Contra: produtos com necessidades específicas sofrem N+1 ou
  over-fetching.

### Opção B — BFF por produto
- 6 binários, cada um casca fina sobre os módulos internos.
- Pró: produto tem exatamente o que precisa.
- Contra: 6× operação. Cara para o estágio atual.

### Opção C — GraphQL gateway único
- Um endpoint, produtos pedem o shape que querem.
- Pró: flexível, evita over-fetching.
- Contra: caching difícil, N+1 problema clássico, exige disciplina
  forte de resolver design.

### Opção D — **API core + projeções dedicadas** (recomendada)

**Estrutura:**

```
                    cmd/api (REST + WebSocket)
                          │
                ┌─────────┼──────────┐
                ▼         ▼          ▼
            modules    projections   stream
            /repo      /<product>    /hub
                │         │          │
                ▼         ▼          ▼
           Neo4j +    Postgres /   Redis pub/sub
           ClickHouse Meili        (refresh events)
```

- **Core**: endpoints REST por domínio + WebSocket para streams.
  Atende Arquitetura, Teams (lê direto do grafo).
- **Projeções (CQRS leve)**: produtos com perfil de query divergente
  (Kanban, Documentos, Dashboard) leem de uma **projeção** otimizada,
  alimentada por jobs ou listeners do grafo. A projeção é descartável e
  reconstruível.
- **Stream hub**: WebSocket único para eventos de mudança (refresh do
  Dashboard, drag-and-drop do Kanban, alerta de observabilidade).
  Produto se inscreve nos tópicos relevantes.

**Por que essa funciona:**
- Não força BFF por produto antes da hora.
- Não força GraphQL (que tem ônus operacional).
- Permite que cada projeção evolua independentemente.
- Mantém o grafo como fonte de verdade, sem que consultas pesadas de
  Dashboard derrubem queries de Arquitetura.

### Como produto escolhe entre core e projeção

| Tipo de query | Para onde vai |
|---|---|
| Traversal de grafo, ad-hoc | API core → Neo4j |
| Agregação sobre milhões de linhas | API core → ClickHouse |
| Listagem com filtro/ordenação custom (Kanban board) | Projeção dedicada (Postgres) |
| Busca textual | Projeção (Meili/OpenSearch) |
| "Quais features mudaram no último minuto?" | Stream hub |

---

## 5. Read models / CQRS leve

Projeções vivem em `modules/<plano>/projection/` ou em um módulo
`projection/` dedicado, dependendo do escopo:

```
internal/modules/
  projection/
    kanban/          # Postgres: feature_card(id, status, sprint, squad, ...)
    documents/       # Meili: doc index + linked_urns
    dashboard/       # ClickHouse cubes refresh
  ...
```

**Como são alimentadas:**

1. **Pull periódico** (simples): job lê grafo e regrava projeção.
2. **CDC do grafo** (avançado): `Upsert` do repo emite evento no stream
   hub; listener atualiza projeção. Latência sub-segundo.
3. **Reconstrução total** (recovery): qualquer projeção é descartável.
   Drop + replay garante consistência.

**Trade-off explícito:** projeção é eventualmente consistente com o
grafo. Documentar a janela aceitável por produto (Kanban: <5s, Dashboard:
<5min). Operações que exigem consistência forte vão pelo core API.

---

## 6. O Agente — onde mora, como conversa

### 6.1 Princípio

O agente **não tem dados próprios**. Ele tem:
- Um LLM (Claude, etc.) com capacidade de tool calling.
- Um conjunto de **ferramentas** que são wrappers finos sobre as APIs
  que os produtos já consomem.
- Um runtime de conversa com memória (RAG sobre Documentos + histórico
  do usuário).

Isso garante: **qualquer coisa que o agente pode fazer, um humano também
pode fazer pela UI**. Não existe API privada do agente. Isso é
crítico para auditoria.

### 6.2 Onde fica no repo

Duas opções:

**(a) Subir o agente como `cmd/agent/`** no mesmo monorepo.
- Pró: compartilha tipos, deploy unificado.
- Contra: acopla ciclo de release LLM ↔ core.

**(b) Repo separado**, consumindo a API pública.
- Pró: independente; o agente é apenas mais um consumidor.
- Contra: duplica modelos de dados (DTOs) no cliente.

**Recomendação: (b)**. O agente é um produto, não um módulo de plataforma.
A plataforma fica honesta consigo mesma sobre seu contrato externo.

### 6.3 Tools via MCP — o atalho que vale a pena

Se cada produto (ou pelo menos o backend deles) **expõe um servidor
MCP**, o agente ganha acesso uniforme:

```
agent  ──┬──► MCP: arquitetura  (query graph, simulate)
         ├──► MCP: teams        (find_person_by_role, ownership)
         ├──► MCP: observability(get_alerts, fetch_trace)
         ├──► MCP: documents    (search, create, link)
         ├──► MCP: dashboard    (query_metric, list_drilldown)
         └──► MCP: kanban       (move_card, list_sprint)
```

Cada MCP server é um **adapter fino** sobre a API do produto. Mesma
política de auth, mesma observabilidade, mesma versão de schema. Humanos
usam UI, agentes usam MCP — a regra de negócio é uma só.

### 6.4 Multi-agente — útil ou over-engineering?

Tentação: criar agentes especialistas (ArchAgent, FinOpsAgent,
ProductAgent) com um router. Útil quando:
- Os domínios têm **vocabulário próprio** distinto (FinOps fala em
  Allocation/Burn Rate; Arquitetura fala em latência/topologia).
- Os **prompts ficam grandes demais** se um único agente domina tudo.
- Há **dados sensíveis** que não devem fluir entre contextos (custo
  individual de pessoa, p.ex.).

Cuidados:
- Roteamento entre agentes é fonte de bugs sutis ("o coordenador
  decidiu que era pergunta de Arch, mas era de FinOps").
- Latência se acumula.
- **Comece com agente único** + ferramentas bem nomeadas. Promova para
  multi-agente quando o prompt do agente único passar de ~6k tokens só
  de instruções de domínio.

### 6.5 Memória e RAG

- **Memória de curto prazo**: histórico da conversa (rolling window).
- **Memória de longo prazo**: vetorizar `Documentos` + decisões
  anteriores do agente. Quando consultar a memória vetorial, retornar
  URN do documento → o produto Documentos pode mostrar a fonte.
- **Sem alucinação de URN**: o agente nunca inventa URNs. Quando precisa
  citar uma entidade, chama uma tool que retorna URN + nome humano. URN
  inválida → erro explícito no tool, não chute do modelo.

---

## 7. Documentos como novo plano

Não tínhamos discutido. Encaixa naturalmente:

```
modules/docs/
  node/                # Document, DocumentVersion
  edge/                # nenhum intra-doc relevante (versões viram property)
  ingest/              # markdown files, Confluence, Notion
  search/              # adapter para Meili/OpenSearch
  repo/
  service/
```

**Document node:**
- `urn:ce:docs:<tenant>:document/<id>`
- Campos: title, body_ref (apontador para object store), format (md, adoc),
  kind (runbook, adr, design, postmortem, howto), status, version.
- Bitemporal: versões antigas continuam buscáveis as-of.

**Edge cross-plane (em `bridge/`):**
- `DocumentedBy`: Service|Capability|Compute|Person → Document
- `References`: Document → qualquer URN (links explícitos no markdown
  podem virar edges; ganha-se grafo de citação)
- `WrittenBy`: Document → Person

**Por que vale virar plano e não property:**
- Documentos são **artefatos com ciclo próprio** (versão, revisão,
  aprovação).
- Mesmo documento descreve várias entidades; relação é N-N.
- O grafo de citação entre documentos (`References`) é valioso por si
  ("este postmortem cita 3 runbooks e 2 ADRs").

---

## 8. Source-of-truth matrix

Crítico para evitar dois sistemas brigando pela verdade.

| Entidade | SoT primário | CostEngine | Estratégia |
|---|---|---|---|
| Compute, Persistence, Network | Cloud provider (AWS) | mirror | ingest periódico via Discoverer |
| Service, Endpoint, Function | Código no Git | mirror | ingest via webhook do GitHub Action |
| Person, Team estrutural | HRIS (BambooHR, Workday) | mirror | ingest SCIM |
| Capability, Feature | **CostEngine** | owner | UI Kanban escreve via API |
| UserStory status | Jira/Linear (se existe) | mirror | webhook bidirecional |
| Document | **CostEngine** | owner | escrita pela UI Documentos |
| Budget, Objective | FP&A / Workboard ou CostEngine | mirror ou owner | depende da maturidade |
| Telemetria (métricas) | Prom/Datadog | **não armazena**, só aponta | proxy + enrichment |
| Contract, Revenue | Stripe/ERP | mirror | ingest via webhook/API |

**Regra:** quando há SoT externo, CostEngine espelha e só. Tentar
substituir HRIS/Jira/Stripe perde a guerra; complementá-los, ganha.

---

## 9. Real-time e streaming

Três cenários distintos:

1. **Observabilidade**: latência <5s, volume alto. **Não** armazenar
   séries no CostEngine. Endpoint da API faz **fan-out**: busca dados
   crus em Prom/Datadog/Tempo e *enriquece* com metadados do grafo
   antes de responder. Cache curto (segundos).

2. **Kanban / live updates**: WebSocket dedicado.
   - Conexão por usuário/produto.
   - Tópicos: `tenant:<id>:kanban:sprint:<id>`, `tenant:<id>:dashboard:
     widget:<id>`.
   - Publicado quando `Upsert` no `product` ocorre. Implementação: bus
     interno (Redis Streams, NATS) → stream hub no `cmd/api` →
     WebSocket.

3. **Dashboard refresh**: polling de 1-5min é suficiente para a maioria
   das métricas (custo, MRR, OKR). Não vale a pena pagar pelo
   streaming.

---

## 10. Permissões e multi-tenancy

Cada URN já tem `<tenant>` no segmento de conta. Enforcement de ACL
acontece em **três camadas**:

1. **API auth**: token identifica `tenant_id` + `roles`.
2. **Query rewriter**: toda query Cypher/SQL recebe filtro `tenant`
   automaticamente. Bug aqui = data leak; testar exaustivamente.
3. **Field-level**: dados sensíveis (custo individual de pessoa, salário,
   contratos) atrás de role. Default = mascarar.

Produto vê apenas seu tenant. Agente herda permissões do usuário que o
invoca — **nunca** roda com permissão expandida.

---

## 11. Impacto no monólito modular

A estrutura de `arquitetura-modular.md` continua válida. O que muda:

```
internal/
  core/                       # inalterado
  modules/
    infra/   cost/   code/    # inalterados
    org/     product/         # de arquitetura-negocio.md
    revenue/                  # de arquitetura-negocio.md
    docs/                     # NOVO
    bridge/                   # cresce com edges para docs
    projection/               # NOVO: read models por produto
      kanban/
      documents/
      dashboard/
  app/
    query/                    # resolvers das APIs públicas
    stream/                   # NOVO: stream hub (pub/sub interno)
    simulate/                 # já planejado
    webhook/                  # já planejado
  platform/
    httpserver/               # endpoint REST + WebSocket
    streambus/                # NOVO: NATS/Redis Streams client
    searchindex/              # NOVO: adapter Meili/OpenSearch
cmd/
  cli/                        # extração, ingest, jobs
  api/                        # produtos consomem isto
  # cmd/agent — fora deste repo
```

Princípios mantidos: dependência unidirecional, módulos não se importam,
`bridge/` concentra cross-plane, `platform/` é infra técnica.

---

## 12. Roadmap pragmático

Sequência sugerida (cada passo entrega produto utilizável):

1. **Core API REST** sobre `infra` + `bridge`. Habilita o produto
   **Arquitetura** com queries de grafo.
2. **Auth multi-tenant + ACL básica**. Sem isso, nenhum produto sai.
3. **API sobre `org` + ingest CODEOWNERS**. Habilita produto **Teams**.
4. **WebSocket hub mínimo**. Eventos de upsert publicados; nenhum
   produto consome ainda — preparação.
5. **Projeção Kanban** (Postgres). Habilita produto **Kanban** com sync
   bidirecional Jira opcional.
6. **Plano `docs` + projeção search**. Habilita **Documentos**.
7. **Rollups ClickHouse + API agregada**. Habilita **Dashboard**.
8. **Proxy de observabilidade**. Habilita **Observabilidade** com
   enrichment.
9. **MCP servers** por produto. Agente externo passa a operar.
10. **Multi-agente** apenas se telemetria do agente único justificar.

Cada passo é deployável e gera feedback antes do próximo. Nenhuma etapa
exige reescrita das anteriores.

---

## 13. O que esta arquitetura previne

| Risco | Prevenção |
|---|---|
| Produto novo pede mudança que quebra os outros 5 | Versionamento de API + projeção isolada |
| Dashboard pesado degrada Arquitetura | Storage físico diferente (ClickHouse ≠ Neo4j) |
| Observabilidade vira data lake gigante | Não armazenamos séries — só apontamos |
| Agente faz coisa que humano não pode | Tools = APIs públicas, sem atalho |
| Trocar Jira por Linear vira projeto de 6 meses | Kanban é projeção; troca de SoT é trocar listener |
| Vazamento entre tenants | URN tem tenant; query rewriter enforce em todo path |
| Read model fica inconsistente | É descartável; reconstrução por replay |

---

## 14. Limitações honestas

1. **6 produtos = 6 backlogs concorrentes** para evoluir a API. Sem
   priorização forte, um produto dominante esmaga os outros.
2. **Projeções multiplicam custo operacional.** Cada uma é storage +
   sync + monitoring. Avalie sempre se vale vs query direta.
3. **WebSocket exige sticky session ou bus distribuído.** Não é trivial
   horizontalmente.
4. **Agente com tools é tão bom quanto a documentação das tools.**
   Schema ruim do tool = LLM chuta. Investir em descrição de ferramenta
   é investir em qualidade de agente.
5. **MCP ainda evolui.** Comprometer-se com MCP é apostar num padrão
   que está estabilizando. Manter o adapter MCP fino, o miolo continua
   na API REST/GraphQL — assim pivota se necessário.
6. **Source-of-truth ambígua é veneno.** Definir caso a caso e
   documentar. "Kanban é SoT ou Jira é SoT?" deve ter resposta
   inequívoca por tenant.
