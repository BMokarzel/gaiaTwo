# Arquitetura de Custos — Camada Econômica

> Camada econômica do CostEngine: como o CUR da AWS é ingerido, armazenado, vinculado aos nodes de infra e usado para cálculos e simulações. Complementa `modelagem.md` e os pacotes `internal/entity/node` e `internal/entity/edge`.

---

## TL;DR — decisões-chave

1. **Custo NÃO é node do grafo.** É *projeção temporal* sobre nós existentes (modelagem.md §2.2). Volumetria (10⁸–10⁹ linhas/mês) destrói qualquer grafo.
2. **Storage = Parquet em data lake + engine colunar** (Athena/DuckDB/Trino/ClickHouse). CUR já é Parquet — não brigar com o formato nativo.
3. **Bridge = URN.** Mapping `lineItem/ResourceId` ↔ `node.URN` é construído a partir dos nós de infra já descobertos.
4. **Pipeline em 3 camadas** — bronze (CUR cru), silver (normalizado + URN resolvido), gold (rollups indexados).
5. **Nodes de infra precisam de 1 campo novo** (`ExternalID`) e **0 edges novas**. Custo se conecta por *foreign key lógica* (URN), não por aresta.

---

## 1. Por que NÃO node, NÃO NoSQL pesado, e SIM colunar

### Volumetria do CUR

| Item | Ordem de grandeza |
|------|-------------------|
| Linhas/hora por conta média | 10k–500k |
| Linhas/mês conta grande | 10⁸ – 10⁹ |
| Colunas no CUR 2.0 | 100–250 |
| Tipo de query dominante | `SUM(cost) GROUP BY ...` em janelas temporais |
| Cardinalidade típica | alta em `resourceId`, baixa em `productCode` |

### Comparativo

| Opção | Veredito | Por quê |
|-------|----------|---------|
| **Node no grafo** | ❌ | 10⁹ nós/mês quebram traversals. Custo nunca é "vizinho semântico". |
| **DynamoDB / Cassandra** | ❌ para agregação | Bom para lookup pontual; péssimo para `GROUP BY` ad-hoc. |
| **Postgres puro** | ⚠️ só pra gold | Não escala para silver com bilhões de linhas. |
| **TimescaleDB** | ⚠️ útil pra séries agregadas | Limitado em colunas dimensionais (`productCode`, `usageType`, tags…). |
| **Parquet + Athena/Trino/DuckDB** | ✅ silver/bronze | Formato nativo do CUR. Pay-per-query. Schema-on-read. |
| **ClickHouse / Druid** | ✅ gold real-time | Sub-segundo em rollups; alimenta UI. |
| **Iceberg/Delta sobre Parquet** | ✅ silver maduro | ACID, time-travel, schema evolution — combina com bitemporal. |

**Stack alvo:** S3 (Parquet/Iceberg) + Trino/Athena para ad-hoc + ClickHouse (ou Timescale) para gold + Postgres pra metadados (regras de alocação, catálogo de preços).

---

## 2. Modelo de output do CUR

CUR 2.0 (e o legado CUR v1) entrega:

- **Formato**: Parquet (preferido) ou CSV.gz.
- **Particionamento físico no S3**: `s3://.../cur/<report-name>/year=YYYY/month=MM/`
- **Granularidade**: horária ou diária; CUR sobrescreve o mês corrente a cada update (idempotente por `billing_period_start`).
- **FOCUS** (FinOps Open Cost & Usage Spec) — formato padronizado cross-cloud. Vale entregar **silver no schema FOCUS** desde o dia 1.

### Colunas críticas (subconjunto operacional)

| Coluna CUR | Uso |
|---|---|
| `bill_billing_period_start_date` | partição lógica |
| `line_item_usage_start_date` / `..._end_date` | janela do uso |
| `line_item_usage_account_id` | bridge → `Account` node |
| `line_item_product_code` | EC2, RDS, S3… |
| `line_item_usage_type` | `BoxUsage:m6i.large`, `DataTransfer-Out-Bytes`… |
| `line_item_operation` | `RunInstances`, `CreateVolume`… |
| `line_item_resource_id` | **bridge → `node.URN`** (quando habilitado) |
| `line_item_line_item_type` | `Usage`, `Tax`, `Credit`, `Fee`, `RIFee`, `SavingsPlanCoveredUsage`, `DiscountedUsage`… |
| `line_item_usage_amount` | quantidade (h, GB, requests…) |
| `line_item_unblended_cost` | custo on-demand bruto |
| `line_item_net_unblended_cost` | depois de discounts privados |
| `reservation_effective_cost` / `savings_plan_savings_plan_effective_cost` | custo amortizado real |
| `product_region`, `product_instance_type`, `product_storage_class`… | dimensões |
| `pricing_unit`, `pricing_public_on_demand_cost` | catálogo (útil para what-if) |
| `resource_tags_user_*` | tags de negócio (env, team, app) |
| `cost_category_*` | categorias FinOps |

