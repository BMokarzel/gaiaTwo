# Arquitetura de Negócio: Organização, Produto e Lucro

> Extensão do CostEngine para representar **empresa, pessoas, produtos e
> receita** como cidadãos de primeira classe do grafo. Permite responder:
> *quanto custa uma capability?*, *qual a margem deste produto?*, *o gasto
> está alinhado com a estratégia?*, *qual o ROI deste time/ferramenta?*.
>
> Complementa `modelagem.md` (infra), `arquitetura-custos.md` (custo) e
> `arquitetura-modular.md` (organização do código).

---

## 1. TL;DR

Adicionamos **três planos novos** ao grafo, mantendo o mesmo padrão
bitemporal e a mesma URN canônica:

| Plano | Razão de existir | Storage primário |
|---|---|---|
| `org` | Hierarquia empresarial, pessoas, budget, OKRs | Neo4j |
| `product` | Capabilities, features, produtos, contratos | Neo4j |
| `revenue` | Receita observada por (produto, período, cliente) | Parquet + ClickHouse |

A **mesma engine de alocação** que distribui custo da AWS sobre URNs é
reutilizada para distribuir:
- **receita** sobre features/capabilities,
- **custo de pessoas** sobre features/squads,
- **custo de ferramentas** (Claude, SaaS) sobre features/times.

Resultado: o grafo deixa de responder só "*quanto a infra está custando?*"
e passa a responder "*esta capability tem margem positiva? está alinhada
com OKR? está dentro do budget do seu BusinessArea?*".

---

## 2. Novos nós

### 2.1 Plano `org` (empresa e pessoas)

```
Company
  └─ Contains ─► BusinessArea          (ex.: Engineering, Finance, Product)
                     │
                     ├─ Contains ─► Team
                     │                 │
                     │                 └─ Contains ─► Squad
                     │                                  │
                     │                                  └─ Contains ─► Person
                     │
                     └─ Owns ─► Budget (bitemporal, por período)

Person ─ Owns ─► Service | Function | Endpoint     (bridge → code)
Squad  ─ Maintains ─► Service                       (bridge → code)
Team   ─ AccountableFor ─► Capability               (bridge → product)
```

**Kinds:**

| Kind | URN exemplo | Campos chave |
|---|---|---|
| `Company` | `urn:ce:org:acme:company/root` | name, hq_region, fiscal_year_start |
| `BusinessArea` | `urn:ce:org:acme:business_area/eng` | name, parent_company_urn, head_person_urn |
| `Team` | `urn:ce:org:acme:team/platform` | name, business_area_urn, lead_urn |
| `Squad` | `urn:ce:org:acme:squad/checkout` | name, team_urn, mission |
| `Person` | `urn:ce:org:acme:person/<hash>` | role, level, squad_urn, fully_loaded_cost_per_period, capacity_pct |
| `Budget` | `urn:ce:org:acme:budget/eng-2026` | owner_urn (BusinessArea/Team), period, amount, currency, category |
| `StrategicObjective` | `urn:ce:org:acme:objective/grow-arr-30` | name, owner_urn, target, deadline |
| `KeyResult` | `urn:ce:org:acme:kr/<id>` | objective_urn, metric, baseline, target, current |

> ⚠ **PII**: `Person.urn` usa **hash** do identificador interno (employee id).
> Dados sensíveis (nome, email, salário absoluto) **não** vivem no grafo —
> ficam num store separado com lookup por URN sob ACL. O grafo guarda
> `fully_loaded_cost` apenas se a política da empresa permitir; caso
> contrário, guarda **faixa** (band) ou **role-rate** (custo médio por
> role × nível).

### 2.2 Plano `product` (o que a empresa entrega)

```
Domain  (opcional, taxonomia)
   │
   └─ Contains ─► Capability
                     │
                     └─ RealizedBy ─► Feature
                                        │
                                        └─ DecomposesInto ─► UserStory

Product ─ Bundles ─► Feature
Offering ─ Includes ─► Product
Contract ─ Sells ─► Offering         (origem da receita)
```

**Kinds:**

