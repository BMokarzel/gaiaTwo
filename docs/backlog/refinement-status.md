# Painel de Refinamento

> Estado de cada feature/épico. Atualiza junto com cada mudança de
> estado.
>
> Atualizado: 2026-05-16

## Features por estado

| Estado | Quantidade | IDs |
|---|---|---|
| `idea` | 0 | — |
| `refining` | 0 | — |
| `refined` | 3 | F-008, F-012, F-013 |
| `ready` | 0 | — |
| `in_progress` | 0 | — |
| `done` | 13 | F-001, F-002, F-003, F-004, F-005, F-006, F-007, F-009, F-010, F-011, F-014, F-015, F-016 |
| `blocked` | 0 | — |

## Épicos por estado

| Épico | Estado | Features `done`/total |
|---|---|---|
| E-001 Infra Foundation | done | 2/2 |
| E-002 Cost Layer | done | 4/4 (F-003, F-004, F-005, F-006) |
| E-003 Code Plane | parcial | 2/3 (F-007, F-009 done; F-008 refined) |
| E-004 Org & Ownership | done | 2/2 (F-010, F-011) |
| E-005 Product & Capability | refined | 0/2 (F-012, F-013) |
| E-006 Plataforma de Leitura | done | 3/3 (F-014, F-015, F-016) |

## Atenção / stale

(nada stale agora)

## Próximas ações de refinamento

1. **F-008** OpenAPI ingest — fatiar em stories (prompt 03) quando
   entrar na fila.
2. **F-012** Capability/Feature CRUD — fatiar quando entrar na fila.
3. **F-013** Feature → Service link via PR — fatiar quando entrar na
   fila (depende de F-012).
4. Sem feature `in_progress` no momento.

## Histórico recente

- 2026-05-16: F-016 marcada como `done`. Catálogo de erros publicado em
  `docs/api/v1/errors.md`. `04-modular.md §9` atualizado refletindo
  passos 4-6 executados; passos 1-3 (`entity/*` → `core/*`) movidos
  para `pendencias.md`. Desvios documentados no próprio F-016
  (S-006/S-007 absorvidos por S-008 — `code` e `infra` sem controller
  próprio até aparecer endpoint de domínio).
- 2026-05-15: F-016 refinada (arquitetura modular v2: controllers por
  módulo, `core/errs`, `platform/httpserver`, port-sets em `app/`).
  Implementação executada ao longo do mesmo ciclo (S-001..S-011 no
  código).
- 2026-05-14: F-001 S-002 entregue. Hierarquia `Account → Region → Zone`
  + edges `Contains` via EC2 `DescribeAvailabilityZones`. Novos pacotes
  `infra/collector` (ScopeDiscoverer), `infra/collector/aws/topology`,
  `infra/service`. CLI persiste em backend memory; helper bitemporal de
  ancestrais reutilizável pelas próximas stories. 8 testes novos verdes.
- 2026-05-14: F-001 → `in_progress`. S-001 entregue (scaffold do
  collector AWS em `internal/modules/infra/collector/aws/` + CLI
  `ce extract aws` com validação STS GetCallerIdentity). Testes verdes.
- 2026-05-13: ADR-001 (monolito modular) e ADR-002 (bitemporal como
  invariante) escritas retroativamente.
- 2026-05-13: F-001 fatiada em 8 stories (S-001 a S-008), promovida
  para `ready`.
- 2026-05-13: F-003 fatiada em 5 stories (S-001 a S-005), promovida
  para `ready`.
- 2026-05-13: F-004 a F-015 refinadas em batch (idea → refined).