### Pegadinhas

- `resource_id` **vazio** em ~30% das linhas (taxas, suporte, IP público idle, egress agregado). → precisa de regra de alocação.
- `line_item_type` discrimina o "custo real" — somar tudo errado é o bug clássico. Use sempre `effective_cost` derivado.
- RI/Savings Plan: a linha original aparece com custo $0 (`DiscountedUsage`) e a "fatura" vem separada. Para custo por recurso → use:
  `effective_cost = COALESCE(reservation_effective_cost, savings_plan_savings_plan_effective_cost, line_item_net_unblended_cost)`.

---

## 3. Pipeline de extração

```
┌─────────────────────────────────────────────────────────────────────────┐
│  AWS                                                                     │
│  └── BCM Data Exports / Legacy CUR  ─── delivery ──▶  S3 (bronze bucket) │
└──────────────────────────────────────────────────┬──────────────────────┘
                                                   │ S3 EventBridge
                                                   ▼
                                  ┌────────────────────────────────┐
                                  │  Ingestor (Go service)         │
                                  │  - detecta novo manifest       │
                                  │  - publica evento Kafka:       │
                                  │    cur.partition.available     │
                                  └────────────────┬───────────────┘
                                                   ▼
                                  ┌────────────────────────────────┐
                                  │  Transformer (Spark/DuckDB/Go) │
                                  │  - lê bronze                   │
                                  │  - resolve URN (lookup)        │
                                  │  - aplica regras de alocação   │
                                  │  - escreve silver (Iceberg)    │
                                  │  - normaliza para FOCUS schema │
                                  └────────────────┬───────────────┘
                                                   ▼
                       ┌─────────────────────────────────────────┐
                       │   Silver  (Iceberg em S3, partitioned)  │
                       │   PK lógico: (urn, hour, dimension)     │
                       └────────────┬───────────────┬────────────┘
                                    │               │
                       ┌────────────▼──────┐  ┌─────▼──────────────┐
                       │  Rollup builder   │  │  Ad-hoc / Trino    │
                       │  (incremental)    │  │  para exploração   │
                       └────────────┬──────┘  └────────────────────┘
                                    ▼
                       ┌───────────────────────────┐
                       │  Gold (ClickHouse / TS)   │
                       │  - cost_by_urn_hour       │
                       │  - cost_by_service_day    │
                       │  - cost_by_team_month     │
                       └─────────────┬─────────────┘
                                     ▼
                              GraphQL / API / UI
```

### Princípios da pipeline

- **Idempotência por `billing_period_start + run_id`** — CUR reescreve mês corrente; aceitamos *replace-by-partition*.
- **URN resolution é cache local** carregado da view current de `node` (Postgres/grafo), atualizada a cada run.
- **Late-arriving infra**: se uma linha tem `resource_id` que ainda não virou node, vai para *partition unresolved* — reprocessada quando o coletor de infra rodar.
- **FOCUS é o schema canônico do silver**. CUR vira FOCUS → multi-cloud futuro custa zero.

---

## 4. Bridge URN ↔ CUR

### Função de resolução (determinística)

```go
// internal/cost/urn/resolve.go (proposta)
package urn

import "costEngine/internal/entity/node"

// FromCUR converte um lineItem do CUR num URN canônico.
// Retorna URN vazio quando resource_id é nulo (custo não atribuível).
func FromCUR(productCode, accountID, resourceID string) node.URN {
    if resourceID == "" {
        return ""
    }
    kind := mapProductToKind(productCode) // EC2→compute, RDS→persistence, etc.
    return node.URN(fmt.Sprintf("urn:ce:aws:%s:%s/%s",
        accountID, kind, lastSegment(resourceID)))
}
```

