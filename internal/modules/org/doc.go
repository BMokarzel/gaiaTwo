// Package org é o módulo do **org plane** (F-010+).
//
// Modela hierarquia organizacional (Person/Team/Squad) e a liga ao
// resto do grafo (futuro F-011 ownership, F-013 feature↔service).
//
// Subpacotes:
//
//   - `ingest/hris` — ingestão a partir de CSV mínimo (F-010).
//     Schema: email,name,role,team,squad,manager_email,start_date,
//     end_date_or_blank.
//   - `service` — orquestrador (parser → emitters → repository),
//     análogo a `modules/code/service` e `modules/infra/service`.
//
// Imports permitidos: `entity/node`, `entity/edge`, `repository`.
// NÃO importa `modules/infra/*`, `modules/cost/*` ou `modules/code/*`.
package org
