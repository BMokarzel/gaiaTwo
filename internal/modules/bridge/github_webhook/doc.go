// Package github_webhook implementa o endpoint
// `POST /v1/webhooks/github` (F-013) — bridge gov→code via PR merges.
//
// Fluxo:
//
//  1. GitHub envia evento `pull_request` quando um PR é fechado.
//  2. Verifica-se a assinatura HMAC SHA-256 do header
//     `X-Hub-Signature-256` (secret compartilhado).
//  3. Se `action=closed` e `merged=true`, extraem-se:
//     - URN da Feature a partir da label `feature:<urn>`;
//     - Lista de arquivos tocados (via `pull_request.changed_files` +
//     campo `files` do payload custom — webhook configurado com
//     "Send me everything").
//  4. Cada path é resolvido para a URN de um `Service` por
//     longest-prefix-match contra `repo.ModulePath` (lista corrente
//     em `NodeRepository`).
//  5. Para cada par (feature, service) distinto, upsert idempotente de
//     edge `Realizes(Feature → Service)` carregando `pr_url` e
//     `merged_at` em `Meta.Properties` (auditoria).
//
// **Idempotência:** `DeterministicID(from, REALIZES, to, validFrom=mergedAt)`
// — re-entregas do mesmo webhook resolvem para o mesmo ID.
//
// **Placement (ADR-001):** plano `bridge` porque cruza gov ↔ code.
// Não importa `controller` do gov nem do code; opera só sobre
// repository + entities.
package github_webhook
