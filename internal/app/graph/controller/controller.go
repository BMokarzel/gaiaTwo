// Package controller é o controller cross-kind do "graph plane"
// (F-016 S-008). Hospeda os endpoints `/v1/architecture/*` que operam
// sobre qualquer Kind do grafo: GET de um nó, history, neighbors, paths.
//
// EXCEÇÃO à regra "controller não importa repository" (ADR-005):
// graph é um thin wrapper sobre `NodeRepository`/`EdgeRepository` —
// não tem lógica de negócio acima do grafo. Endpoints de domínio
// (por plano) vão pelos `modules/*/controller` correspondentes; aqui
// fica apenas o que é genuinamente polimórfico.
//
// Imports permitidos: stdlib, `internal/entity/*`, `internal/repository`,
// `internal/platform/httpserver`.
package controller

import (
	"net/http"

	"costEngine/internal/repository"
)

// Controller agrega os repos e satisfaz `httpserver.Registrar`.
type Controller struct {
	nodes repository.NodeRepository
	edges repository.EdgeRepository
}

// New cria o Controller. Repos nil disparam panic — fail-fast em wire-up.
func New(nodes repository.NodeRepository, edges repository.EdgeRepository) *Controller {
	if nodes == nil || edges == nil {
		panic("graph/controller: New requires non-nil repos")
	}
	return &Controller{nodes: nodes, edges: edges}
}

// Register satisfaz `httpserver.Registrar` registrando os endpoints
// cross-kind do graph plane.
//
// `{rest...}` (Go 1.22+) captura URNs com `/` e `:`; o dispatch
// interno (`dispatchNode`) reparte por sufixo (`/neighbors`, `/history`).
func (c *Controller) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/architecture/nodes/{rest...}", c.dispatchNode)
	mux.HandleFunc("GET /v1/architecture/paths", c.handlePaths)
}
