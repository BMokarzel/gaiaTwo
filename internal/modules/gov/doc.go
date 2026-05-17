// Package gov é o módulo do **governance plane** (F-024).
//
// Cobre o eixo product-arch: Company → BusinessArea → Domain →
// Capability → Feature (com Feature aninhável). É read-only no MVP —
// não há pipeline de ingestão estilo HRIS aqui ainda; cadastro virá
// via CLI/UI no backlog.
//
// Princípios herdados de F-016 / ADR-005:
//   - Uma Service por módulo (Bounded Context = Module).
//   - Entidades são structs do `entity/node`.
//   - Service não conhece HTTP: retorna `gov.ErrXxx` tipados.
package gov
