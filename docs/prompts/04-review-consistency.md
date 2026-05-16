# Prompt 04 — Review Consistency

> Varre toda a pasta `docs/` (e idealmente o `GRAPH_REPORT.md` do
> graphify) procurando órfãos, refs mortas, inconsistências entre
> arquitetura/módulos/features e drift entre doc e código.

## Quando invocar
- Toda semana, ou antes de iniciar planejamento de próximo ciclo.
- Após mudança grande em arquitetura.
- Quando o usuário disse "revisa consistência" / "tem buraco?".

## Entrada esperada
- Acesso a toda a pasta `docs/`.
- Se existir, `GRAPH_REPORT.md` na raiz do projeto (produzido por
  graphify).
- Opcionalmente, acesso ao código para checar drift.

## Checks

### 1 — Órfãos
- [ ] Cada Kind mencionado em `01-modeling.md` está em algum módulo?
- [ ] Cada Kind em módulo está em `01-modeling.md`?
- [ ] Cada edge na adjacency matrix tem uso documentado em
      algum módulo ou feature?
- [ ] Cada feature em estado `refined`/`ready`/`in_progress` tem
      módulo declarado em `modules:`?
- [ ] Cada módulo tem doc em `docs/modules/`?

### 2 — Referências mortas
- [ ] Links relativos em todos os docs apontam para arquivos
      existentes?
- [ ] IDs de feature/epic/ADR referenciados existem como arquivo?
- [ ] `depends_on:` aponta para features que existem?
- [ ] `adrs:` aponta para ADRs existentes?

### 3 — Drift arquitetura ↔ módulo
- [ ] Princípios em `architecture/00-overview.md §"Princípios"` são
      respeitados pelos módulos?
- [ ] Módulos não introduzem conceito ausente da arquitetura sem
      ADR correspondente.

### 4 — Drift doc ↔ código (se acesso a código)
- [ ] Cada Kind documentado tem struct/type no código?
- [ ] Cada edge documentado está na adjacency matrix do código?
- [ ] Cada interface pública documentada tem implementação?
- [ ] Reverso: structs/types no código têm contrapartida no doc?

### 5 — Estado de features
- [ ] Features `in_progress` por mais de 2 semanas → alertar staleness.
- [ ] Features `blocked` → o motivo ainda existe?
- [ ] Features `refining` há muito tempo → empurrar para `refined` ou
      voltar para `idea`.

### 6 — Backlog
- [ ] `priorities.md` reflete features em estado `ready` ou
      `in_progress`?
- [ ] `epics.md` agrupa features sem deixar features órfãs?

### 7 — Frontmatter
- [ ] Campos obrigatórios presentes (id, title, status, updated)?
- [ ] `updated` nas features tocadas no último ciclo está fresco?

## Saída

```markdown
# Consistency Review — YYYY-MM-DD

## Resumo
- N órfãos
- M refs mortas
- K drifts
- J features stale

## Detalhe

### Órfãos
- [ ] Kind `Foo` em modules/cost.md sem doc em 01-modeling.md
- ...

### Refs mortas
- [ ] F-007 referencia ADR-009, que não existe
- ...

### Drift arquitetura/módulo
- ...

### Drift doc/código
- ...

### Features stale
- ...

## Ações sugeridas (priorizadas)
1. **[crítico]** Resolver refs mortas — bloqueia agente
2. **[alto]** Decidir sobre features stale (X, Y)
3. **[médio]** Atualizar módulo Z para refletir feature F-NNN concluída
4. **[baixo]** Cosméticos de frontmatter
```

## Uso com graphify

Se `GRAPH_REPORT.md` existe, **começar lendo-o**. Ele cobre:
- Mapa de entidades extraídas
- Comunidades detectadas (Leiden) — bom para spot de fragmentação
- "Surprising connections" — possíveis vazamentos de boundary

O prompt 04 *complementa* graphify validando o que o grafo derivado
não consegue inferir (estados de feature, frontmatter, frescor).

## Heurísticas

- **Mais de 10 órfãos** = sistema fora de controle, parar de adicionar
  feature até resolver.
- **Drift de código** que existe há mais de 1 ciclo = atualizar doc
  *antes* de próxima feature naquele módulo.
- **Features `refining` há 3+ semanas** sem progresso = candidatas a
  arquivar (estado `dropped` ou deleção).
