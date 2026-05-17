# Pendências do backlog

> Lista única e priorizada do que ficou aberto. Cada item registra
> origem, escopo, dor/risco de adiar e gatilho para promover a feature.
>
> Atualizado: 2026-05-17

## Visão geral

- **Sem features `refined`/`in_progress`.** Roadmap só tem slices
  dentro de features `done com backlog parcial` ou follow-ups
  técnicos.
- **Backlogs vivos** (slices dentro de features done): F-017, F-019,
  F-022, F-023, F-025.
- **Entity-layer só**: F-026 (Persona) — sem coletor, sem dor.
- **Follow-ups técnicos**: migração `entity→core`, OpenAPI sync,
  docs de módulo, controllers code/infra, BusinessArea, etc.

Para o painel de estado por feature/épico, ver
[refinement-status.md](refinement-status.md).

---

## 1. Backlogs vivos por feature

Slices que vivem dentro de uma feature já `done`. Promover a story
nova quando aparecer dor concreta (usuário real, integração nova,
ADR que mude o modelo).

### F-017 — Cross-language identity

- **Origem:** ADR-006/F-017. Go coletor pronto; Python/TS faltam.
- **Pendente:**
  - Coletor Python (`pyproject.toml`, `setup.py`) — emite Service +
    Module com `Namespace` = nome do pacote.
  - Coletor TypeScript/Node (`package.json` + `tsconfig`) — emite
    Service + Module com `Namespace` = `name` do package.json.
  - Normalização de `ManifestType` ao indexar (já existe enum, falta
    coletores populá-lo).
- **Dor adiada:** usuários atuais são Go-only. Não há demanda.
- **Gatilho:** primeiro repo Python ou TS no escopo de algum tenant.

### F-019 — Call boundary

- **Origem:** ADR-007. Inbound + outbound subkinds entregues
  (HTTP/gRPC/SQS/SNS/Kinesis/cron + FunctionCall).
- **Pendente:** `EventSubscribe` (consumer Kafka/Pub-Sub) — requer
  type-resolve (depende de F-021 IMPLEMENTS).
- **Gatilho:** primeira app event-driven entrar no grafo.

### F-021 — Type/Variable

- **Origem:** F-021. `Type`, `Variable`, `EXTENDS`, `ALIASES` prontos.
- **Pendente:** `IMPLEMENTS` (Type→Type, struct↔interface em Go;
  `implements` explícito em TS/Java). Requer resolução estrutural.
- **Gatilho:** quando F-019 `EventSubscribe` ou type-aware allocation
  pedir.

### F-022 — Schema

- **Origem:** F-022. OpenAPI adapter pronto.
- **Pendente:**
  - JSON Schema standalone (sem OpenAPI ao redor).
  - Avro (Confluent Schema Registry).
  - GraphQL SDL.
  - `oneOf`/`allOf`/`anyOf` em FieldSlots — hoje achatado.
- **Gatilho:** primeiro contrato fora de OpenAPI/Proto.

### F-023 — Framework / License / SecurityAdvisory

- **Origem:** F-023. Coletores Go (go.mod) + npm prontos. Edges
  `LICENSED_UNDER`, `AFFECTED_BY`, `PATCHED_IN` modelados.
- **Pendente:**
  - Coletor `pyproject.toml` / `Cargo.toml`.
  - Lookup SPDX (mapping licença → ID canônico).
  - Sync OSV / GHSA (popula `SecurityAdvisory` + edges).
- **Gatilho:** primeira dor de compliance/security tracking.

### F-025 — Epic / UserStory

- **Origem:** F-025. Entities + edges prontos, CRUD via `ce gov`.
- **Pendente:** integrações ALM (Jira / Linear / GitHub Issues) que
  ingestam epic/userstory automaticamente em vez de CRUD manual.
- **Gatilho:** primeiro produto/feature owner pedir
  "quanto custou entregar epic X?".

---

## 2. Features sem coletor (entity-layer only)

### F-026 — Persona

- **Origem:** F-026 / ADR-009.
- **Estado:** entity `Persona` + edge `Serves(UserStory→Persona)`
  existem; sem coletor, sem CLI, sem CRUD.
- **Decisão atual:** manter em backlog. Sem dor concreta — ADR-009
  proíbe denormalização em código (Persona só chega via UserStory).
- **Gatilho:** primeiro dashboard pedir agregação por Persona.

---

## 3. Follow-up F-013 (Feature→Service via PR)

Backlog explícito documentado em `docs/features/F-013-*.md`:

- **Fila de pendentes** para Features ausentes (label `feature:<urn>`
  aponta para URN inexistente — hoje 400). Replay automático quando a
  URN aparece.
- **Hidratação de `files[]`** via GitHub REST API. Hoje o payload
  assume que um proxy/CI enriquece o evento com a lista de arquivos
  (o evento nativo `pull_request` não traz). Implementar fetch lazy
  com token de instalação.
- **Outros providers** (GitLab MR, Bitbucket PR) — feature dedicada
  futura, não slice de F-013.
- **Decay de confidence** ao longo do tempo (edge antiga perde força
  se nenhum PR recente reforça).

