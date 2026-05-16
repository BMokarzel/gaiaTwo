# Prompt 03 — Slice Into Stories

> Pega uma feature em `refined` e a fatia em stories verticais
> entregáveis. Cada story descreve comportamento, não implementação.

## Quando invocar
- Feature está `refined` e o usuário disse "fatia em stories" ou
  "como entregar isso".

## Entrada esperada
- Caminho da feature em `docs/features/F-NNN-*.md`.

## Princípios

1. **Slice vertical.** Cada story atravessa as camadas necessárias
   (UI → API → repo → DB) para entregar comportamento externo.
2. **Independente quando possível.** Se duas stories são sequenciais
   por força maior, marcar `blocked_by`.
3. **Pequena.** Se a story passa de "~3 dias de trabalho", quebrar
   mais. Se passa de 5, definitivamente quebrar.
4. **Critério em termos observáveis.** "Dado/quando/então". Não
   "implementar repository X".
5. **Nunca mais de 8 stories por feature.** Se exceder, a feature é
   na verdade um épico — sugerir reagrupar.

## Processo

### Passo 1 — Identificar valor mínimo
Qual é a primeira coisa que, se entregue isolada, já dá feedback útil
para usuário/operador? Essa vira S-001.

### Passo 2 — Identificar caminhos paralelos
Há trabalho que pode ser feito independente (ex.: backend repo +
front-end mock; coletor de dados + UI separados)? Marcar como
candidatos a paralelizar.

### Passo 3 — Identificar dependências forçadas
Há algo que só faz sentido depois de outra (ex.: filtros só fazem
sentido depois de listagem)? Marcar `blocked_by`.

### Passo 4 — Listar stories

Para cada story:

```markdown
### S-NNN — <título curto>
**Comportamento:** Dado <contexto>, quando <ação>, então <resultado observável>.
**Camadas tocadas:** [api, repo, ui, docs, ...]
**Blocked by:** [S-XXX, ...]  # se aplicável
**Notas:** <opcional>
```

### Passo 5 — Identificar "story zero"
Toda feature precisa de uma story de **scaffold** (estrutura de pacote,
endpoint vazio, migration vazia) que custa pouco e desbloqueia o resto.
Se ainda não existe, criar S-001 como scaffold.

### Passo 6 — Sanity check
- [ ] Todas as stories têm critério observável (não interno)?
- [ ] A soma das stories cobre todos os critérios de aceite da feature?
- [ ] Nenhuma story descreve implementação ("usar Neo4j tx", "criar
      struct X")?
- [ ] Total ≤ 8 stories?
- [ ] Cada story poderia ser deployada/demonstrada isoladamente?

## Saída — append na feature

Adicionar à feature uma seção:

```markdown
## Stories

### S-001 — <título>
**Comportamento:** ...
**Camadas tocadas:** ...

### S-002 — ...
```

Atualizar frontmatter:
```yaml
status: ready    # ← era refined
```

## Heurísticas

- **Story 1 = caminho feliz, mínimo, sem auth, sem multi-tenant.**
  Adições viram stories próprias.
- **UI vem em story própria** quando há UX significativa; senão,
  juntar com a story de API.
- **Migration de schema é story** (em sistemas com schema versionado).
- **Observabilidade (log/métrica/trace)** geralmente vira nota em cada
  story, não story própria — exceto se for instrumentação dedicada.
- **Stories com `blocked_by` em mais de 2 outras** = sinal de fatiamento
  errado. Tentar achar slice paralelo.
