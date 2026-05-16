// Package service_compute implementa F-009: ponte Service→Compute via
// edge RUNS_ON ([ADR-004]).
//
// Pipeline:
//
//  1. Para cada Compute current na conta-alvo, aplica estratégias de
//     resolução em ordem (tag > name_convention > manifest). A primeira
//     que produz um Service válido vence; demais são ignoradas.
//  2. Compara o resultado com o edge RUNS_ON corrente (se existe):
//     - match: noop.
//     - mismatch (dono mudou): fecha edge antigo (valid_to=now) e abre
//       um novo com source/confidence da estratégia que resolveu.
//     - ausência → edge novo: open.
//     - ausência → nada: orphan (não emite edge).
//  3. Service ambíguo (mesmo nome em múltiplos URNs) → unresolved
//     (não emite, registra em Output.Ambiguous).
//
// O pacote não conhece Neo4j: opera sobre uma interface BridgeRepo
// trocável. Determinismo: a ordem de iteração de Computes é por URN
// ordenada (testabilidade).
//
// [ADR-004]: docs/architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md
package service_compute