| Kind | URN exemplo | Campos chave |
|---|---|---|
| `Domain` | `urn:ce:product:acme:domain/payments` | name, owner_business_area_urn |
| `Capability` | `urn:ce:product:acme:capability/process-refund` | name, domain_urn, description, status |
| `Feature` | `urn:ce:product:acme:feature/refund-self-service` | name, capability_urn, status, jira_key |
| `UserStory` | `urn:ce:product:acme:story/<id>` | feature_urn, points, status |
| `Product` | `urn:ce:product:acme:product/checkout-pro` | name, sku, lifecycle_stage |
| `Offering` | `urn:ce:product:acme:offering/checkout-pro-enterprise` | product_urn, tier, list_price |
| `Customer` | `urn:ce:product:acme:customer/<hash>` | cohort, plan, signup_at (PII por hash) |
| `Contract` | `urn:ce:product:acme:contract/<id>` | customer_urn, offering_urn, mrr, started_at, ends_at |

### 2.3 Sobre `Domain` — vale a pena?

**Provavelmente não no MVP.** `Domain` é uma camada taxonômica que se
justifica quando há **muitas Capabilities** (>50) e múltiplas BusinessAreas
disputando o mesmo conceito (ex.: "Identity" tocado por Eng e por Security).
Para um portfólio inicial, `BusinessArea → Capability → Feature` cobre.
Modela-se `Domain` quando aparecer a dor real: queries de capability que
precisam agrupar por contexto de produto. **Não pague o custo de modelagem
antes da dor existir.**

### 2.4 Plano `revenue` (entrada de dinheiro)

Espelho do plano `cost`. Não é grafo — é tabela colunar.

```
revenue_fact:
  urn_contract, urn_offering, urn_customer
  period_start, period_end
  amount, currency
  type:  recurring | one_time | usage_based
  source: stripe | erp | manual
```

A engine de alocação rateia `revenue_fact` sobre features/capabilities
seguindo o mesmo padrão do CUR rateando custo sobre URNs de infra.

---

## 3. Edges novas (e onde moram)

Aplica-se a regra de `arquitetura-modular.md`: **edges intra-plano** ficam
no módulo do plano; **edges cross-plane** ficam em `modules/bridge/`.

### Intra-plano

| Plano | Edges |
|---|---|
| `org` | `Contains` (Company→BusinessArea→Team→Squad→Person), `Reports` (Person→Person), `Owns` (BusinessArea→Budget), `Pursues` (BusinessArea→Objective→KeyResult) |
| `product` | `Contains` (Domain→Capability), `RealizedBy` (Capability→Feature), `DecomposesInto` (Feature→UserStory), `Bundles` (Product→Feature), `Includes` (Offering→Product), `Sells` (Contract→Offering) |

### Cross-plane (em `bridge/`)

| Edge | De → Para | Significado |
|---|---|---|
| `Owns` | Person → Service \| Function \| Endpoint | Code ownership (substitui CODEOWNERS) |
| `Maintains` | Squad → Service | Squad responsável operacional |
| `AccountableFor` | Team → Capability | Time que entrega a capability |
| `ImplementedBy` | Feature → Service \| Endpoint \| Function | Onde a feature vive em código |
| `Touches` | UserStory → Endpoint \| Function | Caminho de execução da história |
| `Supports` | Capability → Objective \| KeyResult | Alinhamento estratégico |
| `Charges` | Offering → Capability | Capability vendida (origem de receita) |
| `BudgetCovers` | Budget → BusinessArea \| Team \| Capability | Escopo do budget |
| `Consumes` | Person \| Team → Tool | Uso de SaaS/LLM (origem de custo) |

A matriz de adjacência em `core/edge/` cresce — mas o crescimento é
linear no número de Kinds, e cada par é declarado uma única vez.

### Sobre `Tool` e o custo do Claude/SaaS

`Tool` é um sub-kind de `Resource` (já existe a interface no plano infra),
hospedado no plano `org` por proximidade contábil:

```
Tool (Kind=Tool, flavor in {saas, llm, observability, ide, ...})
  ├─ provider: "anthropic" | "datadog" | "atlassian"
  ├─ pricing_model: per_seat | per_token | per_event | flat
  └─ external_id: tenant/account no provedor
```

Consumo:
- **Per seat** (Jira, Datadog, GitHub): `Person ─Consumes─► Tool`
- **Per token** (Claude, GPT): emitido como `ExternalCall` do Service para
  `Tool`, com `tokens_in/tokens_out` no edge. A engine de custo aplica a
  pricing function.
- **Per event** (Segment, Mixpanel): emitido como métrica no `Telemetry`
  ligado ao Service, com tarifa por evento.

