# Plano de Implementação — Ontologia Cross-Language do Code Plane

> Plano de execução para materializar a ontologia descrita em
> `docs/architecture/08-code-ontology.md`. Estende e (em parte) substitui
> o modelo de F-007. Decisões-âncora estão em ADR-006 a ADR-010.
>
> Sem estimativas de tempo — só ordem, dependências e DoD.

---

## Mapa de épicos novos

- **E-007 — Cross-Language Code Ontology** (substrato técnico)
- **E-008 — Governance & Delivery Mapping** (camada de intenção)

Os dois épicos não são puramente sequenciais. **E-007 fases 1–2** desbloqueiam
**E-008** parcialmente; o restante pode rodar em paralelo.

```
F-017 (identidade) ─┬─▶ F-018 (Module) ─┬─▶ F-019 (Call boundary) ──▶ F-020 (Call in-process)
                    │                    └─▶ F-021 (Type + Variable) ─▶ F-022 (Schema) ─▶ F-023 (Framework/License/CVE)
                    │
                    └─▶ F-024 (Domain/Capability) ─┬─▶ F-025 (Epic/UserStory) ─▶ F-026 (Persona)
                                                   └─▶ F-027 (Role) ─▶ F-028 (feature-tag) ─▶ F-029 (ownership bifurcada)
```

---

## E-007 — Cross-Language Code Ontology

### Fase 1 — Migração de identidade (precondição de tudo)

**F-017 — Migração de identidade Service/Endpoint/Function para cross-language**

- Renomear `Function.Package` → `Function.Namespace`.
- Remover `Service.GoModule`; adicionar `Service.Manifest` (`go.mod`,
  `package.json`, etc.) e `Service.ManifestType`.
- Adicionar `Location{File, LineInit, LineEnd, ColInit, ColEnd}` em
  Function/Endpoint como atributo (URN/ContentHash continuam ignorando).
- Discriminador `<symbol>` para closures: `OuterFunc$closure#idx`.
- Script de migração: nós existentes da F-007 reescrevem URN derivando
  Namespace de Package (`Namespace = ModulePath + "/" + Package`).
- ADR-006 governando.

DoD:
- `internal/entity/node/{service,endpoint,function}.go` atualizados.
- Tests em `code_urn_test.go` cobrem cross-language (fixtures Python/TS
  simulados conceitualmente — coletor real é fase posterior).
- Coletor Go (`internal/modules/code/collector/golang`) produz Namespace
  corretamente.
- Migração em produção testada em fixture de F-007.

### Fase 2 — Module como nó

**F-018 — Module como nó com CONTAINS aninhável**

- Novo kind `Module` em `internal/entity/node/module.go`.
- URN: `urn:ce:code:<repo>:module/<service-id>!<namespace>`.
- Aresta `CONTAINS` reusa `internal/entity/edge/contains.go` (já existe
  no infra plane).
- Coletor Go popula: cada package vira Module; ramo `internal/api/users`
  cria 3 Modules aninhados (`internal/api`, `internal/api/users`),
  ligados por `CONTAINS`.
- Migração: nós de F-007 ganham `Module` ancestral.
- `DEFINED_IN` (de F-007) deprecia em favor de `CONTAINS` (Module→Endpoint/Function).

DoD:
- Re-run idempotente do coletor.
- Query `MATCH (m:Module)-[:CONTAINS*]->(f:Function {symbol:'CreateUser'})`
  retorna esperado.

### Fase 3 — Call family

**F-019 — Call boundary nodes**

Subkinds neste escopo: `HttpCall`, `RpcCall`, `EventPublish`,
`EventSubscribe`, `QueueSend`, `QueueReceive`, `DataAccess`, `JobSchedule`.

- Kind `Call` (com discriminador interno) em `internal/entity/node/call.go`.
  Alternativa: 1 arquivo por subkind para manter atributos próprios
  tipados.
