# Prompt 01 — Refine Feature

> Pega uma feature em `idea` (ou `refining`) e a leva para `refined`
> através de perguntas estruturadas. Saída é o próprio doc da feature
> atualizado.

## Quando invocar
- Usuário pediu "refina F-007" ou "vamos pensar em <ideia>".
- Doc da feature tem `status: idea` ou está incompleto.

## Entrada esperada
- Caminho do arquivo da feature OU descrição informal de uma ideia
  (neste caso, primeiro criar `docs/features/F-NNN-<titulo>.md` com
  número sequencial e status `idea`).

## Processo

Conduzir um diálogo curto cobrindo, **nesta ordem**:

### Bloco 1 — Razão de existir
1. Qual problema concreto esta feature resolve?
2. Para quem (que persona/produto/operador)?
3. Que dor mensurável a ausência dela causa hoje?

### Bloco 2 — Escopo
4. Qual é o **menor recorte** que entrega valor real? (Resista a expandir.)
5. O que está **fora** do escopo? Enumerar 2-4 itens explícitos.
6. Há precondições (outra feature, ADR, integração externa)?

### Bloco 3 — Tocar no grafo
7. Que entidades existentes (Kinds) são lidas?
8. Que entidades são escritas? (Atenção: só `graph services` escrevem
   no grafo do CostEngine — produtos leem.)
9. Algum Kind novo? Algum edge novo? *Se sim, marcar `modeling_impact: yes`
   e enfileirar prompt 02 ao final.*
10. Como bitemporalidade aplica? (sempre aplica; perguntar como)

### Bloco 4 — Comportamento observável
11. Que **comportamento externo** prova que está pronta?
    Listar como critérios de aceite executáveis ("dado X, quando Y,
    então Z").
12. Qual o caminho mais simples de teste manual?

### Bloco 5 — Risco e incerteza
13. Onde está a maior incerteza (técnica, de domínio, de UX)?
14. Que partes valem prototipar antes de commit total?

## Saída — preencher o doc da feature

```markdown
---
id: F-NNN
title: <título>
status: refined          # idea → refining → refined
modules: [<lista>]       # módulos tocados
depends_on: [F-XXX, ...] # outras features
modeling_impact: yes|no|unknown
adrs: []                 # ADRs relacionadas
epic: E-NNN              # se já agrupada
updated: YYYY-MM-DD
---

# F-NNN — <Título>

## Problema
<bloco 1 condensado>

## Escopo
**Inclui:**
- ...
**NÃO inclui:**
- ...
**Precondições:**
- ...

## Toque no grafo
- Lê: <Kinds>
- Escreve: <Kinds> via <módulo>
- Novos Kinds/edges: <se houver>
- Bitemporal: <como aplica>

## Critérios de aceite
- [ ] Dado ..., quando ..., então ...
- [ ] ...

## Riscos / incerteza
- ...

## Notas de implementação (opcional)
- ...
```

## Estados resultantes
- Se todas as 14 perguntas têm resposta clara → `status: refined`.
- Se 2+ ficam em aberto → `status: refining`, marcar `[CARENCIA]` nas
  seções incompletas, listar perguntas pendentes ao usuário.
- Se descobre dependência bloqueante → `status: blocked`, documentar a
  dependência em `depends_on`.

## Heurísticas

- **Se a feature tem mais de 8 critérios de aceite**, provavelmente é
  duas features. Sugerir split.
- **Se "tocar no grafo" pede 3+ Kinds novos**, é grande. Sugerir
  modeling impact antes de seguir.
- **Se "fora do escopo" está vazio**, é sinal de subdefinição. Forçar
  pelo menos 2 itens.
- **Se "comportamento observável" vira lista de tarefas técnicas**,
  reescrever em termos de efeito externo.
