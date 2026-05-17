---
id: F-022
title: Schema node (proto, OpenAPI, JSON Schema, Avro, GraphQL)
status: entity + coletor proto + adapter OpenAPI done; demais formatos backlog
modules: [code]
depends_on: [F-021]
modeling_impact: yes
adrs: [ADR-006, ADR-008]
epic: E-007
updated: 2026-05-17
---

# F-022 — Schema

## Problema

Contrato declarado fora do código (proto, OpenAPI) é cidadão distinto
de Type — uma Schema gera múltiplos Types em linguagens distintas.
SERIALIZES_AS é a ponte.

## Mudanças

- Novo kind `Schema` em `internal/entity/node/schema.go` com
  discriminador `SchemaFormat`: proto/openapi/json-schema/avro/graphql.
- URN: `urn:ce:code:<repo>:schema/<service-module-path>!<schema-id>`
  ou global `urn:ce:code:_global:schema/...!<schema-id>`.
- Arestas: `SERIALIZES_AS` (Type→Schema), `IMPORTS` (Schema→Schema).

## DoD

- [x] `node.Schema` + `NewSchemaURN` + `ContentHash`.
- [x] Adjacency SERIALIZES_AS / IMPORTS validadas.
- [x] Coletor proto em `internal/modules/code/collector/proto`: parse
      linha-a-linha de `*.proto` (sem `protoc`), suporta `package`,
      `message` aninhado, modifiers (`repeated`/`optional`), `map<K,V>`
      como TypeRef literal. `enum`/`service`/`rpc` ignorados; `oneof`
      planificado. Confidence=1.0, Method=Declared.
- [x] Adapter OpenAPI: `internal/modules/code/ingest/openapi`
      parseia `components.schemas`, emite `node.Schema`
      (Format=openapi, Version = `info.version`) + arestas `IMPORTS`
      sempre que um `$ref` local (`#/components/schemas/X`) liga dois
      schemas. Fields preenchidos via `properties` (com `required[]`
      → `Optional` invertido); refs externas ignoradas no MVP.
      `array<X>` materializado como `array<$ref:X>` no `TypeRef`.

## Backlog

- JSON Schema standalone (sem envelope OpenAPI).
- Avro (`.avsc`).
- GraphQL SDL (`type ... { ... }`).
- `oneOf`/`allOf`/`anyOf` materializados em FieldSlots (hoje só
  capturados como refs em `IMPORTS`).
