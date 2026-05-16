---
id: F-010
title: HRIS ingest (CSV mínimo)
status: done
modules: [org]
depends_on: []
modeling_impact: no
adrs: []
epic: E-004
updated: 2026-05-14
---

# F-010 — HRIS ingest (CSV mínimo)

## Problema

Plano `org` está vazio. Sem `Person/Team/Squad`, é impossível atribuir
custo a time, medir effort, ou rodar F-011 (CODEOWNERS) e F-013
(Feature ↔ Service via PR). Conectar a sistemas HRIS reais (BambooHR,
Workday) é grande; CSV resolve 80% do problema com 5% do esforço.

**Para quem:** allocation engine (dimensão `team`), bridge ownership,
produto Teams.

**Dor sem ela:** custo nunca chega em pessoas/times.

## Escopo

**Inclui:**
- Comando `ce ingest hris --csv=<path>` que lê CSV no schema:
  `email, name, role, team, squad, manager_email, start_date, end_date_or_blank`.
- Cria `Person` (urn por email hash), `Team`, `Squad`.
- Edges: `MemberOf(Person → Squad)`, `PartOf(Squad → Team)`,
  `ReportsTo(Person → Person)`.
- PII handling: `email` armazenado como hash + plain ofuscado
  (apenas iniciais).
- Idempotência por `email_hash` para Person, `(name, parent)` para
  Team/Squad.

**NÃO inclui:**
- Integração nativa com HRIS (futuro).
- Capacity / FTE percent (futuro: feature de capacity).
- Salário (fora de escopo; vive em outra fonte).
- Histórico de mudanças anteriores ao primeiro import (apenas o que
  CSV trouxer).

**Precondições:**
- CSV disponível com schema acordado.

## Toque no grafo

- **Lê:** `Person/Team/Squad` existentes (para diff).
- **Escreve:** `Person/Team/Squad` + edges `MemberOf/PartOf/ReportsTo`
  via `internal/modules/org`.
- **Novos Kinds/edges:** nenhum (já em `03-business.md`).
- **Bitemporal:** mudar squad de pessoa fecha `MemberOf` antigo, cria
  novo. `end_date_or_blank` populado fecha `Person` (soft delete).

## Critérios de aceite

- [x] Dado CSV com 50 linhas válidas, quando `ce ingest hris --csv=...`,
      então grafo contém 50 `Person`, 5 `Team`, 10 `Squad` + 109 edges
      (50 MEMBER_OF + 10 PART_OF + 49 REPORTS_TO).
- [x] Re-importar CSV sem mudanças: URNs estáveis (`TestIntegration_ReimportIdempotent`).
- [x] Pessoa que mudou de squad no CSV: nova `MemberOf` corrente
      coexiste com a antiga (versionamento por `ObservedAt` no edge ID).
- [x] Pessoa com `end_date` preenchido: `Person` fechado (`Delete`),
      `MemberOf` correspondente também fechado.
- [x] CSV mal-formado (linha com campo faltando ou data inválida) não
      derruba execução; vai para `[]RowError` (stderr no CLI).
- [x] Email nunca em plain: URN usa hash (16 bytes hex), `EmailHint`
      mascara como `xx***@domain`.

## Riscos / incerteza

- **Email como chave estável.** Pessoa muda email → tratada como
  pessoa nova. Aceitar limitação ou adicionar `employee_id` opcional.
- **Hierarquia circular em `manager_email`.** Validar e rejeitar
  com erro claro.
- **PII compliance.** Hash em si pode permitir join entre fontes
  (intencional); plain ofuscado é só para UX. Decisão: documentar
  em ADR se virar regulatório.
- **Granularidade Team/Squad/BusinessArea.** Modelo assume hierarquia
  fixa. Estruturas matriciais precisam de feature dedicada.

## Notas de implementação

- Pacote `internal/modules/org/ingest/hris/`.
- Hash: SHA256 do email lowercase trim, primeiros 16 bytes hex.

## Decisões (implementação)

