# Pendências do backlog

> Itens que não são features completas mas precisam ser resolvidos antes
> de cair no caminho crítico das features que dependem deles.
>
> Atualizado: 2026-05-16

## Migração `internal/entity/*` → `internal/core/*`

- **Origem:** passos 1-3 do `docs/architecture/04-modular.md §9`.
  F-016 executou passos 4-6 (controllers por módulo, httpserver
  compartilhado, depguard, bridge), mas a base de domínio (`entity/node`,
  `entity/edge`, sentinelas de `repository`) segue em `internal/entity`
  e `internal/repository` por dependência transversal.
- **Escopo:**
  - `internal/entity/node/urn*.go` → `internal/core/urn/`
  - `internal/entity/node/{meta,base,source,lineage,confidence}` →
    `internal/core/bitemporal/`
  - `internal/entity/node/kind.go` → `internal/core/kind/`
  - `internal/entity/edge/{edge.go,registry.go}` → `internal/core/edge/`
  - `internal/repository/repository.go` → `internal/core/repository/`
  - Structs concretos de `entity/node` → `modules/infra/node/`,
    `modules/code/node/`, etc.
  - Imports em massa.
- **Quando promover:** transformar em F-NNN quando alguém precisar
  evoluir `core/` (ex.: novo Kind transversal, novo tipo de URN). Hoje
  não há demanda — `entity/` funciona, `depguard` cobre as fronteiras
  que importam.
- **Risco de adiar:** baixo. Os imports atuais (`costEngine/internal/entity`)
  são longos mas estáveis; cada novo Kind concreto que nasce em
  `entity/node` aumenta o trabalho mecânico futuro em ~5 min.

## Atualizar `docs/api/v1/openapi.yaml` para shape RFC 7807

- **Origem:** F-016 trocou o shape de erro da API para
  `application/problem+json` (catálogo em `docs/api/v1/errors.md`),
  mas o `openapi.yaml` possivelmente ainda descreve o shape antigo
  (`{error: {code, message}}`).
- **Ação:** auditar `components/schemas` (Error/ErrorBody), substituir
  por `Problem` (type/title/status/code/detail + extras), atualizar
  todos os `responses: 4xx/5xx` que referenciam o schema antigo,
  enumerar `code`s namespaced como `enum` quando fizer sentido.
- **Quando resolver:** antes do primeiro consumer externo da API
  consumir o openapi para gerar cliente.

## Módulos sem doc próprio

`docs/modules/infra.md` existe como referência. Faltam:
- `cost.md`
- `code.md`
- `org.md`
- `product.md` (vai nascer com F-012)
- `bridge.md`
- `docs.md`

**Ação:** clonar template de `infra.md` por módulo.
**Quando resolver:** quando o módulo ganhar primeira feature
`in_progress` (não antes — evita doc preditivo que envelhece).

`org`, `code`, `cost`, `bridge` já têm features `done` — vale criar o
doc desses antes do próximo refinamento que toque o módulo.

## Follow-up: `BusinessArea` (recorte de F-015)

- **Origem:** F-015 entregou hierarquia org a partir de `Team`. O
  critério "`GET /v1/business-areas` com `team_count`/`person_count`"
  foi tirado do MVP porque `BusinessArea` não existe no modelo (F-010
  só persistiu `Team/Squad/Person`). Promover exige:
  1. Nova entrada em `node.Kind` (`BusinessArea`) + registry de
     adjacência (`BusinessArea -[CONTAINS]→ Team`).
  2. Mapper n4j (label + propriedades) sob a mesma trilha bitemporal
     dos demais Kinds org.
  3. Ingest: estender HRIS (F-010) ou criar fonte dedicada.
  4. Endpoints `GET /v1/business-areas` + `GET /v1/business-areas/{urn}/teams`.
  5. Atualizar OpenAPI + integration test.
- **Quando promover:** quando algum consumer real pedir agrupamento
  acima de `Team`. Hoje não há demanda — não inventar.

## Follow-up: controllers de `code` e `infra`

- **Origem:** F-016 deixou `modules/code` e `modules/infra` com
  `port.go` + `service/` + `errs.go` + `types.go`, mas sem
  `controller/` próprio (S-006/S-007 absorvidos por S-008 — Kinds
  expostos via `app/graph` e `app/search`).
- **Quando promover:** quando aparecer demanda por endpoint REST
  específico desses domínios (`/v1/code/services/{urn}`,
  `/v1/infra/computes/{urn}` com lógica acima do grafo cru). Aí
  replicar o playbook de `modules/org/controller/`.

## Decisões diferidas (não bloqueiam, mas valem registrar)

- **PII expanded view** (F-015): endpoint para ver email pleno requer
  ADR sobre política de privacidade. Adiar até primeiro consumer real
  pedir.
- **Manifesto `costengine.yaml`** no repo de cada Service: aparece em
  F-009, F-011, F-013 como destino. Vale ADR única em vez de
  re-decidir por feature.
- **GraphQL vs REST** (F-014): manter REST até primeiro consumer pedir
  o contrário.
- **Shape `application/problem+json`** já é o padrão da API v1 desde
  F-016 (ver `docs/api/v1/errors.md`). Eventuais clients existentes
  precisam adaptar — não há flag de compatibilidade hoje.

## Stories pendentes

Features `refined` que ainda não foram fatiadas (rodar prompt 03 sob
demanda quando entrarem na fila de trabalho):

- F-008 OpenAPI ingest
- F-012 Capability/Feature CRUD
- F-013 Feature → Service link via PR (depende de F-012)

## Próxima execução sugerida

1. Definir qual das 3 features refined entra primeiro (F-008, F-012 ou
   F-013).
2. Rodar prompt 03 sobre a feature escolhida.
3. Em paralelo: limpar `internal/controller/` (trivial).
