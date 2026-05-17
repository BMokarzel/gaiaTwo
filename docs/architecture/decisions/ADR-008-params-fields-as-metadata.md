---
id: ADR-008
title: Params, returns, fields e methods como metadado estruturado, não edges
status: accepted
date: 2026-05-17
supersedes: []
superseded_by: []
related: [ADR-006, ADR-007]
---

# ADR-008 — Params, returns, fields e methods como metadado estruturado, não edges

## Contexto

Funções têm parâmetros e retornos. Structs/classes têm campos e métodos.
Endpoints têm request e response bodies. Calls têm argumentos.

Modelar essas relações como edges (`Function -ACCEPTS{position,name}-> Type`)
preserva traversal, mas:
- Explode em edges para tipos primitivos sem nó (`int`, `string`, `bool`).
- Perde ordem natural (qual é o primeiro param?).
- Perde nomes (`userID` vs `name` vs `email`).
- Simulação de fluxo precisa reconstruir a assinatura a partir de edges.

## Decisão

Param, return, field, method, body de request/response são **metadados
estruturados embutidos** no nó, não edges. Cada slot referencia um tipo
via `TypeRef` (string discriminada por prefixo).

**Esquema:**

```jsonc
// Function.Params (e simétrico Function.Returns)
[
  { "name": "ctx",    "position": 0, "type_ref": "context.Context",         "optional": false },
  { "name": "userID", "position": 1, "type_ref": "string",                  "optional": false },
  { "name": "user",   "position": 2, "type_ref": "urn:ce:code:repo:type/...!User", "optional": false }
]
```

```jsonc
// Type.Fields
[
  { "name": "ID",    "position": 0, "type_ref": "string", "tags": {"json": "id"} },
  { "name": "Email", "position": 1, "type_ref": "string", "tags": {"json": "email", "validate": "email"} },
  { "name": "Addr",  "position": 2, "type_ref": "urn:...:type/...!Address" }
]
```

```jsonc
// Type.Methods
[
  { "name": "Validate", "function_urn": "urn:...:function/...!api!User.Validate" }
]
```

```jsonc
// Endpoint.Request / Endpoint.Response[status]
{ "type_ref": "urn:...:type/...!CreateUserReq", "content_type": "application/json", "required": true }
```

```jsonc
// Call.Args (com origem do valor — base para simulação)
[
  { "name": "url",  "type_ref": "string",                "source": "literal:'https://api.x/users'" },
  { "name": "body", "type_ref": "urn:...:type/...!Payload", "source": "variable:p" }
]
```

**Discriminação de `type_ref`:**

| Prefixo | Significado | Gera nó? |
|---------|-------------|----------|
| `urn:ce:code:...:type/...` | Tipo declarado (struct/class/interface/enum) | Sim, nó `Type` separado |
| `urn:ce:code:...:schema/...` | Contrato externo (proto, OpenAPI) | Sim, nó `Schema` separado |
| outro literal (`int`, `string`, `error`, `*http.Response`, `google.protobuf.Timestamp`) | Primitivo/built-in | **Não** |

## Consequências

**Positivas:**
- Ordem e nome dos parâmetros preservados nativamente.
- Sem explosão de edges para tipos primitivos.
- Simulação direta: percorrer `Function.Params` casa com `Call.Args` por
  posição.
- Composição de struct (`User` contém `Address`) fica em
  `Type.Fields[].type_ref` — não duplica em edge.

**Negativas:**
- Queries "quais funções aceitam Type X?" exigem scan indexado, não
  traversal nativo. Neo4j: `WHERE 'urn:...!User' IN [p.type_ref for p in
  f.params]`. ClickHouse: `WHERE hasAny(arrayMap(p -> p.type_ref,
  f.params), ['urn:...'])`.
- Atributo de nó com formato complexo: serialização precisa ser estável
  (JSON canônico ou estruturado nativo no store).

## Edges que **permanecem** entre Types

Relações **não-posicionais** entre Types ficam como edges (não cabem como
slot nominal):

- `Type -IMPLEMENTS-> Type` — satisfação de interface
- `Type -EXTENDS-> Type` — herança / embedding
- `Type -ALIASES-> Type` — `type X = Y` semântico
- `Type -SERIALIZES_AS-> Schema` — tipo gerado a partir de contrato externo

## Alternativas consideradas

**Tudo edge.** Rejeitada pelos motivos no Contexto.

**Hybrid (metadado + edges denormalizadas).** Considerada para destravar
queries de "funções que aceitam Type X" via traversal. Rejeitada por
dobrar storage e exigir consistência entre duas representações; index
secundário sobre o atributo resolve sem essa complexidade.