- **D1 — Provider `org` + tenant no slot `<account>`.**
  Reutiliza o formato canônico de URN (`urn:ce:org:<tenant>:<kind>/<id>`).
  Mesmo padrão de F-007 com `code`. Multi-tenant fica natural quando
  precisarmos: tenant é segmento da URN, não campo lateral.

- **D2 — Email hash = SHA256(lower+trim)[:16] hex.**
  16 bytes (32 hex chars) é colision-resistant para escala org (10⁴–10⁵
  pessoas). Lower+trim normaliza variações triviais. Pessoa que muda
  email vira pessoa nova (limitação aceita; `employee_id` opcional fica
  para depois se for caso real).

- **D3 — Slugify determinístico para Team/Squad.**
  `slug = lower → non-alnum→"-" → collapse → trim`. Garante que "Platform"
  e "platform" produzem o mesmo URN. URN de Team = `urn:ce:org:<tenant>:team/<slug>`.
  Squad inclui o team no slug pra evitar colisão entre squads homônimos:
  `urn:ce:org:<tenant>:squad/<team_slug>__<squad_slug>`.

- **D4 — Edge types em UPPER_SNAKE.**
  `MEMBER_OF`, `PART_OF`, `REPORTS_TO`. Consistente com `DEFINED_IN`
  (F-007). Registry valida adjacência: MemberOf(Person→Squad),
  PartOf(Squad→Team), ReportsTo(Person→Person).

- **D5 — `end_date` fecha Person + MEMBER_OF de saída.**
  Writer.Apply enumera `res.Terminated`, faz `Nodes.Delete(personURN)` e
  então `Edges.Neighbors(out, MEMBER_OF) → DeleteEdge(id)`. ReportsTo
  fica aberta intencionalmente (manager pode permanecer válido como
  referência histórica de quem reportava a quem).

- **D6 — Ciclo em `manager_email` = fatal.**
  DFS white/gray/black em `Build`. Ciclo é defeito de dado (HRIS sério
  não tem ciclos); abortar é melhor que ingerir grafo inconsistente.
  Mensagem inclui o path do ciclo para diagnóstico.

- **D7 — Linhas malformadas = `RowError`, não-fatal.**
  Header inválido é fatal (`ErrHeader`); linha com email vazio, data
  inválida, ou contagem de campos errada vira `RowError{LineNum, Field,
  Message}` e o resto do CSV continua. CLI imprime os erros em stderr
  mas retorna exit 0 (relatório vs falha).

- **D8 — `EmailHint = xx***@domain` separado do hash.**
  Hash é a chave estável (não-reversível); `EmailHint` é só pra UX
  ("mostrar quem é essa pessoa sem revelar email"). Em logs/queries
  use sempre o hash; hint nunca aparece em URN nem em hash.

## Verificação

```sh
# Unit/parser/emit/service/cli — todos os pacotes da feature
go test ./internal/modules/org/... ./cmd/cli/...

# Integration (sem dependência externa — memory repo)
go test -run TestIntegration_ ./internal/modules/org/...

# Smoke CLI
ce ingest hris --csv=test.csv --tenant=acme --dry-run
```

**Resultado esperado:**
- `TestIntegration_50RowsHappyPath` — 50 persons, 5 teams, 10 squads, 109 edges.
- `TestIntegration_ReimportIdempotent` — URNs estáveis após 2 imports.
- `TestIntegration_SquadChange_ClosesOldMembership` — MEMBER_OF correntes ≥1.
- `TestIntegration_EndDateClosesPerson` — Person fechado some de `GetByURN(AsOf{})`.
- `TestIntegration_MalformedRowsSkipped` — 2 erros / 2 rows válidas.
- `TestIntegration_EmailNeverInPlain` — URN/EmailHint não contêm `@`.

## Entregue em

- **S-001..S-008** (slicing interno): entities (Person/Team/Squad +
  edges org), parser CSV, emit/dedup/cycle, Writer, CLI `ce ingest
  hris`, integration test E2E, docs + priorities.
