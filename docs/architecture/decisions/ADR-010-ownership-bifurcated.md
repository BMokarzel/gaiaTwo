---
id: ADR-010
title: Ownership bifurcada por granularidade do nó (Team vs Person)
status: accepted
date: 2026-05-17
supersedes: []
superseded_by: []
related: [ADR-009]
---

# ADR-010 — Ownership bifurcada por granularidade do nó (Team vs Person)

## Contexto

`OWNED_BY` é a edge que liga código a quem mantém. Há duas decisões
correlatas:

1. **Quem é o owner?** Team, Squad, Person, ou algo intermediário?
2. **Em que nível ancorar?** Todo nó do code plane, só os de alto-nível,
   só os folhas?

Modelar Team/Squad/Tribe como kinds separados explode o schema sem
ganho semântico (vocabulário organizacional varia entre empresas).
Modelar Person como único kind perde a granularidade conceitual (um
Service tem múltiplos autores ao longo do tempo).

## Decisão

**Ownership bifurca por granularidade do nó:**

| Nó do code plane | `OWNED_BY` aponta para |
|------------------|------------------------|
| `Service`, `Module`, `Endpoint` | `Team` |
| `Function`, `Call`, `Type`, `Variable` | `Person` |
| `Framework`, `License`, `SecurityAdvisory` | — (entidades externas) |

| Nó do governance plane | `OWNED_BY` |
|------------------------|------------|
| `Feature`, `Epic` | `Team` |
| `UserStory` | `Person` (via `ASSIGNED_TO`) |
| `Capability`, `Domain`, `BusinessArea`, `Company` | — (categorização, não ownership; o eixo organizacional já entrega isso via `CONTAINS`) |

**Critério emergente (não-constraint, vale como guia):**
> Nó representa **intenção** (negócio, contrato, agregação) → `-> Team`.
> Nó representa **execução** (linha, call, tipo concreto) → `-> Person`.
> Cutoff em `Module ⇄ Function`.

**Estrutura organizacional:**

```
Company ──CONTAINS──▶ BusinessArea ──CONTAINS──▶ Team ──CONTAINS──▶ Person ──HAS_ROLE──▶ Role
                                                          ▲
                                                          │ LED_BY (opcional)
                                                        Person
```

- `Team` é único kind organizacional. Não há `Squad`, `Tribe`, `Crew`
  como kinds separados — vocabulário varia, semântica não. Atributo
  `Team.kind` opcional (`team`/`squad`/`tribe`/`guild`) cobre o caso.
- `Role` é nó por combinação **track + level**: `backend-junior`,
  `backend-senior`, `frontend-tech-lead`, `qa-staff`, `pm`. Múltiplos
  Persons → mesmo Role. Atributos: `track`, `level`, `is_leadership`.
- `Person -HAS_ROLE-> Role` cardinalidade 1 (uma role corrente;
  bitemporal cobre promoções).

**Dois eixos de ownership podem divergir e isso é desejado:**
- A `Feature:home-banner` é `OWNED_BY Team:marketing` (negócio).
- A `Call` que renderiza o banner é `OWNED_BY Person:alice@platform` (execução).
- O `Endpoint` que expõe a rota é `OWNED_BY Team:platform` (manutenção do contrato).

Auditoria de governança usa essa divergência como input
(*"qual o gap entre quem entrega valor e quem mantém o código?"*).

## Consequências

**Positivas:**
- Schema enxuto: 2 tipos organizacionais (`Team`, `Person`) + `Role`.
- Cardinalidade `OWNED_BY` razoável (cada nó tem 1 dono; um Team tem
  dezenas a centenas de filhos).
- Reorganização org (Team renomeia, fundindo squads) é local — só os
  edges `OWNED_BY` afetados rebumpam versão; o resto permanece.
- Sucessão de propriedade (pessoa sai) tem caminho claro: edge fecha,
  nova edge abre. Bitemporal preserva quem era dono antes.

**Negativas:**
- A bifurcação não é constraint hard — coletor pode acidentalmente
  produzir `Function -OWNED_BY-> Team`. Tratado como lint, não erro.
- Pessoas com múltiplos roles em times diferentes não são representáveis
  no MVP. Solução prevista: introduzir `Membership` como nó
  intermediário (`Person -PART_OF-> Membership -IN-> Team -AS-> Role`)
  quando demanda concreta surgir.

## Fontes de população

- **`Team`/`Person`/`Role`**: HRIS sync (F-010, estendido) ou CRUD
  manual.
- **`OWNED_BY` para Service/Module/Endpoint**: CODEOWNERS (F-011) +
  manifest do projeto.
- **`OWNED_BY` para Function/Call/Type/Variable**: `git blame` do último
  committer significativo no range de linhas. Heurística refinável.
- **`OWNED_BY` para Feature/Epic**: CRUD manual via API (extensão de
  F-012) ou sync com tracker (Linear, Jira).
- **`ASSIGNED_TO` para UserStory**: sync com tracker.

## Alternativas consideradas

**Team/Squad/Tribe como kinds separados.** Rejeitada: cria 3 edges
duplicadas (`OWNED_BY -> Team`, `-> Squad`, `-> Tribe`) e força decidir
taxonomia de RH antes de ter problema concreto.

**Tudo `-> Team` (sem `-> Person` no plane fino).** Rejeitada: perde
granularidade real. Função tem committer identificável; chamar isso de
"propriedade do time" mascara responsabilidade individual.

**Tudo `-> Person` (sem Team em código grosso).** Rejeitada: Service é
multi-autor por natureza; atribuir a uma pessoa só seria injusto e
volátil.

**`Membership` como nó intermediário desde já.** Considerada para
suportar multi-team-role. Rejeitada por over-engineering no MVP: 90% dos
casos são single-team-role; quando o caso real aparecer, evolução é
aditiva.
