---
id: F-008
title: OpenAPI ingest
status: refined
modules: [code]
depends_on: [F-007]
modeling_impact: no
adrs: []
epic: E-003
updated: 2026-05-13
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

- [ ] Dado spec OpenAPI 3 com 20 paths × 2 métodos médios, quando
      `ce ingest openapi --spec=spec.yaml --service=urn:ce:internal::service/payments`,
      então grafo contém ~40 Endpoints linkados a esse Service, versão 1.
- [ ] Re-rodar sem mudanças no spec: nenhuma versão nova.
- [ ] Adicionar 1 path no spec e re-rodar: 1 Endpoint novo aparece;
      os outros não viram versão nova.
- [ ] Mudar `response_schema` de 1 path: aquele Endpoint vai para v2;
      v1 ganha `valid_to`.
- [ ] Quando F-007 também escreveu o mesmo Endpoint (mesmo
      `method+path`), os campos `source` divergem mas o nó é o mesmo
      (mescla via mesmo `external_id`).

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

- Pacote `internal/modules/code/ingest/openapi/`.
- Lib: `github.com/getkin/kin-openapi`.
