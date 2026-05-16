// Package code é o módulo do **code plane** (F-007).
//
// Extrai nós Service/Endpoint/Function de repositórios de código-fonte
// e os escreve no grafo via `repository.NodeRepository` (+ edges
// `DEFINED_IN` via `EdgeRepository`).
//
// Subpacotes:
//
//   - `collector/golang` — extractor para Go (`go/parser` + `go/ast`).
//     Detecta `go.mod` → Service; handlers HTTP (`net/http`, `chi`) →
//     Endpoint; funções exportadas em pacotes service/handler/usecase →
//     Function.
//
//   - `service` — orquestrador (wiring entre collector → repository),
//     análogo a `modules/infra/service`.
//
// Imports permitidos: `entity/node`, `entity/edge`, `repository`.
// NÃO importa `modules/infra/*` ou `modules/cost/*`.
package code
