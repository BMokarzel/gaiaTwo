# Modelagem Central — Plataforma de Inteligência Arquitetural

> Documento de referência para a modelagem de domínio do CostEngine: uma plataforma capaz de mapear infraestrutura multi-cloud, extrair relações, associar custos e representar organizações como grafos vivos, versionados e governáveis.

---

## 1. Visão Conceitual

A plataforma é fundamentada em **três planos lógicos** que se sobrepõem sobre um mesmo substrato de grafo:

| Plano | Pergunta que responde | Exemplo |
|------|----------------------|---------|
| **Topológico** | *O que existe e como está conectado?* | VM → VPC → Region → Account |
| **Semântico** | *O que isso significa para o negócio?* | "Esse cluster pertence ao produto X, time Y, centro de custo Z" |
| **Econômico** | *Quanto custa e quem paga?* | Custo unitário, alocação proporcional, custo de rede inter-AZ |

### Princípios norteadores

1. **Tudo é nó, tudo é aresta** — recursos, contas, times, contratos, métricas e até decisões arquiteturais são nós de primeira classe.
2. **Modelo aberto (open-world assumption)** — a ausência de uma relação não significa que ela não exista; significa que ainda não foi observada.
3. **Imutabilidade por padrão** — cada observação é um fato datado; mutações geram novas versões, nunca sobrescrevem.
4. **Schema-on-read sobre core-schema-on-write** — núcleo rígido e mínimo; atributos extensíveis via *property bags* tipados.
5. **Custos como dimensão transversal** — custo não é entidade isolada; é uma *projeção* sobre o grafo num intervalo temporal.
6. **Lineage é cidadão de primeira classe** — toda inferência guarda origem, agente, confiança e timestamp.

---

## 2. Modelagem de Entidades

### 2.1 Núcleo mínimo (core ontology)

O núcleo é deliberadamente pequeno. Qualquer coisa que não couber aqui vira *extensão tipada*.

```text
┌──────────────────────────────────────────────────────────────┐
│                         Node (abstract)                       │
│  id (URN)  ·  kind  ·  version  ·  validFrom  ·  validTo     │
│  source    ·  confidence  ·  labels[]  ·  properties{}        │
└──────────────────────────────────────────────────────────────┘
        ▲
        │
   ┌────┴────────────┬────────────────┬───────────────┬─────────────┐
   │                 │                │               │             │
Resource         Organization      Concept         Cost          Event
(infra real)    (humano/negócio)  (lógico)      (financeiro)   (observação)
```

### 2.2 Entidades fundamentais

#### `Resource` — infraestrutura observada
Qualquer artefato físico/lógico provisionado em um provedor.

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `urn` | `string` | sim | Identificador canônico estável (`urn:ce:aws:123456:ec2/i-abc`) |
| `kind` | `enum` | sim | `compute`, `storage`, `network`, `db`, `function`, `queue`, ... |
| `provider` | `enum` | sim | `aws`, `gcp`, `azure`, `onprem`, `kubernetes`, `custom` |
| `region` / `zone` | `string` | não | Localização física/lógica |
| `tags` | `map<string,string>` | não | Tags nativas do provedor |
| `spec` | `json` | não | Spec bruta do provedor (preservada) |
| `discoveredAt` | `timestamp` | sim | Quando foi observado |

#### `Organization` — estrutura humana/negócio
Times, BUs, produtos, centros de custo, contratos.

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `urn` | `string` | `urn:ce:org:acme/team/payments` |
| `kind` | `enum` | `company`, `bu`, `team`, `product`, `cost-center`, `contract` |
| `parent` | `urn?` | Hierarquia (não é aresta — é cardinalidade-1 inlined para performance) |

#### `Concept` — abstração lógica
Ambientes (`prod`, `staging`), camadas (`data`, `app`), domínios de negócio, tags semânticas, políticas.

#### `Cost` — projeção econômica
Custo **não é nó persistente** isolado: é um **fato derivado** indexado por `(resource_urn, period, dimension)`.

