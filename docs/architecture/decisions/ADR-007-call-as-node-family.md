---
id: ADR-007
title: Call como família de nós com identidade caller+idx
status: accepted
date: 2026-05-17
supersedes: []
superseded_by: []
related: [ADR-006]
---

# ADR-007 — Call como família de nós com identidade caller+idx

## Contexto

Na primeira versão do code plane (F-007), invocações entre funções eram
implícitas — não havia nó nem aresta para `f() chama g()`. Para destravar:
- Reconstrução de fluxo end-to-end (Endpoint → Function → ... → Persistence).
- Governança fina (uma feature pode estar em UMA call dentro de uma função
  que contém múltiplas features de owners diferentes).
- Observabilidade ancorada (cada span de trace mapeia a uma call site).
- Análise de impacto bidirecional.

A pergunta é: invocação é **edge** (`Function -CALLS-> Function`) ou **nó**?

## Decisão

**Toda invocação é nó.** Família `Call` com 9 subkinds:

| Subkind | Categoria |
|---------|-----------|
| `FunctionCall` / `MethodCall` | In-process |
| `HttpCall` / `RpcCall` | Cross-process síncrono |
| `EventPublish` / `EventSubscribe` | Pub/sub assíncrono |
| `QueueSend` / `QueueReceive` | Fila |
| `DataAccess` | Store (DB, cache) |
| `JobSchedule` | Trigger temporal |

**Identidade:**
```
urn:ce:code:<repo>:call/<kind>!<caller-function-id>#<idx>
```

`<idx>` = ordem de aparição no AST da função caller (1, 2, 3...). Estável
a movimentos de linha. Adicionar uma call **antes** rebumpa idx das
posteriores — mudança real de estrutura, refletida no grafo.

**Arestas:**
- `Function -INVOKES-> Call` — toda call tem um caller.
- `Call -TARGETS-> Function|Endpoint|Topic|Queue|Persistence|ExternalService` — alvo resolvido (zero ou um; pode estar vazio se não-resolvível em compile time).
- `Call -USES-> Framework` opcional (cliente HTTP, ORM).

**Metadados estruturados (não-edges):**
- `Call.Args[]` com `{name, type_ref, source}` onde `source` é
  `literal:'...'` | `variable:name` | `return-of:<call-urn>`.
- `Call.Returns[]` com `{name, type_ref}`.

## Consequências

**Positivas:**
- Granularidade fina destrava governança por feature em nível sub-função.
- Cross-cutting concerns (timeout, retry, circuit breaker, idempotency)
  vivem na call — propriedades nem do caller nem do callee.
- Alvo pode ser desconhecido: a call existe mesmo que `url` venha de
  variável de ambiente. `TARGETS` aparece depois quando resolvível.
- Bitemporal preciso: adicionar/remover call site é evento distinto da
  criação/deleção da função.
- Ponto âncora natural para observabilidade (cada span = uma call URN).

**Negativas:**
- Grafo cresce em ordem de magnitude. Funções típicas têm 5–30 calls.
- Coletor mais caro: AST walk completo, não só declarações.
- Storage e indexação proporcionalmente maiores.
- Resolução de `TARGETS` para chamadas via interface (dispatch dinâmico)
  exige análise estrutural mais sofisticada — alguns `TARGETS` ficam
  vazios no MVP.

## Alternativas consideradas

**Apenas edge `Function -CALLS-> Function`.** Rejeitada porque:
- Sem nó para a call, não há onde ancorar feature_tag fina.
- Edges no modelo bitemporal do projeto não têm versão própria como nós
  têm — perde granularidade temporal.
- Alvo precisa existir para criar a edge; calls dinâmicas ficam de fora.

**Calls de boundary como nó + in-process como edge.** Considerada como
recorte MVP para reduzir grafo. Rejeitada porque governança fina exige
ancoragem no in-process também (uma função pode produzir N banners de
N features via N calls in-process).

**Identidade posicional (`caller + file:line:col`).** Rejeitada pelo
mesmo motivo de ADR-006: instável a reformatação.

## Implementação

`#idx` é determinístico via depth-first traversal do AST da função
caller, contando apenas nós de chamada. Coletor precisa de ordem
canônica do AST da linguagem — para Go: `go/ast` em ordem de fonte;
para outras, idem.