**Gatilho:** primeiro consumer real do endpoint
`POST /v1/webhooks/github` em produção.

---

## 4. Follow-ups técnicos / dívida

### 4.1 Migração `internal/entity/*` → `internal/core/*`

- **Origem:** `docs/architecture/04-modular.md §9`, passos 1-3. F-016
  executou passos 4-6 (controllers por módulo, httpserver, depguard).
- **Escopo:**
  - `internal/entity/node/urn*.go` → `internal/core/urn/`
  - `internal/entity/node/{meta,base,source,lineage,confidence}` →
    `internal/core/bitemporal/`
  - `internal/entity/node/kind.go` → `internal/core/kind/`
  - `internal/entity/edge/{edge.go,registry.go}` → `internal/core/edge/`
  - `internal/repository/repository.go` → `internal/core/repository/`
  - Structs concretos de `entity/node` → `modules/<plane>/node/`
  - Imports em massa.
- **Risco de adiar:** baixo. `depguard` cobre fronteiras críticas;
  imports atuais são longos mas estáveis. Cada novo Kind concreto em
  `entity/node` adiciona ~5 min de migração futura.
- **Gatilho:** transformar em F-NNN quando alguém precisar evoluir
  `core/` (novo Kind transversal, novo tipo de URN).

### 4.2 OpenAPI v1 (sync com shape RFC 7807)

- **Origem:** F-016 trocou o shape de erro para
  `application/problem+json` (`docs/api/v1/errors.md`).
- **Ação:** auditar `docs/api/v1/openapi.yaml`,
  substituir `Error/ErrorBody` por `Problem`
  (type/title/status/code/detail + extras), atualizar todos os
  `responses: 4xx/5xx`, enumerar `code`s namespaced.
- **Gatilho:** antes do primeiro consumer externo gerar client a
  partir do openapi.

### 4.3 Docs de módulo faltantes

Template em `docs/modules/infra.md`. Faltam:

- `docs/modules/cost.md`
- `docs/modules/code.md`
- `docs/modules/org.md`
- `docs/modules/gov.md` (renomear `product.md` planejado)
- `docs/modules/bridge.md` (incluir `github_webhook` agora que F-013
  fechou)

**Gatilho:** quando o módulo ganhar a próxima feature `in_progress` —
evita doc preditivo que envelhece.

### 4.4 Controllers de `code` e `infra`

- **Origem:** F-016 deixou `modules/code` e `modules/infra` com
  `port.go` + `service/` + `errs.go` + `types.go`, mas sem
  `controller/` próprio (S-006/S-007 absorvidos por S-008 — Kinds
  acessíveis via `app/graph` e `app/search`).
- **Gatilho:** demanda por endpoint REST específico
  (`/v1/code/services/{urn}`, `/v1/infra/computes/{urn}` com lógica
  acima do grafo cru). Replicar playbook de `modules/org/controller/`.

### 4.5 `BusinessArea` collector

- **Origem:** F-015 recortou o critério "GET /v1/business-areas com
  `team_count`/`person_count`" do MVP. Entity `BusinessArea` existe
  (E-008/F-024), mas:
  - HRIS (F-010) não popula BusinessArea — só Team/Squad/Person.
  - Sem endpoint `/v1/business-areas/{urn}/teams` agregador.
- **Gatilho:** consumer real pedir agrupamento acima de `Team`.

---

## 5. Decisões diferidas (não bloqueiam)

- **Manifesto `costengine.yaml`** no repo de cada Service. Mencionado
  em F-009/F-011/F-013 como destino comum. Vale ADR única em vez de
  re-decidir por feature.
- **PII expanded view** (F-015): endpoint que devolve email pleno
  exige ADR sobre política de privacidade. Adiar até primeiro
  consumer real pedir.
- **GraphQL vs REST** (F-014): manter REST até primeiro consumer
  pedir o contrário.
- **Shape `application/problem+json`** já é padrão da API v1 desde
  F-016 — não há flag de compat hoje. Eventuais clients existentes
  precisam adaptar.

---

## 6. Próximas ações sugeridas (sem fila firme)

Como não há feature `refined`, qualquer próxima sessão deve começar
por **escolher onde investir**. Sugestões (não-ordenadas):

1. **Promover backlog vivo a F-NNN** apenas se dor concreta apareceu.
   Ex.: usuário Python real → F-017 Python collector.
2. **OpenAPI sync (§4.2)** — barato, destrava qualquer consumer
   externo futuro. ~1-2h.
3. **Docs de módulo (§4.3)** — clonagem mecânica do template; baixo
   risco, alto valor explicativo. Começar por `bridge.md` (acabou de
   ganhar `github_webhook`).
4. **ADR sobre `costengine.yaml`** (§5) — destrava 3 features futuras.
5. **Migração `entity→core` (§4.1)** — só promover quando o custo de
   adiar passar do custo de migrar (~hoje ainda vale adiar).

Para retomar: ler este doc + `refinement-status.md`, escolher um
item, criar branch/PR e atualizar ambos os docs ao fechar.