| Campo | Tipo |
|-------|------|
| `subject` | `urn` (qualquer nó) |
| `period` | `interval` (start, end) |
| `currency` | `iso4217` |
| `amount` | `decimal` |
| `model` | `enum` (`on-demand`, `committed`, `spot`, `allocated`, `estimated`) |
| `breakdown` | `map<string,decimal>` (compute, storage, egress, ...) |
| `provenance` | `Lineage` |

#### `Event` — observação imutável
Toda mudança detectada gera evento: `discovered`, `mutated`, `decommissioned`, `cost-billed`, `policy-violated`.

### 2.3 Metadados transversais (em TODA entidade)

```yaml
meta:
  version: 42                    # versão monotônica
  validFrom: 2026-05-13T10:00Z
  validTo:   null                # null = atual
  source:                        # de onde veio o fato
    collector: "aws-discovery"
    runId: "uuid"
    method: "api|inferred|declared|imported"
  confidence: 0.97               # 0..1, para fatos inferidos
  labels: ["env:prod", "pii"]    # tags da plataforma (≠ tags do provedor)
  properties: { ... }            # property bag extensível
  lineage:                       # cadeia causal
    derivedFrom: [urn1, urn2]
    rule: "billing-allocator-v3"
```

---

## 3. Relações

As arestas são **tipadas, direcionadas, versionadas e ponderadas**.

### 3.1 Tipos canônicos

| Tipo | Semântica | Exemplo |
|------|-----------|---------|
| `CONTAINS` | Contenção física/lógica | `VPC` ⟶ `Subnet` |
| `DEPENDS_ON` | Dependência de runtime | `App` ⟶ `Database` |
| `COMMUNICATES_WITH` | Tráfego de rede observado | `Service-A` ⟷ `Service-B` (com `bytes`, `latency_ms`) |
| `OWNED_BY` | Posse / responsabilidade | `Resource` ⟶ `Team` |
| `BILLED_TO` | Imputação financeira | `Resource` ⟶ `CostCenter` |
| `DERIVED_FROM` | Lineage de dados/inferência | `AggregatedCost` ⟶ `RawBillingLine` |
| `IMPLEMENTS` | Concretização de um conceito | `EC2-Instance` ⟶ `Concept:WebTier` |
| `GOVERNED_BY` | Política aplicável | `Resource` ⟶ `Policy` |
| `REPLACES` | Versionamento estrutural | `Resource@v2` ⟶ `Resource@v1` |
| `DEPLOYED_ON` | Hospedagem física | `Compute(Pod)` ⟶ `Compute(Node)` |
| `RUNS_ON` | Service de código mora num Compute (F-009, [ADR-004](decisions/ADR-004-runs-on-edge-for-service-compute.md)) | `Service` ⟶ `Compute` |
| `DEFINED_IN` | Endpoint/Function pertence a Service (F-007) | `Endpoint` ⟶ `Service` |
| `MEMBER_OF` / `PART_OF` / `REPORTS_TO` | Hierarquia organizacional (F-010) | `Person` ⟶ `Squad` ⟶ `Team` |

### 3.2 Propriedades de aresta

```yaml
edge:
  type: COMMUNICATES_WITH
  weight: 1247389        # bytes/s, latência, custo — depende do tipo
  validFrom: ...
  validTo: ...
  confidence: 0.88
  observedBy: "vpc-flow-logs"
  bidirectional: true
```

### 3.3 Hipergrafos quando necessário

Algumas relações são intrinsecamente n-árias (ex.: *"este custo de egress envolve VM-A, VPC-B e Region-C"*). Modeladas como **nó de relação** (reification): cria-se um nó `Flow` que conecta os participantes — preserva a simplicidade do modelo binário sem perder expressividade.

---

## 4. Estratégia de Persistência

### 4.1 Arquitetura poliglota em camadas