Por isso `Resource` ganha **um campo só**:

```go
// adicionar em node/node.go na Resource interface:
type Resource interface {
    Node
    Provider() ProviderID
    AccountURN() URN
    RegionURN() URN
    ExternalID() string          // ← NOVO. ID nativo no provedor (ARN, instance-id).
    NativeTags() map[string]string
    Spec() map[string]any
}
```

E `Compute/Persistence/Messaging/Network` ganham `ExternalID string` no struct + getter. **Nenhuma edge nova.** A ligação custo↔node é foreign-key lógica via URN — tabela colunar não vai para o grafo.

---

## 5. Modelagem da camada de custo

### 5.1 Tabela silver (Iceberg / Parquet)

Schema FOCUS-aligned + extensões internas:

```sql
CREATE TABLE silver.cost_line (
    -- bitemporal
    billing_period_start TIMESTAMP,
    usage_start          TIMESTAMP,
    usage_end            TIMESTAMP,
    ingested_at          TIMESTAMP,

    -- bridge
    provider             STRING,    -- "aws"
    account_id           STRING,
    account_urn          STRING,    -- urn:ce:aws:123:account/123
    resource_id          STRING,    -- raw do CUR
    resource_urn         STRING,    -- resolvido; vazio se unattributable
    resource_kind        STRING,    -- compute/persistence/...
    region               STRING,
    region_urn           STRING,

    -- categorização
    service              STRING,    -- ProductCode normalizado
    usage_type           STRING,
    operation            STRING,
    charge_category      STRING,    -- Usage|Tax|Credit|Fee|... (FOCUS)
    pricing_model        STRING,    -- on_demand|ri|sp|spot

    -- métrica
    usage_amount         DECIMAL(20,8),
    usage_unit           STRING,
    list_cost            DECIMAL(20,8),  -- on-demand público
    billed_cost          DECIMAL(20,8),  -- net_unblended
    effective_cost       DECIMAL(20,8),  -- amortizado (RI/SP)
    currency             STRING,

    -- contexto
    tags                 MAP<STRING,STRING>,
    cost_category        MAP<STRING,STRING>,

    -- lineage
    source_run_id        STRING,
    source_file          STRING
)
PARTITIONED BY (billing_period_start, provider, resource_kind);
```

### 5.2 Tabelas gold (queryáveis em ms)

```sql
-- ClickHouse / Timescale
cost_by_urn_hour     (urn, hour, dim_breakdown, cost)        -- série por recurso
cost_by_service_day  (service, region, day, cost)            -- dashboards
cost_by_account_day  (account_urn, day, cost)
network_egress_hour  (src_urn, dst_zone, hour, gb, cost)     -- p/ simulação
unattributed_day     (charge_category, day, cost, reason)
```

### 5.3 Pacote Go sugerido

```
internal/
 ├── entity/
 │   ├── node/        (existente)
 │   ├── edge/        (existente)
 │   └── cost/        ← NOVO
 │       ├── line.go         (CostLine — uma linha silver tipada)
 │       ├── projection.go   (CostProjection — agregação para uma URN/período)
 │       ├── allocation.go   (AllocationRule — alocação de custos órfãos)
 │       ├── catalog.go      (PriceCatalog — preço público p/ what-if)
 │       └── ingestion.go    (interface CURImporter)
 └── repository/
     ├── aws/
     │   └── cur/           ← NOVO  (S3 listing, manifest parse, parquet read)
     ├── lake/              ← NOVO  (Iceberg writer, partition swap)
     ├── olap/              ← NOVO  (ClickHouse/Timescale gateway)
     └── n4j/               (graph repo, existente)
```

#### Tipos de domínio

