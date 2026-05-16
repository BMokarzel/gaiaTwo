---
id: F-013
title: Feature → Service link via PR label
status: refined
modules: [product, code, bridge]
depends_on: [F-007, F-012]
modeling_impact: no
adrs: []
epic: E-005
updated: 2026-05-13
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

- [ ] Dado PR merged com label `feature:urn:ce:internal::feature/checkout-redesign`
      tocando arquivos em repo `payments`, quando webhook chega,
      então edge `Realizes(checkout-redesign → payments-service)` é
      criado.
- [ ] Webhook chega 2x para o mesmo PR: idempotente, sem duplicidade.
- [ ] PR sem label `feature:` é ignorado silenciosamente (log info).
- [ ] PR com label `feature:` mas URN inexistente: erro retornado +
      registro em fila de "pendentes" para futura resolução.
- [ ] HMAC inválido retorna 401.
- [ ] PR tocando 3 services cria 3 edges para a mesma feature.

## Riscos / incerteza

- **Path → Service.** Convenção é frágil em mono-repos sem manifesto.
  Reuso da convenção decidida em F-007/F-011.
- **Labels múltiplos.** Cria 1 edge por feature label. Sem limite no
  MVP.
- **Backfill.** PRs antigos exigem job dedicado. Fora do escopo.
- **Webhook reliability.** Aceitar perda (GitHub não garante delivery
  100%). Mitigação futura: replay via API GitHub.

## Notas de implementação

- Handler em `internal/modules/product/webhook/github/`.
- Verificação HMAC SHA-256.
- Idempotência via `(pr_url, feature_urn, service_urn)`.
