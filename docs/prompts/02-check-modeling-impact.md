# Prompt 02 — Check Modeling Impact

> Recebe uma feature em `refining`/`refined` e avalia se ela exige
> mudança nos docs de arquitetura ou na modelagem do grafo. Produz
> parecer + rascunhos.

## Quando invocar
- Feature acabou de ser refinada e tem `modeling_impact: yes` ou
  `unknown`.
- Usuário disse "essa mexe na modelagem?" ou "valida o impacto".

## Entrada esperada
- Caminho da feature.
- Acesso a `docs/architecture/01-modeling.md` e ao módulo principal
  da feature em `docs/modules/`.

## Processo

### Passo 1 — Inventariar o que a feature pede
Ler a seção "Toque no grafo" da feature. Listar:
- Kinds lidos
- Kinds escritos
- Kinds candidatos a novos
- Edges candidatos a novos
- Propriedades novas em Kind existente

### Passo 2 — Checar contra a modelagem atual

Para cada item:

| Item | Existe em `01-modeling.md`? | Existe no módulo? | Status |
|---|---|---|---|
| Kind X | sim/não | sim/não | adequa / falta / divergente |

### Passo 3 — Classificar o impacto

| Categoria | Significa | Ação |
|---|---|---|
| **No impact** | tudo já modelado | feature segue para stories |
| **Property only** | só novos campos em Kind existente | atualizar módulo doc |
| **New edge** | par (FromKind, ToKind) novo | atualizar adjacency matrix em `01-modeling.md` |
| **New Kind** | conceito novo no plano | atualizar `01-modeling.md` + módulo + provavelmente ADR |
| **New plane** | nada existente cobre | ADR obrigatória; pode disparar nova feature de modelagem |
| **Divergência** | feature contradiz arquitetura | resolver ANTES; pode exigir ADR de mudança |

### Passo 4 — Rascunhar o que precisa mudar

**Se `No impact`:** marcar `modeling_impact: no` na feature, fechar.

**Se `Property only`:**
- Listar propriedades novas e tipo.
- Apontar arquivo do módulo onde inserir.

**Se `New edge`:**
- Especificar `(FromKind, ToKind, EdgeType)`.
- Atualizar adjacency matrix em `01-modeling.md`.
- Apontar se vive no módulo do plano ou em `bridge/`.

**Se `New Kind` ou `New plane`:**
- Rascunho de seção em `01-modeling.md`: nome, URN pattern, campos,
  Resource interface se aplicável.
- Rascunho de ADR em `docs/architecture/decisions/ADR-NNN-<titulo>.md`
  usando template abaixo.
- Atualizar `00-overview.md` se for plano novo.

**Se `Divergência`:**
- Apontar exatamente onde feature contradiz arquitetura.
- Apresentar duas opções:
  - (a) ajustar feature para alinhar
  - (b) abrir ADR propondo mudança da arquitetura
- **Não decidir sozinho.** Devolver ao usuário.

### Passo 5 — Atualizar feature

```yaml
modeling_impact: no       # ou yes — descrever resumo na seção
adrs: [ADR-NNN]          # se aplicável
```

Adicionar à feature uma seção:

```markdown
## Impacto na modelagem
**Classe:** No impact | Property only | New edge | New Kind | New plane | Divergência
**Mudanças propostas:**
- `architecture/01-modeling.md`: <resumo>
- `modules/<plano>.md`: <resumo>
- ADR-NNN: <título> (status: proposed)
```

## Template de ADR

```markdown
---
id: ADR-NNN
title: <decisão em uma linha>
status: proposed         # proposed → accepted → superseded
date: YYYY-MM-DD
related_features: [F-NNN, ...]
---

# ADR-NNN — <título>

## Contexto
<o que motivou esta decisão, 1-3 parágrafos>

## Decisão
<o que decidimos, em prosa direta>

## Alternativas consideradas
- A: <alternativa>. Descartada porque ...
- B: <alternativa>. Descartada porque ...

## Consequências
**Positivas:**
- ...
**Negativas / custo:**
- ...
**Quando reabrir:**
- ...
```

## Heurísticas

- **3+ Kinds novos numa única feature** = provavelmente a feature é
  uma iniciativa, não uma feature. Sugerir quebra.
- **Nenhuma ADR existe ainda e estamos no MVP**: pode ser saudável
  forçar ADR-001 retroativa só para registrar uma decisão importante
  prévia (ex.: "monolito modular inicial").
- **Divergência detectada**: nunca aplicar mudança em arquitetura sem
  ADR. Arquitetura é constituição.
