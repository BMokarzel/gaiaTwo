---
id: F-008
title: OpenAPI ingest
status: done
modules: [code]
depends_on: [F-007]
modeling_impact: no
adrs: []
epic: E-003
updated: 2026-05-17
---

# F-008 — OpenAPI ingest

## Problema

Nem todo serviço tem AST acessível (terceiros, gateways, serviços em
linguagens não suportadas). OpenAPI é o contrato comum. Ingerir specs
permite popular `Endpoint`s mesmo sem fonte, e — quando combinado com
F-007 — reconciliar AST com contrato declarado.

**Para quem:** mesmos consumers que F-007 + casos de
serviços externos / linguagens não suportadas.

**Dor sem ela:** plano `code` é cego a serviços fora do escopo do AST.

## Escopo

**Inclui:**
- Comando CLI `ce ingest openapi --spec=<path|url> --service=<service_urn>`.
- Parse de OpenAPI 3.0/3.1 (não 2.0 / Swagger no MVP).
- Cria/atualiza nós `Endpoint` (1 por `path × method`) ligados ao
  `Service` apontado por `--service`.
- Propriedades capturadas: `method`, `path`, `operation_id`, `tags`,
  `summary`, `request_schema_hash`, `response_schema_hash`.
- Idempotência por `(service_urn, method, path)`.

**NÃO inclui:**
- Geração de `Service` (precisa pré-existir; criar manualmente ou
  via F-007).
- Diff de schema entre versões (futuro: drift detection).
- Tabelas/persistência (continua fora).
- Swagger 2.0.

**Precondições:**
- Service URN existe.
- OpenAPI spec acessível.

## Toque no grafo

- **Lê:** `Service` (referenciado por URN).
- **Escreve:** `Endpoint` + edge `DefinedIn(Endpoint → Service)`,
  com `source = openapi` para distinguir de AST.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** mesma regra padrão (hash de campos significativos
  → nova versão se mudou).

## Critérios de aceite

- [x] Parser stdlib-only (JSON+YAML) aceita OpenAPI 3.0/3.1 e
      rejeita Swagger 2.0, specs vazios e ausência de `paths`.
- [x] `ce ingest openapi --spec=… --service=urn:ce:<prov>:<acct>:service/<id>`
      gera N Endpoints (1 por `path × método HTTP reconhecido`),
      idempotentes por URN canônica `endpoint/<service-id>!<METHOD>:<route>`.
- [x] Re-rodar sem mudanças no spec: ContentHash idêntico → repo
      bitemporal não versiona (idempotência delegada).
- [x] Adicionar 1 path no spec e re-rodar: 1 Endpoint novo aparece;
      os outros não viram versão nova.
- [x] Mudar request/response schema de uma operação → `request_schema_hash`
      / `response_schema_hash` muda → ContentHash diverge → v2.
- [x] `source = openapi` registrado em `Source.Collector`
      (`code/ingest/openapi`), em `Endpoint.Properties.source` e nas
      properties da edge `DefinedIn`. Quando F-007 também escreveu o
      mesmo Endpoint (mesmo `method+path`), o nó é o mesmo
      (URN compartilhada) e os campos `source` divergem.
- [x] `operationId` ausente → handler fallback `openapi:<sha8>` derivado
      de `(method, path)`.

## Riscos / incerteza

- **Reconciliação AST vs OpenAPI.** Mesmo Endpoint vindo de duas
  fontes pode ter campos divergentes. **Política inicial:** propriedades
  prefixadas por fonte (`ast_handler_symbol`, `openapi_operation_id`).
- **`operation_id` ausente.** Caem em fallback hash do `(method+path)`.
- **Refs externas (`$ref` para URLs).** Não resolver no MVP — apenas
  hash do `$ref` literal.
- **Versionamento da spec.** Hash inclui versão da spec; bump de
  versão = nova versão de Endpoint.

## Notas de implementação

- Pacote `internal/modules/code/ingest/openapi/` (parser + collector + writer).
- **Stdlib + `gopkg.in/yaml.v3`** (já no go.mod). Decisão: não trazer
  `kin-openapi` — só consumimos um subset (`openapi`, `info.version`,
  `paths.{p}.{method}.{operationId,summary,tags,requestBody,responses}`).
  Schemas de request/response são reduzidos a `sha256(canonical-json)`
  para detectar drift sem nó-Schema dedicado (F-022 fará isso).
- URN do Endpoint reusa `node.NewEndpointURN(<account>, <id>, METHOD,
  route)` onde `<account>` e `<id>` vêm do parse do Service URN. Isso
  garante que F-007 (AST) e F-008 (spec) convergem para a mesma URN
  quando o mesmo `(service, method, path)` é detectado pelos dois lados.
- Idempotência delegada ao repo bitemporal via `Endpoint.ContentHash`.
- CLI: `cmd/cli/ingest_openapi.go` espelha `ingest_hris` (memory|neo4j,
  `--dry-run`, `--run-id`).
