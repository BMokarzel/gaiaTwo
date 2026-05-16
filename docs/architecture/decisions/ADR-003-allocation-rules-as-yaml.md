---
id: ADR-003
title: Allocation rules vivem como YAML versionado, não como Kind no grafo
status: accepted
date: 2026-05-14
related_features: [F-006]
supersedes: []
superseded_by: []
---

# ADR-003 — Allocation rules como YAML

## Contexto

F-006 (Shared infra cost) precisa de **regras** para ratear linhas CUR
que não puderam ser atribuídas diretamente a uma URN (taxa, suporte,
data transfer, savings plans). Exemplos:

- "AWS Support rateia proporcional ao alocado por team."
- "Data transfer cross-region atribuído aos recursos da região de
  destino."
- "Savings Plans credit subtrai do account-owner proporcionalmente."

Surge uma escolha de modelagem: **onde essas regras vivem?**

ADR-002 estabelece bitemporalidade como invariante do **grafo**.
Regras de allocation poderiam ser promovidas a Kind no grafo
(`SharedCostRule`) com `valid_from/valid_to`, audit nativo e queries
históricas via Cypher. Ou viver como **configuração externa** —
YAML versionado em git, lido por execução do allocator.

A questão não é trivial: regras de cost mudam ao longo do tempo
(política muda, novo time é criado, contrato muda), e cada linha
em `fct_cost_by_urn` precisa ser rastreável até a regra que a
originou.

## Decisão

**Regras de allocation vivem como YAML versionado em git**, **não**
como Kind no grafo. Concretamente:

1. **Storage:** arquivos em `config/shared_rules/*.yaml`, lidos por
   execução do allocator. Cada regra tem `id` (slug estável),
   `version` (int monotônico), `valid_from`/`valid_to` opcionais.

2. **Audit trail:** cada linha em `fct_cost_by_urn` com
   `allocation_type='shared'` carrega `rule_id` e `rule_version`. O
   conteúdo completo da regra em qualquer ponto do tempo é recuperável
   via `git log -- config/shared_rules/<id>.yaml`.

3. **Bitemporal por proxy:** `valid_from/valid_to` na regra YAML é
   honrado pelo allocator (regra fora do intervalo é ignorada).
   "Como era a regra X em 2026-01-15" responde-se com
   `git show <commit>:config/shared_rules/X.yaml`.

4. **CRUD:** alteração de regra = PR. Aprovação de FinOps embutida no
   workflow de revisão de código. Sem CLI/API de mutação no MVP.

5. **Sem novo Kind:** `01-modeling.md` permanece intocado. Nada novo
   na adjacency matrix do grafo.

## Alternativas consideradas

### A — `SharedCostRule` como Kind do plano `cost`
Descartada para o MVP. Pró: audit bitemporal nativo, queries Cypher
históricas, consistência com ADR-002. Contra: custa orçamento (URN,
edges `APPLIES_TO`, CRUD via CLI/API, validação de schema) para
≤30 regras na vida real. Cost/benefit ruim no MVP. **Reabrir** se
aparecer caso de uso de UI de regras, dependências entre regras
modeladas como subgrafo, ou multi-tenant onde regras viajam entre
ambientes via grafo.

### B — Regras em ClickHouse (`dim_shared_rule`)
Descartada. Ganha pouco vs YAML (sem audit git-blame; CRUD precisa
escrever DDL ou API), perde a propriedade "infra-as-code" que faz
PR review natural.

### C — Híbrido (regras no grafo + cache YAML)
Descartada. Complexidade desproporcional para um conceito que é
configuração operacional, não observação do mundo.

## Consequências

### Positivas
- **Zero overhead** no grafo: `01-modeling.md` não muda, sem novos
  Kinds, sem novas edges.
- **Audit trail completo** via git (commit, blame, diff, PR review).
- **PR review = aprovação FinOps**: regra mudada via PR força
  conversa antes de afetar billing.
- **Reversibilidade**: promover para Kind depois é mecânico — `rule_id`
  YAML vira segmento de URN.

### Negativas / custo
- **Queries históricas precisam de git, não Cypher**. Mitigação:
  toolagem de inspeção (`ce rules show <id> --at=<date>`) pode ler
  git internamente se virar dor real.
- **Validação de schema YAML é runtime**, não vinculada ao registry
  de edges. Mitigação: parser de regras tem testes próprios e
  rejeição clara em load.
- **Não vive no grafo** → produtos de UI/grafo não conseguem renderizar
  regras como nós. Aceito por hora — FinOps editor seria feature
  separada se justificar.

### Quando reabrir
- Quando o número de regras passar de ~50 (ergonomia de YAML decai).
- Quando regras passarem a depender entre si (uma regra modifica o
  input de outra) — grafo de dependências é melhor modelado como
  grafo, não como ordem implícita em arquivos.
- Quando produto exigir UI de edição/visualização de regras (CRUD
  via grafo + projeção em UI é caminho natural).
- Quando "qual regra valia em data X" virar consulta operacional
  (vs auditoria pontual) — pull do git fica caro.

## Implementação atual

- Pacote: `internal/modules/cost/shared/`.
- Config: `config/shared_rules/*.yaml`.
- Audit: colunas `allocation_type`, `rule_id`, `rule_version` em
  `fct_cost_by_urn` (schema migration aditiva — ver F-006 S-001).