Resultado: custo do Claude **não é uma linha solta numa planilha**. É um
caminho no grafo `Service ─CommunicatesWith─► Tool(claude)`, com volume
mensurado e rateável.

---

## 4. Onde isso mora — extensão do monólito modular

Acrescentam-se dois (ou três) módulos novos:

```
internal/modules/
  infra/                # existente
  cost/                 # existente
  code/                 # existente
  org/                  # NOVO
    node/               # Company, BusinessArea, Team, Squad, Person,
                        #   Budget, Objective, KeyResult, Tool
    edge/               # Contains, Reports, Owns(budget), Pursues, Consumes
    ingest/             # adapters: BambooHR, Workday, CSV, Okta (SCIM)
    repo/
    service/

  product/              # NOVO
    node/               # Domain, Capability, Feature, UserStory,
                        #   Product, Offering, Customer, Contract
    edge/               # Contains, RealizedBy, DecomposesInto, Bundles,
                        #   Includes, Sells
    ingest/             # adapters: Jira, ProductBoard, Salesforce, Stripe
    repo/
    service/

  revenue/              # opcional separado, ou subpacote de cost/
    domain/             # RevenueFact espelho de CURRow
    ingest/             # Stripe, ERP, manual CSV
    allocate/           # reusa a engine de alocação do módulo cost
    rollup/
    repo/

  bridge/               # crescido
    edge/               # adiciona Owns(code), Maintains, AccountableFor,
                        #   ImplementedBy, Touches, Supports, Charges,
                        #   BudgetCovers
    service/            # resolvers de cross-plane (ex.: jira_key → feature
                        #   key → ImplementedBy services tocados no PR)
```

**Sobre separar `revenue` de `cost`:** o custo *contábil* de manter um
módulo a mais é baixo se a engine de alocação é compartilhada. Recomendo
**começar como subpacote** de `cost/` (`cost/revenue/`) e promover para
módulo próprio quando aparecer pipeline de ingestão substantivamente
diferente (ex.: Stripe webhooks em tempo real ≠ CUR batch diário).

---

## 5. A engine de alocação generalizada

Aqui está o insight central: **custo, receita, tempo de pessoa e tokens
de LLM são todos fluxos numéricos sobre o espaço (URN × período × dimensão)**.
A mesma engine resolve todos.

### 5.1 Generalização

```go
type Flow struct {
    Kind      FlowKind          // cost | revenue | effort | usage
    Currency  string            // USD, BRL, hours, tokens
    Amount    decimal.Decimal
    Period    BillingPeriod
    SourceURN core.URN          // de onde veio (CUR line, Stripe charge, JIRA worklog)
    Tags      map[string]string // raw, pré-alocação
}

type AllocationRule interface {
    Match(Flow) bool
    Distribute(Flow, ctx GraphCtx) []Allocation
}

type Allocation struct {
    FlowID    string
    TargetURN core.URN          // urn:ce:product:.../feature/X, urn:ce:infra:.../compute/Y
    Amount    decimal.Decimal
    Weight    float64           // 0..1, para auditoria
    Method    string            // "tag-match" | "time-weighted" | "even-split"
}
```

Regras concretas:

- **Custo de infra → Feature** via cadeia `Compute ←DeployedOn─ Service ←ImplementedBy─ Feature`. Quando 1 Service implementa N features, ratear por (a) tags explícitas no PR, (b) volume de tráfego (`Telemetry`), (c) split igual.
- **Receita de contrato → Feature** via cadeia `Contract → Offering → Product → Feature`. Quando 1 Offering inclui N produtos, atribuir conforme campo `list_price_share` ou peso configurado.
- **Custo de pessoa → Feature** via `Person → Squad → AccountableFor → Capability → RealizedBy → Feature`. Ponderado por `time_allocation` reportado (Jira worklog, Toggl, ou heurístico: PRs autorados por feature).
- **Tokens de Claude → Feature** via `Service ─CommunicatesWith─► Tool(claude)` × `ImplementedBy(feature)`. Tokens × preço/token = custo.

### 5.2 Auditabilidade

