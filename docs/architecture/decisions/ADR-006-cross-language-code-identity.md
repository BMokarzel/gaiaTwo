---
id: ADR-006
title: Identidade cross-language para nós do code plane
status: accepted
date: 2026-05-17
supersedes: []
superseded_by: []
related: [ADR-001, ADR-002]
---

# ADR-006 — Identidade cross-language para nós do code plane

## Contexto

A primeira versão do code plane (F-007) modelou identidade usando
conceitos Go: `Package` em `Function`/`Endpoint`, `GoModule` em `Service`.
A URN canônica de Function é
`urn:ce:code:<repo>:function/<module-path>!<package>.<symbol>`.

Para mapear Python, TypeScript, Java, Rust e C# precisamos de um esquema
que não vaze convenção de Go. Há também uma decisão correlata: a URN
deve ou **não** carregar `Location` (file, line, col).

## Decisão

**1. Identidade é semântica, nunca posicional.**

URN e `ContentHash` **não** incluem file/line/col. `Location` é atributo
do nó. Refator que apenas move código (sem mudar assinatura/conteúdo)
mantém a mesma URN — preserva a linha temporal bitemporal.

Sobrecargas, closures e funções anônimas são desambiguadas com um
**discriminador estável no escopo** (ex.: `pkg.OuterFunc$closure#1`,
numerado pela ordem de aparição no AST), não com line:col cru.

**2. Endereço cross-language flat: `Namespace` + `Symbol`.**

Cada coletor de linguagem produz dois campos canônicos, separados:

| Linguagem | `Namespace` | `Symbol` |
|-----------|-------------|----------|
| Go | `<module-path>/<package>` (ex.: `internal/api/users`) | `CreateUser` ou `Service.CreateUser` (receiver method) |
| Python | path de packages (`myapp/services/users`) | `create_user` ou `UserService.create_user` |
| TypeScript | path do arquivo/módulo (`src/services/users.ts`) | `createUser` |
| Java | path canônico de package (`com/acme/users`) | `UserService.createUser` |
| Rust | path do módulo (`crates/api/users`) | `create_user` |
| C# | namespace (`MyApp.Users`) | `UserService.CreateUser` |

Métodos de tipo viram `Type.Method` no `Symbol` — convenção universal e
legível, evita kind separado.

URN nova de Function:
```
urn:ce:code:<repo>:function/<service-id>!<namespace>!<symbol>
```

URN nova de Service abandona `GoModule`. Identidade fica em
`Repo + ModulePath`; `Language` e `Manifest` (caminho do `go.mod` /
`package.json` / `pyproject.toml` / `Cargo.toml` / `pom.xml`) são
atributos.

**3. Critério das 4 condições para promover atributo a nó.**

Ao decidir se algo merece nó próprio, todas devem ser verdade:

1. Identidade global que transcende quem referencia.
2. Atributos úteis que se quer consultar.
3. É hub: recebe ou origina arestas para múltiplos nós distintos.
4. O grafo é atravessado **através** dele, não só filtrado **por** ele.

Falha em qualquer uma → atributo.

## Consequências

**Positivas:**
- Coletores de qualquer linguagem podem produzir nós no mesmo schema.
- Refatoração de movimentação não invalida histórico bitemporal.
- Bitemporalidade reflete mudanças semânticas (assinatura, conteúdo),
  não cosméticas (formatação, mover entre arquivos).
- Métodos de tipo não precisam de `MethodNode` — convenção `Type.Method`
  cobre.

**Negativas:**
- Renames de símbolo aparecem como `(delete v1) + (create v1)`.
  Correlação de rename fica como heurística pós-coletor (futuro).
- Mover função para outro pacote rebumpa Namespace; é tratado como nova
  função (não rename). Aceito no MVP — caso raro e detectável.
- Migração: nós existentes em produção (F-007) usam `Package`; precisa
  de script de migração para popular `Namespace` derivando do antigo
  esquema antes de remover o campo.

## Alternativas consideradas

**(B) Identidade posicional (`file + line + col` na URN).** Rejeitada:
qualquer formatter destrói a identidade; histórico bitemporal vira
ruído.

**(B') Scope chain estruturada `[]ScopeFrame`.** Mais expressivo,
permite queries estruturais (`"todas funções dentro da classe X"`).
Rejeitada para o MVP por over-engineering: o `Namespace` plano já
permite isso com index secundário (`STARTS_WITH 'com.acme.users.'`).
Pode ser introduzida sem quebrar URN se necessário.

**(C') Opaco — `qualifier: string` único.** Coletor produz string
canônica; URN não interpreta. Rejeitada: perde poder de query, vira
regex.