```go
package cost

import (
    "time"
    "costEngine/internal/entity/node"
)

// ChargeCategory alinhado com FOCUS.
type ChargeCategory string
const (
    ChargeUsage      ChargeCategory = "usage"
    ChargePurchase   ChargeCategory = "purchase"
    ChargeTax        ChargeCategory = "tax"
    ChargeAdjustment ChargeCategory = "adjustment"
    ChargeCredit     ChargeCategory = "credit"
)

type PricingModel string
const (
    PricingOnDemand PricingModel = "on_demand"
    PricingRI       PricingModel = "reserved"
    PricingSP       PricingModel = "savings_plan"
    PricingSpot     PricingModel = "spot"
)

// CostLine é uma linha silver tipada (1:1 com o schema).
type CostLine struct {
    BillingPeriod  time.Time
    UsageStart     time.Time
    UsageEnd       time.Time
    AccountURN     node.URN
    ResourceURN    node.URN          // vazio = não atribuído
    ResourceKind   node.Kind
    RegionURN      node.URN
    Service        string
    UsageType      string
    Operation      string
    Charge         ChargeCategory
    Pricing        PricingModel
    UsageAmount    float64
    UsageUnit      string
    ListCost       float64           // on-demand público
    BilledCost     float64           // o que sai do bolso (líquido)
    EffectiveCost  float64           // amortizado — usar em rollups
    Currency       string
    Tags           map[string]string
}

// CostProjection é uma agregação para (subject, period, dimension).
// Nunca persiste como node — é resultado de query.
type CostProjection struct {
    Subject    node.URN
    Period     Period            // [start, end)
    Dimension  string            // "service" | "usage_type" | "region" | ...
    Breakdown  map[string]float64
    Total      float64
    Currency   string
    Confidence float32
    Lineage    node.Lineage
}

type Period struct {
    Start, End time.Time
}
```

#### Importer interface

```go
package cost

// CURImporter abstrai a origem (AWS CUR hoje, GCP/Azure futuro).
type CURImporter interface {
    // Discover detecta partições disponíveis desde "since".
    Discover(ctx context.Context, since time.Time) ([]Partition, error)
    // Read entrega um stream de CostLine normalizadas.
    Read(ctx context.Context, p Partition) (<-chan CostLine, error)
}

type Partition struct {
    Provider     node.ProviderID
    BillingMonth time.Time
    URI          string   // s3://...
    Manifest     string
    RunID        string
}
```

### 5.4 Regras de alocação (custos órfãos)

Linhas sem `resource_id` (taxa, suporte, egress agregado) precisam de **regras**:

```go
type AllocationRule struct {
    ID        string
    Match     LineMatcher       // critério: service="Tax", region="*", ...
    Strategy  AllocationStrategy
    Weight    string            // "billed_cost" | "usage_hours" | "equal"
    TargetURN node.URN          // ou expressão (e.g., "team owners")
}

type AllocationStrategy string
const (
    StrategyProportional AllocationStrategy = "proportional" // % por uso
    StrategyEvenSplit    AllocationStrategy = "even_split"
    StrategyFixed        AllocationStrategy = "fixed"        // alvo único
    StrategyTagOverride  AllocationStrategy = "tag_override" // usa resourceTags
)
```

Resultado: cada linha "órfã" gera N linhas derivadas com `lineage.rule = "allocator/<id>"` e `confidence < 1`. Audit perfeito.

---

## 6. Cálculos e simulações habilitados

### Diretos (1 query no gold)

1. **Custo por recurso** — qualquer janela.
2. **Custo por serviço/região/conta/tag** — pivot trivial.
3. **Top movers** — `WoW`, `MoM` deltas, `% change`.
4. **Forecast simples** — regressão linear / Prophet sobre `cost_by_*_day`.

### Que dependem do GRAFO (joins URN ↔ infra)

5. **Custo por time/produto/ambiente** — traversa `OWNED_BY`/`IMPLEMENTS` no grafo, coleta URNs folha, soma no gold.
6. **Custo "total" de um microserviço** — pega subgrafo via `DEPLOYED_ON`/`ATTACHED_TO`/`DEPENDS_ON`, soma.
7. **Egress atribuído** — combina `CommunicatesWith.BytesOut` com linhas `DataTransfer-Out-Bytes`. Sem o grafo de rede, egress fica como "custo de conta", não de serviço.
8. **Blast radius econômico** — "se eu deletar este RDS, quanto deixo de gastar?" → soma do recurso + dependências exclusivas.

### Simulações (precisam de `PriceCatalog`)

9. **What-if region migration** — re-precificar `usage_amount × catalog[target_region]`.
10. **What-if instance resize** — substituir `m6i.large → m6i.medium`, recalcular.
11. **RI/SP coverage** — given current usage, qual commitment maximiza saving?
12. **Spot conversion** — % de workloads elegíveis × Δ rate.
13. **Egress avoidance** — "se eu mover este DB para a mesma AZ do serviço X, economizo Y" — usa `Network` + `CommunicatesWith.CrossZone`.
14. **Shutdown windows** — apagar dev/staging fora do horário, simular sobre série horária.