- URN: `urn:ce:code:<repo>:call/<kind>!<caller-function-id>#<idx>`.
- Coletor Go detecta:
  - `net/http`, `chi`, `gin`, `echo` clients → `HttpCall`.
  - `database/sql`, `pgx`, `gorm` → `DataAccess`.
  - SDKs SQS/SNS/Kinesis → `EventPublish`/`Subscribe`/`Queue*`.
  - `time.AfterFunc`, cron libs → `JobSchedule`.
- `Args[]` e `Returns[]` populados com `type_ref` (primitivo ou URN de Type).
- Edges: `INVOKES` (Function→Call), `TARGETS` (Call→destino), `USES` (Call→Framework).
- ADR-007 governando.

DoD:
- Re-run idempotente.
- Coletor produz `TARGETS` resolvido quando possível, vazio quando
  dinâmico — sem erro.
- Test fixture: handler que abre conexão DB + chama outro service por
  HTTP gera 1 `DataAccess` + 1 `HttpCall` ligados ao caller.

**F-020 — Call in-process (FunctionCall, MethodCall)**

- Idem F-019 para chamadas internas.
- Filtro: ignora chamadas para `fmt.*`, `log.*`, builtins (`len`, `append`,
  `make`) configurável via `Config.IgnoredCallTargets`.
- `MethodCall.is_dynamic = true` quando dispatch via interface não-resolvível.

DoD:
- Função com 10 calls produz 10 nós + 10 edges INVOKES.
- Filtro de stdlib funciona (não polui grafo).
- Bench: indexar repo de ~50k LoC em < 60s no laptop dev.

### Fase 4 — Type system

**F-021 — Type e Variable como nós; edges entre Types**

- Kinds `Type` (com atributo `kind: struct|class|interface|enum|alias|union`)
  e `Variable`.
- URN: `urn:ce:code:<repo>:type/<service-id>!<namespace>!<symbol>`.
- `Type.Fields[]` e `Type.Methods[]` como metadado estruturado
  (ADR-008).
- Coletor Go: `*ast.StructType`, `*ast.InterfaceType`, `*ast.TypeSpec`
  com aliases.
- Edges: `IMPLEMENTS` (resolução estrutural Go: type satisfaz interface
  se tem todos os métodos), `EXTENDS` (embedding), `ALIASES`.
- `Function.Params`/`Returns` referenciam Type URN via `type_ref`.

DoD:
- Repo Go com 10 structs + 3 interfaces gera 13 Type nodes.
- `MATCH (s:Type {kind:'struct'})-[:IMPLEMENTS]->(i:Type {kind:'interface'})`
  funciona.
- `Variable` para package-level `var`/`const` populado.

**F-022 — Schema como nó (proto/OpenAPI)**

- Kind `Schema` (atributos: `format`, `version`, `source_file`, `Fields`).
- URN: `urn:ce:code:<repo>:schema/<service-id>!<schema-id>` ou `_global`.
- Coletores:
  - `.proto` files → `Schema` por message.
  - `openapi.yaml` files → `Schema` por components.schemas.X.
- Edge `Type -SERIALIZES_AS-> Schema` quando há Type gerado.
- Edge `Schema -IMPORTS-> Schema` (proto imports).
- `Endpoint.Request/Response.type_ref` aceita URN de Schema.

DoD:
- Repo com `service.proto` + Go gerado: tanto `Schema:User` quanto
  `Type:User` existem, ligados por `SERIALIZES_AS`.
- Query "qual contrato esse endpoint serve?" funciona partindo do
  Endpoint.

### Fase 5 — Framework, License, SecurityAdvisory

**F-023 — Framework/License/SecurityAdvisory globais**

