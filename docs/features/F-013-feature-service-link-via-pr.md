---
id: F-013
title: Feature → Service link via PR label
status: done
modules: [bridge, code, gov]
depends_on: [F-007, F-012]
modeling_impact: yes
adrs: []
epic: E-005
updated: 2026-05-17
---

# F-013 — Feature → Service link via PR label

## Problema

Saber qual `Service` realiza qual `Feature` é a peça que conecta
produto a código (e, transitivamente, a custo). Fazer isso manualmente
não escala. PRs já carregam essa informação implicitamente — labels
e arquivos tocados. Capturar via webhook do GitHub é barato e
auto-mantém.

**Para quem:** allocation por feature/capability, produto
Arquitetura, produto Dashboard.

**Dor sem ela:** feature fica como nó solto; impossível responder
"quanto custou entregar feature X?".

## Escopo

**Inclui:**
- Endpoint `POST /v1/webhooks/github` recebendo eventos `pull_request`
  com `action=closed` e `merged=true`.
- Extração de label `feature:<feature_urn>` ou
  `feature/<feature_name>` do PR.
- Para cada arquivo tocado, resolve `path → Service` (via convenção
  repo+path → service URN definida em F-007/F-011).
- Cria edge `Realizes(Feature → Service)` se ainda não existe; reforça
  `confidence` se já existe.
- Auditoria: cada edge guarda `pr_url`, `merged_at` em `source`.

**NÃO inclui:**
- Outros providers (GitLab, Bitbucket — features dedicadas).
- Inferência sem label (ex.: heurística por título de PR).
- Decay de confidence ao longo do tempo (futuro).
- Reconciliação retroativa de PRs antigos (apenas adiante; backfill
  opcional via job).

**Precondições:**
- Webhook configurado no GitHub apontando para o endpoint.
- Secret HMAC compartilhado.
- F-007 (Service URN resolvível por path).
- F-012 (Feature URN existe).

## Toque no grafo

- **Lê:** `Service`, `Feature`.
- **Escreve:** edge `Realizes(Feature → Service)` com
  `source={provider:"github", pr_url:..., merged_at:...}`.
- **Novos Kinds/edges:** edge `Realizes` já modelado em
  `03-business.md`.
- **Bitemporal:** edge versionado por (feature_urn, service_urn);
  reabrir merge cria nova versão (re-merge não acontece geralmente,
  mas idempotência cobre).

## Critérios de aceite

- [x] Dado PR merged com label `feature:urn:ce:gov:acme:feature/checkout-redesign`
      tocando arquivos em repo `acme/api`, quando webhook chega,
      então edge `Realizes(checkout-redesign → acme/api:services/billing)`
      é criado (longest-prefix-match em `Service.ModulePath`).
- [x] Webhook chega 2x para o mesmo PR: idempotente via
      `DeterministicID(from, REALIZES, to, mergedAt)`.
- [x] PR sem label `feature:` é ignorado (200 + `{ignored:true}`).
- [x] PR com label `feature:` mas URN inexistente: 400
      `feature_unresolved` (fila de pendentes é backlog explícito —
      não MVP).
- [x] HMAC inválido retorna 401.
- [x] PR tocando N services distintos cria N edges para a mesma feature.

## Estado de implementação (2026-05-17)

- `internal/entity/edge`: `TypeRealizes`, struct `Realizes`, adjacency
  `Feature→Service` e teste smoke adicionados.
- `internal/modules/bridge/github_webhook/`: pacote completo
  (`doc.go`, `hmac.go`, `payload.go`, `resolver.go`, `service.go`,
  `controller.go`) com `POST /v1/webhooks/github`.
- `cmd/api`: flag `--github-webhook-secret` (env
  `CE_GITHUB_WEBHOOK_SECRET`) habilita o registrar. Vazio mantém
  endpoint desligado (fail-closed).
- Testes: HMAC (happy + 6 falhas), payload parse (happy / ignored /
  malformed), resolver (longest-prefix, sem fallback raiz, distinct
  services), service.Ingest (idempotente, feature ausente, no-op),
  controller end-to-end (HMAC bad, happy, evento não-PR).

## Backlog vivo

- Fila de pendentes para Features ausentes (replay quando a URN
  aparece no grafo).
- Hidratação automática da lista de arquivos via GitHub REST API
  (hoje o payload assume `files[]` enriquecido por proxy).
- Suporte a múltiplos providers (GitLab/Bitbucket).
- Decay de confidence ao longo do tempo.

## Riscos / incerteza

- **Path → Service.** Convenção é frágil em mono-repos sem manifesto.
  Reuso da convenção decidida em F-007/F-011.
- **Labels múltiplos.** Cria 1 edge por feature label. Sem limite no
  MVP.
- **Backfill.** PRs antigos exigem job dedicado. Fora do escopo.
- **Webhook reliability.** Aceitar perda (GitHub não garante delivery
  100%). Mitigação futura: replay via API GitHub.

## Notas de implementação

- Handler em `internal/modules/bridge/github_webhook/` (plano `bridge`
  porque cruza gov ↔ code; `product` foi descartado — não há plano
  "product" no monolito modular atual).
- Verificação HMAC SHA-256 com `crypto/subtle.ConstantTimeCompare`.
- Idempotência via `DeterministicID(feature, REALIZES, service, mergedAt)`
  no nível do `EdgeRepository` (mais simples que carregar tupla
  `(pr_url, …)` como chave externa).