### Avançadas

15. **Anomaly detection** — z-score / IsolationForest por `(urn, service)` em janelas móveis.
16. **Causal attribution** — quando custo de A sobe e A `DEPENDS_ON` B, verificar correlação no grafo.
17. **Counterfactual lineage** — "por que o custo subiu?" → grafo + lineage rastreia a causa raiz (deploy, scaling, tag mudou).
18. **Carbon co-projection** — `cost_line × region_carbon_factor × usage_amount` → CO₂e por URN.

---

## 7. O que muda nos nodes de infra

| Mudança | Onde | Por quê |
|---------|------|---------|
| `ExternalID string` no Resource interface | `node/node.go` | Bridge direto sem parsear URN |
| Campo `ExternalID` em Compute/Persistence/Messaging/Network | respectivos arquivos | Idem |
| **Nenhuma edge nova** | — | Custo é projeção, não vizinho semântico |
| Index secundário: `(provider, account, external_id) → urn` | `repository/n4j/` | Hot path da resolução no ingest |

Nada além disso. O grafo permanece enxuto; toda a complexidade do custo vive no pacote `cost/` + data lake.

---

## 8. Trade-offs

| Decisão | Ganho | Custo |
|---|---|---|
| **Custo fora do grafo** | Grafo continua rápido; OLAP nativo para agregação | Joins URN-to-cost são responsabilidade da API, não free do grafo |
| **FOCUS no silver** | Multi-cloud futuro grátis; vocabulário padrão | Mapping CUR→FOCUS adiciona uma etapa |
| **Iceberg/Parquet** | Time-travel; partition evolution; ACID em S3 | Operacional menor que Snowflake/BigQuery |
| **Gold em ClickHouse** | Sub-segundo em rollups; barato | Mais um sistema; eventually consistent |
| **AllocationRule explícita** | Auditável; reversível; `lineage` preserva origem | Requer UI/governança para os times definirem regras |
| **`effective_cost` como métrica padrão** | Reflete realidade financeira (pós RI/SP) | Não é o que aparece no console AWS — exige educação |
| **URN como FK** | Desacoplado, multi-store | Requer ingestor robusto para late-arriving infra |

---

## 9. Roadmap mínimo de implementação

1. **`node.Resource.ExternalID`** — patch nos structs (5 arquivos, ~30 LOC).
2. **`internal/entity/cost/`** — types + interfaces (`CostLine`, `CostProjection`, `CURImporter`, `AllocationRule`).
3. **`internal/repository/aws/cur/`** — leitor de manifest + Parquet streaming. Output: `<-chan CostLine`.
4. **`internal/repository/lake/`** — writer Iceberg silver (start: Parquet bruto, evoluir).
5. **URN resolver** — cache em memória alimentado do grafo current view.
6. **Allocator** — engine declarativa (YAML/DB-driven).
7. **Gold rollups** — jobs incrementais (Materialize / dbt / Go puro).
8. **Query API** — `GET /cost?subject=urn:...&period=...&groupBy=service`.
9. **Simulator** — pacote `cost/sim` consumindo `PriceCatalog`.

---

## Resumo executivo

- **NoSQL, SQL ou node?** Nenhum dos três como primário — **colunar (Parquet/Iceberg)** para silver, **colunar OLAP (ClickHouse)** para gold, **Postgres** para metadados (regras, catálogo).
- **CUR output:** Parquet horário no S3, sobrescrito por mês — schema FOCUS-compatível recomendado.
- **Extração:** event-driven via S3 EventBridge → ingestor Go → transformer (DuckDB/Spark) → Iceberg silver → rollups gold.
- **Vinculação:** `lineItem/ResourceId` → função determinística → `node.URN`. Tabela silver guarda o URN como FK lógica.
- **Infra nodes precisam?** Sim: 1 campo `ExternalID` no Resource interface + structs. Zero edges novas.
- **Cálculos/simulações:** ~18 famílias possíveis, das mais simples (top spend) até counterfactuals via grafo (egress se mover AZ, ROI de RI, blast radius econômico).