- Kinds globais em `urn:ce:code:_global:...`.
- Coletor de `go.sum`/`go.mod` produz `Framework` por dependência.
- Coletor `licensee` ou similar produz `License` SPDX.
- Sync periódico com GitHub Security Advisories API → `SecurityAdvisory`.
- Edges:
  - `Module -DEPENDS_ON-> Framework` (com atributo `version_constraint`).
  - `Framework -LICENSED_UNDER-> License`.
  - `Framework -AFFECTED_BY-> SecurityAdvisory`.
  - `SecurityAdvisory -PATCHED_IN-> Framework` (com `since_version` na
    edge).
  - `Call -USES-> Framework` (cliente library detectado em F-019).

DoD:
- Query CVE → frameworks → modules → owners executa em < 100ms em repo
  típico.
- SBOM gerado a partir do grafo é comparável a `go mod graph` +
  `licensee`.

---

## E-008 — Governance & Delivery Mapping

### Fase 6 — Hierarquia de produto

**F-024 — Domain e Capability como nós (extensão de F-012)**

- Estende F-012 (Capability/Feature CRUD) com endpoints `/v1/domains` e
  `/v1/businessareas`.
- Hierarquia: `Domain -CONTAINS-> Capability -CONTAINS-> Feature`.
- `Feature -CONTAINS-> Feature` (sub-features aninhadas).
- URN: `urn:ce:gov:<company>:{domain,capability,feature}/<short_id>`.
- Validação: criar Feature exige `parent_capability_urn` existente.

DoD:
- CRUD completo via REST.
- Bitemporal: PATCH cria nova versão.
- Test: feature sob capability inexistente retorna 400.

### Fase 7 — Delivery agile

**F-025 — Epic e UserStory como nós**

- Kinds `Epic` e `UserStory`.
- Edges: `Epic -CONTAINS-> UserStory`, `Feature -DELIVERS-> UserStory`.
- `Person -ASSIGNED_TO-> UserStory` (cardinalidade 1).
- Atributos UserStory: `status` (`backlog|in-progress|done`),
  `story_points`, `acceptance_criteria`.
- Atributos Epic: `target_date`, `success_metrics`, `business_goal`.

DoD:
- CRUD via REST.
- Sync opcional com Linear/Jira (interface pluggável, MVP entrega só CRUD).

**F-026 — Persona como nó**

- Kind `Persona`.
- Edge: `UserStory -SERVES-> Persona`.
- **Sem** denormalização em código (ADR-009 — Persona segue caminho
  estrito via UserStory).

DoD:
- CRUD via REST.
- Query "endpoints servindo persona B2B" passa por
  Endpoint←feature_tags→Feature→UserStory→Persona — funciona apenas se
  UserStory estiver populada (diagnóstico esperado).

### Fase 8 — Estrutura organizacional

**F-027 — Role como nó por (track, level)**

- Kind `Role`.
- Atributos: `track` (backend/frontend/mobile/data/qa/pm/sre/...),
  `level` (junior/mid/senior/staff/principal/director), `is_leadership`.
- Edge `Person -HAS_ROLE-> Role` (cardinalidade 1, bitemporal cobre
  promoções).
- Roles podem ter `Role -SENIORITY_ABOVE-> Role` (opcional, MVP pode
  omitir).
- Substitui o esquema atual (F-010) onde role é string em Person.

DoD:
- CRUD via REST.
- Migração: roles textuais existentes ficam como `unknown-<text>` até
  reconciliação manual.

### Fase 9 — Ponte code↔governance

**F-028 — Feature-tag denormalizada em código**

- Adicionar atributo `feature_tags []string` em Service, Module,
  Endpoint, Function, Type, Variable, Call.
- Coletor reconcilia 3 fontes:
  - Anotações inline (`// @feature:home-banner`).
  - Manifest `features.yaml` do repo.
  - PR labels (extensão de F-013).
- Lint job: tag órfã (Feature inexistente) → warning.
- Lint job: Feature sem código tagueado → relatório.
- Index secundário sobre `feature_tags` no Neo4j.

DoD:
- Anotação `// @feature:X` em uma Function aparece em
  `function.feature_tags`.