```
┌─────────────────────────────────────────────────────────────┐
│  Query layer  (GraphQL + Cypher/Gremlin via gateway único)  │
├─────────────────────────────────────────────────────────────┤
│ Graph store  │  Time-series  │  Object store  │  Search    │
│  (topologia) │  (métricas,   │  (specs brutas,│  (full-text│
│              │   custos)     │   snapshots)   │   tags)    │
│  Neo4j /     │  Timescale /  │  S3 / GCS /    │  Elastic / │
│  JanusGraph  │  InfluxDB     │  Azure Blob    │  OpenSearch│
├─────────────────────────────────────────────────────────────┤
│           Event log  (Kafka / Pulsar — fonte da verdade)    │
└─────────────────────────────────────────────────────────────┘
```

**Princípio fundamental:** o **event log é a fonte da verdade**. Os demais stores são *materializações* reconstruíveis. Isso desacopla o modelo da implementação e habilita re-processamento histórico.

### 4.2 Escolhas e justificativas

| Necessidade | Tecnologia sugerida | Por quê |
|-------------|---------------------|---------|
| Topologia + traversals profundos | Grafo nativo (Neo4j, JanusGraph, Neptune) | Queries de N hops em ms; ACID em subgrafos |
| Custos e séries temporais | TSDB (Timescale, InfluxDB) | Compressão, rollups, retenção tiered |
| Specs brutas (JSON dos providers) | Object store + tabela de índice | Barato, imutável, audit-friendly |
| Busca semântica/textual | OpenSearch + embeddings | "Onde estão os recursos parecidos com X?" |
| Eventos | Kafka (compactado por `urn`) | Replay, lineage, integração assíncrona |

### 4.3 Versionamento (bitemporal)

Toda entidade carrega **dois eixos temporais**:

- `validTime` — *quando o fato vale no mundo real* (ex.: VM existiu de T1 a T2)
- `transactionTime` — *quando o sistema soube* (ex.: descobrimos em T1+5min)

Isso permite responder *"qual era nossa visão da arquitetura em 2026-01-15, segundo o que sabíamos naquela data?"* — essencial para auditoria, debugging de incidentes e SOX/governança.

### 4.4 Particionamento e escala

- **Sharding por tenant + provider account** — isolamento natural e blast radius limitado.
- **Snapshots periódicos** do grafo em formato Parquet/colunar para analytics offline (Spark/Trino).
- **Hot/warm/cold tiering**: últimos 30d quentes, 90d mornos, restante em object store comprimido.

---

## 5. Trade-offs

| Decisão | Ganho | Custo / Risco |
|---------|-------|---------------|
| **Núcleo rígido + extensões livres** | Estabilidade do core; evolução sem migrações | Risco de "property bag soup" — exige governança de schemas de extensão |
| **Event sourcing** | Auditoria total, replay, lineage gratuito | Complexidade operacional; armazenamento; idempotência em consumers |
| **Bitemporal** | Auditoria perfeita; queries históricas | Custo de storage 2-3x; queries mais complexas |
| **Poliglota** | Cada store no que faz melhor | Consistência eventual entre projeções; ops de N sistemas |
| **Grafo nativo** | Traversals expressivos e rápidos | Menos maduro que SQL; tooling menor; vendor lock-in se gerenciado |
| **Custo como projeção (não entidade)** | Flexível, recomputável, sem duplicação | Queries de custo dependem de joins temporais não-triviais |
| **Open-world assumption** | Tolera dados parciais; integração incremental | Queries precisam lidar com "desconhecido ≠ inexistente" |
| **Hipergrafos via reification** | Modela relações n-árias sem mudar engine | Mais nós; queries levemente mais verbosas |

---

## 6. Exemplo Prático

**Cenário:** time de Payments tem um microserviço `checkout-api` rodando em EKS, conversando com RDS e enviando eventos para Kinesis. Queremos saber o custo total atribuído a esse serviço no último mês, incluindo egress de rede.

### Subgrafo (simplificado)

