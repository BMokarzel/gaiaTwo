# Catálogo de erros — API v1

> Toda resposta 4xx/5xx da API v1 segue o padrão **RFC 7807** (com
> extensões controladas). Esta página é a referência viva dos códigos.
>
> Relacionado: F-016 (arquitetura modular v2), ADR-005.
>
> Atualizado: 2026-05-16

## Shape do payload

```http
HTTP/1.1 404 Not Found
Content-Type: application/problem+json

{
  "type":   "ce:err:org.team.not_found",
  "title":  "Team not found",
  "status": 404,
  "code":   "org.team.not_found",
  "detail": "org: team not found: urn:ce:org:team:inexistente",
  "urn":    "urn:ce:org:team:inexistente"
}
```

Campos:

| Campo | Origem | Notas |
|---|---|---|
| `type` | derivado de `code` (`ce:err:<code>`) | URI estável; clientes podem hard-codar |
| `title` | `Title()` do erro tipado, ou `http.StatusText(status)` | curto, humano |
| `status` | `HTTPStatus()` do erro tipado, ou inferido do sentinel | redundante com a linha HTTP, intencional (RFC 7807) |
| `code` | `<modulo>.<entidade>.<situacao>` lowercase | chave estável para clientes |
| `detail` | `Error()` do erro tipado | mensagem técnica; pode mudar |
| `extras` (top-level) | `Details()` do erro tipado | campos achados em top-level (não num objeto aninhado) |

## Convenção de `code`

`<modulo>.<entidade>.<situacao>`, tudo lowercase, separado por ponto.

- **`modulo`**: nome do módulo (`org`, `code`, `infra`) ou domínio
  transversal (`graph`, `search`).
- **`entidade`**: nome da entidade do bounded context
  (`team`, `service`, `compute`, `urn`, `reports`).
- **`situacao`**: `not_found`, `invalid`, `cycle`, `conflict`,
  `bad_request`.

Códigos legados sem prefixo (`not_found`, `bad_request`, `conflict`,
`ambiguous`, `internal`) são fallback do `core/errs.Render` quando o
erro propagado é um sentinel cru de `repository` (sem tipo de domínio
acima). Devem desaparecer à medida que cada caminho recebe erro tipado.

## Códigos por módulo

### `org` (modules/org/errs.go)

| `code` | Status | Title | Extras | Disparado por |
|---|---|---|---|---|
| `org.team.not_found` | 404 | Team not found | `urn` | GET/list de Team inexistente (ou em `as_of`) |
| `org.squad.not_found` | 404 | Squad not found | `urn` | GET/list de Squad inexistente |
| `org.person.not_found` | 404 | Person not found | `urn` | GET/list de Person inexistente |
| `org.urn.invalid` | 400 | Invalid URN | `urn`, `reason` | URN malformada ou Kind incompatível com endpoint |
| `org.reports.cycle` | 422 | Reporting cycle detected | `path` | Ciclo em `ReportsTo` (não emitido hoje — controller trunca) |

### `code` (modules/code/errs.go)

| `code` | Status | Title | Extras | Disparado por |
|---|---|---|---|---|
| `code.service.not_found` | 404 | Service not found | `urn` | GET/list de Service inexistente |
| `code.endpoint.not_found` | 404 | Endpoint not found | `urn` | GET/list de Endpoint inexistente |
| `code.function.not_found` | 404 | Function not found | `urn` | GET/list de Function inexistente |
| `code.urn.invalid` | 400 | Invalid URN | `urn`, `reason` | URN malformada, Kind incompatível, `q` vazia em busca |

### `infra` (modules/infra/errs.go)

| `code` | Status | Title | Extras | Disparado por |
|---|---|---|---|---|
| `infra.account.not_found` | 404 | Account not found | `urn` | GET de Account inexistente |
| `infra.region.not_found` | 404 | Region not found | `urn` | GET de Region inexistente |
| `infra.compute.not_found` | 404 | Compute not found | `urn` | GET de Compute inexistente |
| `infra.persistence.not_found` | 404 | Persistence not found | `urn` | GET de Persistence inexistente |
| `infra.network.not_found` | 404 | Network not found | `urn` | GET de Network inexistente |
| `infra.urn.invalid` | 400 | Invalid URN | `urn`, `reason` | URN malformada ou Kind incompatível |

### Fallback (`core/errs.Render`)

Acionados quando o erro é sentinel de `repository` sem `HTTPProblem` na
cadeia. Devem ir desaparecendo na medida em que módulos novos declaram
erros tipados.

| `code` | Status | Title | Origem |
|---|---|---|---|
| `not_found` | 404 | Not Found | `repository.ErrNotFound` |
| `ambiguous` | 409 | Conflict | `repository.ErrAmbiguous` |
| `conflict` | 409 | Conflict | `repository.ErrConflict` |
| `bad_request` | 400 | Bad Request | `repository.ErrInvalidArgument` (propaga `err.Error()`) |
| `internal` | 500 | Internal Server Error | qualquer outro (sem leak de `err.Error()`) |

## Como adicionar um código novo

1. **Declarar tipo em `modules/<X>/errs.go`** com `Error`, `HTTPStatus`,
   `Code`, `Title`, `Details`, `Unwrap` (apontando para o sentinel mais
   próximo de `repository`).
2. **Retornar do Service** (`modules/<X>/service`) — nunca do controller.
3. **Atualizar esta tabela** acrescentando o `code` na seção do módulo.
4. **Não precisa tocar no controller**: `httpserver.WriteError` ↔
   `core/errs.Render` descobrem via `errors.As`.

## Como o cliente deve tratar

- **Hard-coda `code`**, não `detail` nem `title`. `code` é contrato;
  os demais podem mudar entre versões.
- **Lê `status` da linha HTTP**, redundância no body é só para clientes
  que perderam o cabeçalho (gateways, logs achatados).
- **Trata códigos desconhecidos** como genéricos pelo `status`. Códigos
  novos podem aparecer a qualquer release (semver minor).
- **`type` é uma URI**: pode ser usado para deduplicar em sistemas de
  agregação de erros (Sentry/Datadog).

## Pendências relacionadas

- Módulos `code` e `infra` hoje só expõem dados via `app/graph` e
  `app/search` — ainda não há endpoints `/v1/code/*` ou `/v1/infra/*`
  específicos. Quando surgirem, os controllers desses módulos vão
  retornar diretamente os erros catalogados acima.
- O fallback `bad_request` ainda propaga `err.Error()` direto do
  sentinel. Substituir por erros tipados quando aparecer caso real onde
  isso vaze detalhe sensível.