- Lint executa em CI e gera relatório.
- Query `MATCH (c) WHERE 'home-banner' IN c.feature_tags` < 100ms.

**F-029 — Ownership bifurcada (extensão de F-011 — CODEOWNERS)**

- Service/Module/Endpoint/Feature/Epic → `OWNED_BY` Team.
- Function/Call/Type/Variable → `OWNED_BY` Person (via `git blame`).
- UserStory → `ASSIGNED_TO` Person.
- Heurística do `git blame`: maior contribuidor no range de linhas nos
  últimos 12 meses.
- Lint: nó com `OWNED_BY` apontando para Person/Team em granularidade
  errada → warning.

DoD:
- Reorganização: renomear Team rebumpa só edges `OWNED_BY` afetadas;
  Service/Module permanecem (Bitemporal preserva).
- Pessoa removida: relatório "código órfão" lista Function/Call afetadas.

---

## Princípios transversais (aplicam-se a todas as fases)

1. **Migração antes de incremento.** F-017 (identidade) é precondição
   técnica para qualquer kind novo. Sem ela, novos nós herdam o conceito
   Go-specific.
2. **Idempotência total dos coletores.** Re-run em commit estável produz
   o mesmo grafo, sem efeitos colaterais. Test obrigatório em cada fase.
3. **Bitemporal sempre.** Nenhuma mudança no schema (renome de campo,
   migração de URN, recategorização de edge) destrói histórico — abre
   versão nova.
4. **Lint > Constraint.** Regras semânticas (granularidade de ownership,
   tag órfã, drift entre intenção e código) rodam como auditoria, não
   bloqueiam ingestão. Permite código legado conviver com governança
   imperfeita enquanto se evolui.
5. **Coletor Go é o canário.** Cada feature precisa funcionar primeiro
   no coletor Go existente antes de pensar em multi-linguagem. Coletores
   adicionais (Python, TS, Java, Rust) são fases pós-MVP.
6. **Sem feature flags permanentes.** Migrações destrutivas (ex.: remover
   `Package` em favor de `Namespace`) são feitas com toggle só durante
   transição.

---

## Definition of Done — geral

Aplicável a todo `F-NNN` desta ontologia:

- [ ] Schema do nó/edge em `internal/entity/{node,edge}/` com tests de
      URN e ContentHash.
- [ ] Coletor Go produz o nó/edge corretamente em fixture de repo.
- [ ] Test de integração com Neo4j (Testcontainers) — upsert + query.
- [ ] Bitemporal: re-run sem mudanças não rebumpa versão; com mudança
      relevante, fecha v_n e abre v_{n+1}.
- [ ] Doc do feature `docs/features/F-NNN-*.md` com escopo, decisões e
      critérios de aceite.
- [ ] ADR referenciado no frontmatter se a decisão é arquiteturalmente
      relevante.
- [ ] Lint job (quando aplicável) integrado ao CI.

---

## Próximo passo concreto

Começar por **F-017 (migração de identidade)**. Sem ela, qualquer kind
novo nasce com o conceito `Package` herdado de Go e o coletor não tem
contrato cross-language para apoiar.

Tamanho esperado: pequeno-médio — toca 3 arquivos em `internal/entity/node/`,
1 arquivo em `internal/modules/code/collector/golang/`, e adiciona
migração de URN para nós existentes. ~200 LoC + tests.

Plano de ataque de F-017:
1. Add `Namespace` ao lado de `Package` em `Function`/`Endpoint`
   (não-breaking, populando os dois).
2. Atualizar `NewFunctionURN`/`NewEndpointURN` para receber Namespace
   (mantendo overload com Package marcado deprecated).
3. Coletor Go popula ambos.
4. Adicionar Location estruturada.
5. Script de migração: converte URNs existentes derivando Namespace de
   `<module-path>/<package>`.
6. Remover `Package`/`GoModule` na versão seguinte (após migração de
   produção estabilizar).
