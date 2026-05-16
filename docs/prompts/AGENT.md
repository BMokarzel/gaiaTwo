# AGENT.md — Instrução-Mestre para Assistentes

> Este arquivo orienta como Claude (ou outro LLM) deve operar dentro
> da pasta `docs/` do CostEngine. Carregue-o no início de cada sessão
> de refinamento.

## Contexto

CostEngine é uma plataforma de inteligência arquitetural baseada em
grafo. A documentação em `docs/` é viva: arquitetura, módulos, features
e backlog evoluem juntos. Existe biblioteca de prompts nesta pasta
mesma para orquestrar refinamento.

**Antes de qualquer trabalho sério**, leia:
1. `docs/README.md` (mapa)
2. `docs/architecture/00-overview.md` (síntese)
3. Se existir, `GRAPH_REPORT.md` na raiz (índice do graphify)

## Quando o usuário pedir...

| Pedido | Use o prompt | Saída esperada |
|---|---|---|
| "refina a feature X" / "vamos pensar em X" | [01-refine-feature](./01-refine-feature.md) | feature doc movida para `refining` ou `refined` |
| "essa feature mexe na modelagem?" / "valida o impacto" | [02-check-modeling-impact](./02-check-modeling-impact.md) | parecer + rascunhos de update / ADR |
| "fatia em stories" / "como entregar isso" | [03-slice-into-stories](./03-slice-into-stories.md) | lista de stories verticais |
| "revisa consistência" / "tem buraco?" | [04-review-consistency](./04-review-consistency.md) | lista de órfãos, refs mortas, sugestões |

## Princípios de operação

1. **Não invente conceito que não está nos docs.** Se faltar contexto,
   pergunte ou marque `[CARENCIA]` no doc gerado.
2. **Frontmatter mínimo.** Não preencha campo que não se aplica;
   omita-o.
3. **Prosa direta.** Sem prosa motivacional, sem "vamos juntos
   construir...". O usuário é técnico.
4. **Mudança em arquitetura é evento, não detalhe.** Se uma feature
   força mudança em `architecture/*.md`, propor ADR antes de aplicar.
5. **Toda referência cruzada usa link relativo:**
   `[F-007](../features/F-007-titulo.md)`, `[ADR-003](../architecture/decisions/ADR-003-titulo.md)`.
6. **Datas sempre absolutas.** "Quinta" → `2026-05-14`.
7. **Idioma:** pt-BR no texto; en em identificadores/código.
8. **Não duplique conteúdo.** Se já existe em outro doc, referencie.

## Limites

- Não criar feature/módulo/ADR sem aprovação explícita do usuário.
- Não mover arquivo de estado sem que o critério do prompt esteja
  satisfeito.
- Não inventar URN, Kind, edge — usar apenas o que está documentado.
  Inventar é sinal de gap → marcar como tal.

## Ciclo padrão de refinamento

```
usuário traz ideia
    │
    ▼
[01] refine feature ────► feature em refining
    │
    ▼
[02] check modeling ────► impacto identificado?
    │                          │
    │                       sim│
    │                          ▼
    │                     ADR + update arch
    │                          │
    └──────────────────────────┘
    │
    ▼
feature em refined
    │
    ▼
[03] slice stories ─────► feature em ready
    │
    ▼
usuário prioriza e desenvolve
    │
    ▼
[04] revisa consistência (periodicamente)
```
