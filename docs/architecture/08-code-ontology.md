# Ontologia do Code Plane e Governance Plane

> Modelo de domínio do **code plane** (descoberto a partir de código-fonte) e do
> **governance plane** (intenção organizacional e de produto). Define os nós,
> arestas, regras de identidade e como esses dois planos se atravessam.
>
> Complementa `01-modeling.md` (core ontology) e estende o que F-007 começou
> (`Service`/`Endpoint`/`Function` em Go). O objetivo aqui é uma ontologia que
> sirva para **qualquer linguagem** e habilite simulação, governança fina e
> análise de impacto a partir do grafo.

---

## 1. Princípios

1. **Identidade é semântica, nunca posicional.** Mover uma função de
   `users.go` para `users_handler.go` mantém o mesmo nó. Location (file,
   line, col) é **atributo**, jamais entra no `URN` nem no `ContentHash`.
   Sem isso, qualquer reformatação destrói a linha temporal bitemporal.

2. **Endereço cross-language via `Namespace` + `Symbol` flat.** Em vez de
   `Package` (conceito Go), todo símbolo tem dois campos: um caminho lógico
   canônico (`Namespace`) e um nome final (`Symbol`, com convenção
   `Type.Method` quando for método). Coletor de cada linguagem mapeia para
   esse contrato.

3. **Promoção atributo→nó segue 4 condições (todas necessárias):**
   1. Identidade própria que transcende quem referencia.
   2. Atributos úteis que se quer consultar.
   3. É hub: recebe ou origina arestas para múltiplos nós distintos.
   4. O grafo é atravessado **através** dele, não só filtrado **por** ele.

   Falha em qualquer uma → atributo.

4. **Granularidade fina destrava governança.** Mapear feature a *endpoint*
   é grosso demais (endpoints servem múltiplas features). A unidade mínima
   de mapeamento é a *Call* — a invocação concreta que entrega um banner,
   um botão, uma persistência. Por isso **toda invocação é nó**.

5. **Dois eixos de ownership independentes.** Quem mantém código
   (`Person`/`Team`) e quem é dono de negócio da Feature (`Team`/`Person`
   diferente) podem divergir. O grafo expressa as duas dimensões sem
   colapsar.

6. **Tag em código é denormalização aceita.** Edges `Code -IMPLEMENTS-> Feature`
   explodiriam em milhões. Tags `feature_tags []string` no nó cumprem o mesmo
   papel com índice secundário. `Feature` continua nó (governance plane); a
   ligação é via lookup, não traversal.

---

## 2. Code Plane

### 2.1 Mapa dos nós

```
Service ──CONTAINS──▶ Module ──CONTAINS──▶ Endpoint
                          │                Function ──INVOKES──▶ Call ──TARGETS──▶ Function | Endpoint | Topic | Queue | Persistence | ExternalService
                          │                Type ──IMPLEMENTS──▶ Type
                          │                Variable
                          │                Schema
                          │
                          ├── DEPENDS_ON ──▶ Module    (intra/inter-service)
                          └── DEPENDS_ON ──▶ Framework
```