Toda `Allocation` guarda o `method` e o `weight`. Resultado: para qualquer
total agregado, é possível **drill-down** até o flow original ("este R$
12.000 alocado para a capability `process-refund` vem de: R$ 8.000 do RDS
`db-refunds`, R$ 3.500 do salário rateado de 2 devs, R$ 500 em tokens
Claude"). Sem isso, o relatório vira mágica e ninguém confia.

---

## 6. Budget — orçamento como nó vivo

`Budget` é um nó bitemporal porque budgets **mudam**: revisão de Q2,
remanejamento, congelamento. Modelar como nó (não como propriedade)
permite:

- **Versionar** alterações com `ValidFrom/ValidTo`.
- **Justificar** mudanças via `Lineage.Rule` (ex.: "revisão de meio de
  ano por board").
- **Comparar** com `Allocation` no mesmo período (variance analysis).

### Queries possíveis

```cypher
// Quanto a Engineering gastou vs orçou no Q2-2026?
MATCH (ba:CeNode {kind:'BusinessArea', urn:'urn:ce:org:acme:business_area/eng'})
MATCH (ba)-[:OWNS]->(b:CeNode {kind:'Budget'})
WHERE b.period_start = date('2026-04-01') AND b.valid_to IS NULL
WITH ba, b.amount AS budget
MATCH allocation_path  // soma via engine
RETURN budget, spent, (budget - spent) AS variance
```

### Alertas e governança

- **Burn rate**: `spent / elapsed_pct_of_period > 1.1` → alerta.
- **Orphan spend**: custo sem `BudgetCovers` que aponte para algum Budget
  → flag de "gasto sem dono orçamentário".
- **Forecast**: extrapolação linear do burn atual contra deadline.

---

## 7. Alinhamento estratégico

`StrategicObjective` e `KeyResult` são primeira classe. A pergunta
"**estamos gastando no que importa?**" vira query:

```
% spend aligned = Σ cost(capability) where capability -[Supports]-> objective
                  ÷ Σ cost(all capabilities)
```

**Casos detectáveis:**

1. **Spend órfão**: features sem `RealizedBy` ⇒ capability ⇒ objective.
   Custa, não sustenta nenhum OKR declarado. Material para conversa de
   priorização.
2. **Capability subfinanciada**: capability marcada como "estratégica" no
   plano anual mas com custo < threshold ⇒ gap entre discurso e execução.
3. **Drift**: capability que era estratégica em Q1 e deixou de ser em Q3
   continua consumindo budget ⇒ candidate to sunset.

### Limitação honesta

Alinhamento estratégico **depende de classificação humana** correta. Se
o time não vincula `Feature ─RealizedBy─► Capability ─Supports─► Objective`,
o sistema só sabe o que foi declarado. A automação pode sugerir vínculos
via NLP em descrições de Jira/PR — mas a fonte da verdade é humana.

---

## 8. Margem e impacto no lucro

### 8.1 Margem por capability

```
margin(capability, period) =
    revenue_allocated(capability, period)
  - infra_cost_allocated(capability, period)
  - people_cost_allocated(capability, period)
  - tool_cost_allocated(capability, period)
```

Cada parcela vem da mesma `Allocation` table, filtrada por `FlowKind`.

### 8.2 Margem por produto

Produto agrega capabilities via `Bundles → Feature → RealizedBy → Capability`.
Cuidado: features podem ser compartilhadas entre produtos. Atribuição:
- Por peso explícito (`Bundles.weight`), ou
- Proporcional ao uso observado (`Telemetry` por endpoint vs cliente).

### 8.3 Impacto no lucro

Lucro = `Σ revenue - Σ cost` agregado no escopo da empresa, do
BusinessArea ou da capability. O grafo permite **decompor** a variação
do lucro entre períodos:

```
ΔProfit = ΔRevenue - ΔInfraCost - ΔPeopleCost - ΔToolCost
```

E **atribuir** cada delta a uma causa raiz:
- `ΔInfraCost(capability-X) = +12k` ← rastrear para qual Service cresceu,
  qual Compute foi escalado, em que data, por qual PR (via `code/edge`
  + `Lineage`).
- `ΔRevenue(product-Y) = -8k` ← rastrear para churn em contracts
  específicos.

### 8.4 Simulações de impacto financeiro

Combinando o grafo com cenários hipotéticos:

| Simulação | Mecânica |
|---|---|
| "Quanto custa migrar svc-X para outra região?" | recomputa infra cost com novo `Region`; mantém revenue; calcula ΔProfit |
| "Qual o impacto de remover Capability Z?" | zera custo das features, zera receita atribuída, mostra delta |
| "Se aumentarmos 30% o tráfego no Product P?" | escala usage no `Telemetry`; recomputa cost via pricing; mantém revenue/contract |
| "ROI de adotar Claude no time A?" | adiciona Tool(claude) cost; comparar com Δ entrega (PRs/feature) se houver baseline |

---

## 9. Limitações e armadilhas conhecidas

1. **Atribuição de pessoa a feature é frágil.** Worklog completo é raro.
   Heurísticas: PRs com label de feature, tempo entre primeiro/último
   commit no path do feature, ou (mais simples) split igual entre as
   features do squad no período. Documente o método; valide ocasionalmente.
2. **Dados de salário são politicamente sensíveis.** Default: usar
   **role-rate** (custo médio por cargo×nível, vindo de pesquisa de
   mercado ou HR agregado). Salários individuais ficam fora do grafo;
   só HR resolve a tradução, e apenas quando necessário e autorizado.
3. **Receita raramente cola perfeitamente em feature.** Contratos
   enterprise são vendidos por valor total, não por feature. Atribuição
   por uso observado (telemetria por cliente × endpoint × feature) é a
   melhor aproximação, mas tem erro. Aceite e documente.
4. **OKRs mudam.** Modelar `Objective` como bitemporal é essencial; senão
   o relatório histórico vira ficção.
5. **Capabilities/Features sem dono geram a maior parte do ruído.**
   Antes de tirar relatório executivo, validar cobertura: % de
   features sem `Capability`, % de services sem `Squad`, % de custo
   sem `Feature` atribuída.
6. **PII e LGPD/GDPR.** `Person`, `Customer`: usar hash + tabela lateral.
   Logs do CostEngine **não devem** carregar nome/email. Auditoria de
   acesso aos dados sensíveis em store separado.
7. **Granularidade da feature.** Feature deve ser observável no código
   (label em PR, path, anotação) — senão a atribuição vira chute. Se a
   organização não tem cultura de tag, comece por Capability e adicione
   Feature quando o processo amadurecer.
8. **Receita atrasa o cost.** CUR fecha M+1; Stripe é tempo real;
   contratos enterprise são reconhecidos por accounting rules (ASC 606).
   A engine de margin precisa lidar com **horizonte de fechamento** —
   margem provisória vs margem fechada.

---

## 10. Ingestão de dados — quem alimenta o quê

| Plano | Fonte primária | Cadência | Adapter |
|---|---|---|---|
| `org` (estrutura) | BambooHR / Workday / Okta SCIM | diária | `org/ingest/hris/` |
| `org` (ownership código) | CODEOWNERS + git blame agregado | por commit | `org/ingest/codeowners/` (chama bridge) |
| `org` (budget) | planilha / NetSuite / SAP | mensal/trimestral | `org/ingest/finance/` |
| `org` (OKR) | manual / ProductBoard / Workboard | trimestral | `org/ingest/okr/` |
| `product` (capability/feature) | Jira / Linear / Notion | semanal | `product/ingest/jira/` |
| `product` (produto/offering) | catálogo interno / Salesforce | mensal | `product/ingest/catalog/` |
| `revenue` (contratos) | Stripe / Chargebee / ERP | diária ou tempo real | `revenue/ingest/stripe/` |
| `revenue` (usage) | telemetria + tarifa | hora/dia | `revenue/ingest/usage/` |
| `tools` (Claude tokens) | Anthropic console API | diária | `org/ingest/anthropic/` |
| `tools` (SaaS) | provedor (Datadog API, etc.) | diária | `org/ingest/saas/` |

Todos os adapters seguem o padrão do `Collector` definido em
`arquitetura-modular.md`: interface no módulo, implementação em subpacote,
injeção pelo `cmd/cli`.

---

## 11. Queries de exemplo (Cypher pseudo)

**1. Margem por capability no último trimestre**

```
MATCH (c:CeNode {kind:'Capability'})
OPTIONAL MATCH (c)<-[:REALIZED_BY]-(f:CeNode {kind:'Feature'})
WITH c, collect(f) AS features
CALL apoc.cypher.run('compute_allocation', {capability: c.urn, period:'2026-Q1'}) YIELD revenue, cost
RETURN c.name, revenue, cost, (revenue - cost) AS margin
ORDER BY margin DESC
```

**2. Gasto por BusinessArea vs Budget**

```
MATCH (ba:CeNode {kind:'BusinessArea'})-[:OWNS]->(b:CeNode {kind:'Budget'})
WHERE b.period = '2026-Q1' AND b.valid_to IS NULL
WITH ba, b
MATCH (ba)<-[:BUDGET_COVERS*]-(allocation_root)   // engine resolve
RETURN ba.name, b.amount AS budget, allocated_sum AS spent,
       (b.amount - allocated_sum) AS variance,
       (allocated_sum / b.amount) AS burn_pct
ORDER BY burn_pct DESC
```

**3. % do spend alinhado com OKR**

```
MATCH (cap:CeNode {kind:'Capability'})
WITH cap,
     exists((cap)-[:SUPPORTS]->(:CeNode {kind:'Objective'})) AS aligned
MATCH allocations(cap, '2026-Q1') YIELD cost   // engine
WITH aligned, sum(cost) AS spend
RETURN aligned, spend, spend / sum(spend) OVER () AS pct
```

**4. ROI estimado de Claude por squad**

```
MATCH (s:CeNode {kind:'Squad'})-[:CONSUMES]->(t:CeNode {kind:'Tool', flavor:'llm'})
WITH s, t
CALL get_token_cost(s.urn, t.urn, '2026-Q1') YIELD claude_cost
CALL get_squad_delivery(s.urn, '2026-Q1') YIELD prs_merged, features_shipped
RETURN s.name, claude_cost, prs_merged, features_shipped,
       claude_cost / nullIf(features_shipped, 0) AS cost_per_feature
```

---

## 12. Trade-offs do plano `business`

| Decisão | Pró | Contra |
|---|---|---|
| Modelar pessoas no grafo | Ownership rastreável; ROI por time | Carrega PII; risco LGPD |
| `Domain` como camada extra | Taxonomia para portfólios grandes | Premature em MVP; deletar depois |
| `Tool` como nó de primeira classe | Custo de SaaS/LLM unificado | Mais um Kind para manter |
| Engine única para cost+revenue+effort | DRY; auditável; mesmo drill-down | Engine fica complexa; precisa de tipos `Flow` |
| `revenue` como subpacote de `cost` | Reuso máximo; menos boilerplate | Acopla planos; promover quando dor aparecer |
| `Budget` como nó (não property) | Versionável; auditável | Mais nós no grafo |
| Atribuir pessoa a feature por heurística | Cobertura sem precisar de worklog | Erro de atribuição; precisa documentar método |

---

## 13. Como caminhar — entrada na realidade

Sequência recomendada (sem prometer prazos):

1. **`org` mínimo**: Company, BusinessArea, Team, Squad, Person + ingest
   via CODEOWNERS. Resultado: "quem é dono deste código?".
2. **Bridge ownership**: edges `Owns` e `Maintains` ligando `org` ao
   `code`. Habilita: "qual squad é responsável pelo gasto deste service?".
3. **`product` mínimo**: Capability + Feature, ingest via Jira labels.
   Edge `ImplementedBy` (Feature→Service). Habilita: custo por capability.
4. **`Budget`**: ingest manual via CSV. Variance vs allocated cost.
5. **Tool/LLM cost**: ingest tokens do Claude. Atribui via
   `CommunicatesWith` no plano code.
6. **`revenue`**: começa com Stripe (mais simples), MRR por customer.
   Atribuição a feature via uso observado.
7. **`Objective/KeyResult`**: ingest manual ou ProductBoard. Edge
   `Supports`. Habilita relatório de alinhamento estratégico.
8. **Margem e simulações**: combinar tudo no `app/simulate/`.

Cada passo entrega valor isolado. **Não construir os 8 antes de testar
o 1.** O grafo é incremental por design.

---

## 14. O que isto NÃO substitui

- **Não é HRIS**. BambooHR/Workday continuam fonte primária; o grafo é
  espelho parcial e desnormalizado para análise.
- **Não é ERP nem accounting**. Receita reconhecida segundo ASC 606
  continua no sistema contábil; o grafo opera em receita atribuída
  (managerial), não receita reconhecida (statutory).
- **Não é product management tool**. Jira/Linear continuam o backlog; o
  grafo importa o estado para correlacionar com infra/custo.
- **Não substitui FP&A**. Budget vs actuals continua na ferramenta de
  finanças; o grafo enriquece com o "*por quê*" granular que o FP&A
  agregado não tem.

A proposta é ser a **camada de correlação** que liga sistemas que hoje
não conversam — não reimplementá-los.