```
(Team:Payments)
   │ OWNED_BY⁻¹
   ▼
(Service:checkout-api)──IMPLEMENTS──▶(Concept:WebTier)
   │                                       │
   │ DEPLOYED_ON                           │ GOVERNED_BY
   ▼                                       ▼
(Deployment:eks/checkout)            (Policy:prod-tier-1)
   │ CONTAINS
   ▼
(Pod:checkout-7f...)──COMMUNICATES_WITH──▶(RDS:payments-db)
   │ RUNS_ON                                  │ BILLED_TO
   ▼                                          ▼
(Node:i-abc123)                       (CostCenter:CC-042)
   │ BILLED_TO
   ▼
(CostCenter:CC-042)
```

### Query conceitual (Cypher-like)

```cypher
MATCH (s:Service {urn:'urn:ce:svc:checkout-api'})
      -[:DEPLOYED_ON|CONTAINS|RUNS_ON*1..4]->(r:Resource)
WITH collect(r) AS resources
MATCH (c:Cost)
WHERE c.subject IN [x.urn | x ∈ resources]
  AND c.period OVERLAPS interval('2026-04-01','2026-05-01')
RETURN
  sum(c.amount)                       AS total,
  sum(c.breakdown.egress)             AS network_cost,
  collect(DISTINCT c.subject)         AS contributing_resources
```

### Fato emitido no event log

```json
{
  "event": "cost.allocated",
  "subject": "urn:ce:svc:checkout-api",
  "period": {"from": "2026-04-01", "to": "2026-05-01"},
  "amount": 4218.77,
  "currency": "USD",
  "breakdown": {"compute": 3104.12, "storage": 412.40, "egress": 702.25},
  "model": "allocated",
  "lineage": {
    "rule": "service-cost-rollup-v2",
    "derivedFrom": [
      "urn:ce:aws:123:ec2/i-abc123",
      "urn:ce:aws:123:rds/payments-db",
      "urn:ce:aws:123:kinesis/checkout-events"
    ],
    "confidence": 0.94
  }
}
```

---

## 7. Evolução Futura

### Curto prazo
- **Coletores plugáveis** via interface única (`Discoverer`) — habilita on-prem, SaaS e clouds menores sem mudar o core.
- **DSL de políticas** sobre o grafo (estilo OPA/Rego) — `deny when resource.kind=='db' and not exists((r)-[:GOVERNED_BY]->(:Policy{encryption:true}))`.

### Médio prazo
- **Embeddings de nós** (Node2Vec / GraphSAGE) para detecção de anomalias e *resource similarity* — *"recursos parecidos com este estão custando 40% menos"*.
- **Simulação contrafactual** — *"se eu mover este workload para outra região, qual o impacto em custo de egress e latência?"* — viável porque o grafo já modela rede + custo.
- **Federação multi-tenant** — grafos por organização com *views* federadas para parceiros/auditores.

### Longo prazo
- **Ontologia compartilhada** (estilo Schema.org para arquitetura) — interoperabilidade entre ferramentas de FinOps, SRE e Security.
- **Knowledge graph + LLM** — agente capaz de responder *"por que o custo de checkout subiu 20% essa semana?"* navegando o grafo, séries temporais e eventos.
- **Auto-healing baseado em lineage** — quando um fato é invalidado, recomputar transitivamente todas as projeções dependentes.

### Princípios para sustentar a evolução
1. **Núcleo fechado, bordas abertas** — mudar o core requer RFC; extensões são livres.
2. **Backward compatibility por versionamento de schema** — nunca quebre consumidores; deprecate com janelas.
3. **Lineage obrigatório** — qualquer feature nova deve preservar a cadeia causal.
4. **Custo computacional é métrica de produto** — o próprio CostEngine deve ser observado pelo CostEngine (dogfooding).

---

> **Resumo executivo:** um core ontológico minimalista (Resource, Organization, Concept, Cost, Event) sobre um substrato de grafo bitemporal, alimentado por event sourcing e materializado em stores poliglotas. Custo é projeção, não entidade. Lineage é obrigatório. Evolução acontece nas bordas, nunca no núcleo.