| Kind | Escopo | URN |
|------|--------|-----|
| `Service` | raiz de manifest (`go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `pom.xml`) | `urn:ce:code:<repo>:service/<module-path>` |
| `Module` | namespace lógico dentro do Service (package Go, pacote Python, submódulo TS) | `urn:ce:code:<repo>:module/<service-id>!<namespace>` |
| `Endpoint` | handler HTTP/gRPC/RPC registrado | `urn:ce:code:<repo>:endpoint/<service-id>!<METHOD>:<route>` |
| `Function` | função/método declarado | `urn:ce:code:<repo>:function/<service-id>!<namespace>!<symbol>` |
| `Type` | struct, class, interface, enum, alias, union | `urn:ce:code:<repo>:type/<service-id>!<namespace>!<symbol>` |
| `Variable` | constante ou variável de pacote | `urn:ce:code:<repo>:variable/<service-id>!<namespace>!<symbol>` |
| `Schema` | contrato externo (proto, OpenAPI, JSON Schema, Avro, GraphQL SDL) | `urn:ce:code:<repo>:schema/<service-id>!<schema-id>` (local) ou `urn:ce:code:_global:schema/<id>` (compartilhado) |
| `FunctionCall` / `MethodCall` | invocação in-process | `urn:ce:code:<repo>:call/<kind>!<caller-function-id>#<idx>` |
| `HttpCall` / `RpcCall` | invocação cross-process | idem |
| `EventPublish` / `EventSubscribe` | pub/sub | idem |
| `QueueSend` / `QueueReceive` | filas (SQS, RabbitMQ) | idem |
| `DataAccess` | leitura/escrita em store | idem |
| `JobSchedule` | trigger agendado (cron, scheduler) | idem |
| `Framework` | biblioteca/runtime externo (chi, express, Django, Spring) | `urn:ce:code:_global:framework/<ecosystem>!<name>@<version>` |
| `License` | SPDX-id (MIT, Apache-2.0, GPL-3.0) | `urn:ce:code:_global:license/<spdx-id>` |
| `SecurityAdvisory` | CVE ou advisory público | `urn:ce:code:_global:advisory/<id>` |

### 2.2 Identidade da Call

Toda call é nó, identificada por `caller-function-id + ordinal AST`:

```
caller.func: usecase.CreateUser
calls dentro (em ordem de aparição no AST):
  #1 → http.Post(...)        → HttpCall    call/http!.../usecase.CreateUser#1
  #2 → db.Exec(...)           → DataAccess  call/dataaccess!...#2
  #3 → events.Publish(...)    → EventPublish call/eventpublish!...#3
  #4 → log.Info(...)          → FunctionCall call/function!...#4
```

`#idx` sobrevive a refator de linha. Adicionar uma call **antes** rebumpa os
idx das posteriores — isso é mudança real de estrutura, refletida no grafo
como expected.

### 2.3 Metadados estruturados (não-edges) em Function/Endpoint/Call/Type

Param, return, field, method, body de request/response **não são nós nem
edges**. São metadados estruturados embutidos. Cada slot referencia um
tipo via `TypeRef` (string discriminada):

```jsonc
// Function
{
  "params": [
    { "name": "ctx",    "position": 0, "type_ref": "context.Context",                       "optional": false },
    { "name": "userID", "position": 1, "type_ref": "string",                                "optional": false },
    { "name": "user",   "position": 2, "type_ref": "urn:ce:code:repo:type/.../api!User",    "optional": false }
  ],
  "returns": [
    { "name": "result", "position": 0, "type_ref": "urn:ce:code:repo:type/.../api!User" },
    { "name": "err",    "position": 1, "type_ref": "error" }
  ]
}
```

Discriminação de `type_ref`:
- Começa com `urn:` → resolve nó (`Type` ou `Schema`).
- Caso contrário → primitivo/built-in (não gera nó: `int`, `string`, `error`, `*http.Response`).

Mesmo padrão para:
- `Type.Fields[]` (composição) e `Type.Methods[]` (referência para `Function`).
- `Endpoint.Request` / `Endpoint.Response[status]` (single `BodyDef` com `type_ref`).
- `Call.Args[]` / `Call.Returns[]` — com `source` adicional (`literal:'...'`, `variable:name`, `return-of:<call-urn>`) para simulação.

### 2.4 Arestas do code plane

| Aresta | Origem | Destino | Quando emitir |
|--------|--------|---------|---------------|
| `CONTAINS` | Service / Module | Module / Endpoint / Function / Type / Variable | Coletor escreve hierarquia |
| `DEPENDS_ON` | Module | Module / Framework | Análise de imports |
| `HANDLED_BY` | Endpoint | Function | Handler resolvido do AST |
| `USES_MIDDLEWARE` | Endpoint | Function / Framework | Chain de middlewares |
| `INVOKES` | Function | Call | Toda call site declarada na função |
| `TARGETS` | Call | Function / Endpoint / Topic / Queue / Persistence / ExternalService | Alvo resolvido (pode estar vazio) |
| `USES` | Call | Framework | Biblioteca cliente usada (ex.: `net/http`, `pgx`) |
| `IMPLEMENTS` | Type | Type | Satisfação de interface (estrutural ou declarada) |
| `EXTENDS` | Type | Type | Herança / embedding |
| `ALIASES` | Type | Type | `type X = Y` semântico |
| `SERIALIZES_AS` | Type | Schema | Tipo gerado a partir de proto / mapeado para OpenAPI |
| `IMPORTS` | Schema | Schema | `import "other.proto"` |
| `EXTENDS` (schema) | Schema | Schema | JSON Schema `allOf`, OpenAPI inheritance |
| `LICENSED_UNDER` | Framework / Service | License | Catálogo SBOM |
| `AFFECTED_BY` | Framework | SecurityAdvisory | Vulnerabilidade conhecida |
| `PATCHED_IN` | SecurityAdvisory | Framework | Aresta com atributo `since_version` |

### 2.5 Por que `Call` é família, não kind único

Cada subkind de Call carrega atributos próprios que importam para *queries
específicas*. Tratar tudo como `Call` genérico com `kind: string` força
schema dinâmico (property bag para cada subkind). Família com kinds distintos
mantém schema fechado e legível:

| Subkind | Atributos próprios | TARGETS |
|---------|---------------------|---------|
| `FunctionCall` | — | `Function` |
| `MethodCall` | `interface_qualifier`, `is_dynamic` | `Function` (resolvido) ou nada |
| `HttpCall` | `method`, `url_template`, `timeout`, `retry_policy`, `is_dynamic_url` | `Endpoint` ou `ExternalService` |
| `RpcCall` | `service`, `method`, `proto` | `Endpoint` (gRPC server) |
| `EventPublish` / `EventSubscribe` | `topic`, `consumer_group`, `payload_schema_ref` | `Topic` (messaging plane) |
| `QueueSend` / `QueueReceive` | `queue_name`, `visibility_timeout` | `Queue` |
| `DataAccess` | `operation` (read/write), `table_or_collection`, `dialect`, `query_template` | `Type` (linha lida/escrita) + `Persistence` (infra plane) |
| `JobSchedule` | `schedule` (cron), `payload_template` | `Function` |

---

## 3. Governance Plane

Eixos ortogonais, cada um respondendo a uma pergunta organizacional
diferente.

### 3.1 Eixo organizacional (ownership real)

```
Company ──CONTAINS──▶ BusinessArea ──CONTAINS──▶ Team ──CONTAINS──▶ Person ──HAS_ROLE──▶ Role
                                                            ▲
                                                            │ LED_BY (opcional)
                                                            │
                                                          Person
```

`Role` é nó por **combinação track+level**: `backend-junior`,
`backend-senior`, `frontend-tech-lead`, `mobile-staff`, `qa-senior`, `pm`.
Vários `Person` apontam para o mesmo `Role`.

### 3.2 Eixo product / business-architecture (categorização)

```
Domain ──CONTAINS──▶ Capability ──CONTAINS──▶ Feature
                                                  │
                                                  └── CONTAINS ──▶ Feature (sub-features, aninhamento via edge)
```

**Não-ownership.** Domain e Capability são puramente categorização — uma
Capability pode reunir Features de Teams diferentes (caso real:
`Authentication` mistura web, mobile e identity teams).

### 3.3 Eixo delivery (planejamento ágil)

```
Epic ──CONTAINS──▶ UserStory ──SERVES──▶ Persona
                       ▲
                       │ DELIVERS
                       │
                    Feature
```

`Epic` agrega `UserStory`s (relação 1:N). Uma Feature **entrega** múltiplas
UserStories. Cada UserStory **serve** um `Persona`.

### 3.4 Eixo audiência

`Persona` representa o "para quem" — B2B, aposentado, premium, free-tier,
cliente novo, recorrente. Ancorado **apenas** via `UserStory -SERVES->`.
Código legado sem UserStory **não** fica mapeado a Persona — diagnóstico
incentiva criar a Story.

### 3.5 URN do governance plane

`urn:ce:gov:<company-id>:<kind>/<id>`. O `<company-id>` é o tenant.
Provider `gov` separa do code plane (`code`) e infra plane (`aws`, `gcp`).

---

## 4. Cross-plane: Ownership e Feature-Tag

### 4.1 Ownership bifurcada por granularidade

| Nó do code plane | `OWNED_BY` aponta para |
|------------------|------------------------|
| `Service`, `Module`, `Endpoint` | `Team` |
| `Feature`, `Epic` (governance) | `Team` |
| `Function`, `Call`, `Type`, `Variable` | `Person` |
| `UserStory` | `Person` (via `ASSIGNED_TO`) |
| `Framework`, `License`, `SecurityAdvisory` | — (entidades externas, sem owner organizacional) |

**Regra emergente:** quanto mais **intenção** (negócio, contrato, agregado),
`-> Team`. Quanto mais **execução** (uma linha, uma call, um tipo concreto),
`-> Person`. Cutoff fica em `Module ⇄ Function`.

### 4.2 Feature-tag denormalizada

`Feature` é nó (no governance plane), com hierarquia própria, owner, status.
Mas a **ligação código→feature** é via atributo:

```jsonc
{
  "kind": "HttpCall",
  "id":   "urn:ce:code:repo:call/http!internal/api!handler.GetHome#3",
  "feature_tags": ["home-banner", "promo-strip"],
  ...
}
```

Cada tag = `short_id` canônico de um `Feature` (kebab-case, único por
company). Coletor valida que tag aponta para `Feature` existente (lint de
governança, não constraint hard).

**Query típica:** "todo código que implementa `home-banner`":

```cypher
MATCH (c) WHERE 'home-banner' IN c.feature_tags
RETURN c.id, c.kind, c.location
```

Equivalente em ClickHouse: `WHERE has(feature_tags, 'home-banner')`.

Resolução para o `Feature` completo (com owner, capability, etc.):

```cypher
MATCH (c) WHERE 'home-banner' IN c.feature_tags
MATCH (f:Feature {short_id: 'home-banner'})-[:BELONGS_TO]->(cap:Capability)
RETURN c.id, f.name, cap.name, f.owner_team
```

---

## 5. Possibilidades operacionais

A ontologia destrava um conjunto de casos de uso que ferramentas tradicionais
(IDE, search, observability isolada) não cobrem porque elas não têm a
**costura cruzada** entre código, intenção e infra.

### 5.1 Onboarding técnico cirúrgico

> *"Acabou de entrar no time Payments. O que ele toca?"*

```
Team:Payments
  └─ OWNS ─▶ Service / Module / Endpoint
                │
                └─ CONTAINS ─▶ Function ─ OWNED_BY ─▶ Person
                                  │
                                  └─ INVOKES ─▶ Call ─ feature_tags ─▶ Feature
```

Em uma query, o novo dev tem: serviços do time, endpoints expostos, funções
ativas, donos individuais (mentores naturais), e features de negócio que
o time entrega. Em vez de garimpar README, ele entra no grafo.

### 5.2 Análise de impacto pré-PR

> *"Vou mudar `Function:CreateUser`. O que pode quebrar?"*

Reverter as arestas:
1. `Function ← INVOKES ← Call` — quem chama em-processo
2. Para cada Call cujo `TARGETS` = essa Function: subir para o caller Function
3. Recursivamente, até bater em `Endpoint` ou `JobSchedule`

Resultado: lista de endpoints/jobs afetados, com features (via tags) e
teams owners para notificar. Pré-PR sai do "achismo" para mapeamento.

### 5.3 Patching de segurança

> *"Saiu CVE-2024-XXXX no chi v5.0.x. Quem é afetado?"*

```
SecurityAdvisory:CVE-2024-XXXX
  └─ AFFECTS ─▶ Framework:chi@v5.0.7
                    │
                    └─ ← DEPENDS_ON ─ Module
                                          │
                                          └─ CONTAINS ─▶ Function/Endpoint
                                                            │
                                                            └─ OWNED_BY ─▶ Person/Team
```

Lista de pessoas a notificar é direta. Bonus: se `SecurityAdvisory
-PATCHED_IN-> Framework{since_version=v5.0.8}`, dá pra calcular quem **já**
está patcheado vs quem precisa upgradar.

### 5.4 Compliance de licença

> *"Quais services produzem binário com dependência copyleft (GPL/AGPL)?"*

```
License{copyleft: true}
  └─ ← LICENSED_UNDER ─ Framework
                            │
                            └─ ← DEPENDS_ON ─ Module ─ CONTAINS← ─ Service
```

Auto-geração de SBOM filtrado por classe de licença. Antes de release,
audit automático.

### 5.5 Cost-of-feature

> *"Quanto custa entregar a feature `home-banner`?"*

Ponte code↔infra↔cost (já documentada em `02-cost.md`):

```
Feature:home-banner
  ←── feature_tags ── Call (DataAccess)
                          │
                          └─ TARGETS ─▶ Persistence:rds/users-db
                                            │
                                            └─ subject de ─▶ Cost:[2026-04-01,2026-05-01]
```

Soma de custos dos resources atingidos pelas Calls com tag `home-banner`,
ponderada por intensidade de uso, dá custo atribuído à feature.

### 5.6 Detecção de cross-domain coupling

> *"Algum HttpCall sai do domain Catalog em direção a um Endpoint em domain
> Billing fora do canal contratual?"*

```
HttpCall ──TARGETS──▶ Endpoint
   │                      │
   │ ← INVOKES            │ ← CONTAINS ─ Service
   │                                       │
   │ ← CONTAINS ─ Service                  └─ feature_tags → Feature → Capability → Domain:Billing
   │                │
   │                └─ feature_tags → Feature → Capability → Domain:Catalog
```

Quando `Domain(caller) ≠ Domain(callee)` e não há `Schema` documentando o
contrato, é *boundary violation*. Detectável por consulta.

### 5.7 Simulação de fluxo a partir de Endpoint

> *"Dado um POST `/checkout`, simule o que acontece com payload X."*

A partir do `Endpoint`:
1. `Endpoint.Request.type_ref` resolve `Type` (campos e formato).
2. `Endpoint -HANDLED_BY-> Function` — entra na função handler.
3. `Function.Params` casa com payload deserializado.
4. `Function -INVOKES-> Call` em ordem (`#1, #2, #3...`).
5. Cada `Call.Args[].source` declara origem do valor (literal | variável
   declarada antes | retorno de Call anterior). Reconstrói DAG.
6. `Call -TARGETS->` indica onde o efeito vai (outra Function, Endpoint
   externo, tabela, fila).
7. Recursão até esgotar ou bater em primitivo/external.

Resultado: trace simbólico do request. Não é execução real — é simulação
estrutural. Útil para code-review automatizado, geração de testes, e como
input para LLMs que precisam responder *"o que acontece se eu mandar isso
nesse endpoint?"* sem rodar o sistema.

### 5.8 Auditoria de propriedade órfã

> *"Funcionário X saiu. O que ficou órfão?"*

```
Person:X
  └─ ← OWNED_BY ─ Function / Call / Type / Variable
                       │
                       └─ ← INVOKES / TARGETS ─ outras Calls
```

Lista exata. Sucessores são atribuídos por **CODEOWNERS update** ou por
heurística (próximo committer no `git blame` desses arquivos). Bitemporal
preserva: a versão pré-saída do nó ainda existe e responde queries
*"quem era dono em 2026-03-15?"*.

### 5.9 Contexto cirúrgico para LLM

> *"Gere prompt context apenas para arquivos relevantes à feature `home-banner`."*

Em vez de `git grep` ou tree completo:

```cypher
MATCH (c) WHERE 'home-banner' IN c.feature_tags
WITH DISTINCT c.location.file AS f
MATCH (mod:Module)-[:CONTAINS*]->(any) WHERE any.location.file = f
RETURN DISTINCT f, mod.id
```

LLM recebe só os arquivos onde a feature tem trace, agrupados por Module
(para preservar contexto de namespace). Reduz prompt size em ordem de
grandeza para refactor cross-cutting.

### 5.10 Drift entre intenção e código

> *"Existe Feature no governance plane que NÃO tem código tagueado?"*
> *"Existe código com tag apontando para Feature inexistente?"*

```cypher
// Features sem código
MATCH (f:Feature)
WHERE NOT EXISTS { MATCH (c) WHERE f.short_id IN c.feature_tags }
RETURN f.short_id, f.name, f.owner_team

// Tags órfãs
MATCH (c) UNWIND c.feature_tags AS tag
WITH tag WHERE NOT EXISTS { MATCH (f:Feature {short_id: tag}) }
RETURN DISTINCT tag, count(*) AS occurrences
```

Roteiro de **governance hygiene** rodável em CI. Drift de produto vs código
fica observável.

---

## 6. Trade-offs

| Decisão | Ganho | Custo |
|---------|-------|-------|
| **Toda Call é nó (boundary + in-process)** | Granularidade fina para governance, simulação, observabilidade ancorada | Grafo cresce ~10x em relação a só boundary; coletor mais caro |
| **Params/Fields como metadado (não edge)** | Ordem e nomes preservados; simulação direta; sem explosão de edges para primitivos | Queries "quais funções aceitam Type X?" exigem scan indexado, não traversal |
| **Feature-tag denormalizada** | Sem milhões de edges code→Feature | Tag pode divergir de UserStory (lint resolve) |
| **`Type` ≠ `Schema` quando ambos existem** | Distingue verdade-do-código de verdade-do-contrato | Coletor precisa emitir `SERIALIZES_AS` para ligar |
| **Module como nó com `CONTAINS` aninhável** | Frameworks/teams ancoram em granularidade certa; dependências entre modules viram topologia | Mais um nível de navegação para chegar em Function |
| **5 níveis fixos no governance** (Company→BusinessArea→Team→Person→Role) | Schema fechado, previsível | Empresas com hierarquia atípica adaptam via atributos, não nível novo |
| **Identidade semântica (não posicional)** | Refator de formatação não destrói linha temporal; rename de pacote intermediário só atualiza CONTAINS | Renames de símbolo aparecem como (delete + create); correlação de rename é heurística pós-coletor (MVP não cobre) |
| **`Persona` só via `UserStory`** | Disciplina: associação a perfil obriga story documentada | Código legado sem stories fica sem Persona-mapping (intencional) |

---

## 7. Evolução

### Curto prazo (MVP)
- Coletor Go cobre Service/Module/Endpoint/Function/Type/Variable + Call boundaries (HttpCall/DataAccess/EventPublish/QueueSend) + FunctionCall in-process.
- Framework/License via `go.sum` + `GOPROXY` metadata + `licensee`.
- SecurityAdvisory via GitHub Security Advisories API.
- Governance plane: Company/BusinessArea/Team/Person/Role/Domain/Capability/Feature/Epic/UserStory/Persona via CRUD (extensão de F-012).
- Feature-tag em código via comentário convencional (`// @feature:home-banner`) + lint job.

### Médio prazo
- Coletores adicionais: TypeScript (LSP), Python (`ast` + `mypy`), Java (Spoon), Rust (`syn`).
- `Schema` como nó com coletor proto/OpenAPI nativo.
- Resolução de `Call.TARGETS` para chamadas dinâmicas (interface dispatch) via análise estrutural.
- Bridge `DataAccess.TARGETS` → infra `Persistence` (cruzando code plane e infra plane).
- HRIS sync para popular Team/Person/Role automaticamente.
- PR labels disparam tag de Feature em código (extensão de F-013).

### Longo prazo
- Simulação executável (não só estrutural): valores fluindo em mock através das Calls.
- Graph embeddings (`Node2Vec`/`GraphSAGE`) para detecção de anomalias arquiteturais.
- Knowledge graph + agente LLM sobre essa ontologia (responde "por que `home-banner` está lento?" navegando code→infra→cost→telemetria).
- Federação cross-tenant para benchmark anônimo (qual a topologia média de um Service de Identity em N empresas?).

---

## 8. Resumo executivo

- **Code plane:** 12 kinds (`Service`, `Module`, `Endpoint`, `Function`, `Type`, `Variable`, `Schema`, `Call` família com 9 subkinds, `Framework`, `License`, `SecurityAdvisory`).
- **Governance plane:** 11 kinds (`Company`, `BusinessArea`, `Team`, `Person`, `Role`, `Domain`, `Capability`, `Feature`, `Epic`, `UserStory`, `Persona`).
- **Princípios-âncora:** identidade semântica · endereço cross-language flat · 4 condições para promoção · todas as calls como nós · params como metadado · feature-tag denormalizada · ownership bifurcada (Team alto-nível, Person execução).
- **Resultado prático:** onboarding em uma query · análise de impacto navegável · patching de CVE rastreável até pessoa · cost-of-feature calculável · contexto cirúrgico para LLM · drift entre intenção e código auditável.

ADRs que sustentam: ADR-006 (identidade cross-language) · ADR-007 (Call como família) · ADR-008 (param/field como metadado) · ADR-009 (feature-tag denormalizada) · ADR-010 (ownership bifurcada).

Plano de implementação: `docs/backlog/code-ontology-plan.md`.
