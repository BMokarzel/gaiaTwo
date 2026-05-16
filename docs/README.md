# CostEngine — Documentação

> Sistema vivo de design e refinamento. Esta pasta é organizada para
> evoluir organicamente: arquitetura é estável, módulos espelham o
> código, features são unidades de refinamento, backlog organiza a
> execução.

## Mapa rápido

```
docs/
├── README.md                        ← você está aqui
├── architecture/                    ← como o sistema pensa (estável)
│   ├── 00-overview.md               ← síntese e mapa
│   ├── 01-modeling.md               ← modelo de domínio (nós, edges)
│   ├── 02-cost.md                   ← plano de custo (CUR, ClickHouse)
│   ├── 03-business.md               ← org/product/revenue
│   ├── 04-modular.md                ← organização do código-fonte
│   ├── 05-platform.md               ← 6 produtos + agente
│   ├── 06-system.md                 ← topologias de deploy
│   ├── 07-tiers.md                  ← BFF + products + graph
│   └── decisions/                   ← ADRs numeradas
├── modules/                         ← um doc por módulo do código
│   └── infra.md                     ← referência (template vivo)
├── features/                        ← unidades de refinamento
│   └── F-NNN-titulo.md
├── backlog/
│   ├── roadmap.md                   ← plano de implementação por fase
│   ├── epics.md                     ← E-NNN agrupando features
│   ├── priorities.md                ← ordenação corrente + razão
│   └── refinement-status.md         ← painel de estados
└── prompts/                         ← biblioteca de alavancagem
    ├── AGENT.md                     ← instrução-mestre
    ├── 01-refine-feature.md
    ├── 02-check-modeling-impact.md
    ├── 03-slice-into-stories.md
    └── 04-review-consistency.md
```

## Como usar (loop padrão)

1. **Ideia chega** → criar `features/F-NNN-<titulo>.md` em estado `idea`.
2. **Refinar** → invocar `prompts/01-refine-feature.md`. Estado vira `refining`.
3. **Checar impacto na modelagem** → invocar `prompts/02-check-modeling-impact.md`.
   - Se há impacto: atualizar `architecture/*.md` ou abrir ADR em `decisions/`.
4. **Fatiar em stories** → invocar `prompts/03-slice-into-stories.md`. Estado vira `ready`.
5. **Priorizar** → mover para `backlog/priorities.md`.
6. **Construir** → estado `in_progress` → `done`.
7. **Atualizar módulo afetado** → o doc em `modules/` reflete o estado atual.
8. **Revisar consistência** (toda semana) → invocar `prompts/04-review-consistency.md`.

## Tipos de artefato (papéis)

| Tipo | Pergunta que responde | Estabilidade |
|---|---|---|
| Architecture | Como o sistema pensa | muito estável; muda via ADR |
| Module | O que este pedaço faz | estável; atualiza com refactor |
| Feature | Que capacidade entregar | volátil até `refined` |
| Story | Que fatia construir agora | curta; some após shipped |
| ADR | Por que tomamos esta decisão | imutável após `accepted` |

**Regra de ouro:** descer de nível restringe; subir generaliza. Feature
nunca contradiz arquitetura — se contradiz, abrir ADR.

## Estado das features (máquina simples)

```
idea ──► refining ──► refined ──► ready ──► in_progress ──► done
                │
                └── blocked (depende de outra coisa)
```

## Integração com graphify

Quando rodar `/graphify .` na raiz do projeto, o grafo gerado indexa
toda esta pasta (mais o código). Os prompts (especialmente o 04) leem o
`GRAPH_REPORT.md` resultante para detectar órfãos, refs mortas e
inconsistências entre artefatos.

## Convenções

- **IDs**: `F-NNN` para features, `E-NNN` para epics, `ADR-NNN` para decisões, `S-NNN` para stories (dentro de uma feature).
- **Frontmatter**: YAML mínimo. Não preencher campos que não se aplicam.
- **Idioma**: documentação em pt-BR; código e identificadores em en.
- **Datas**: ISO 8601 (`2026-05-13`).
