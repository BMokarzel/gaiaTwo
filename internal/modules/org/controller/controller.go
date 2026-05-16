// Package controller é o controller HTTP do módulo org (F-016 S-005).
//
// Recebe uma `org.Service` por construtor e registra os endpoints
// /v1/teams, /v1/squads, /v1/people no mux compartilhado. Handlers
// são "dumb": parseiam query string, delegam para a service, e
// serializam o retorno via helpers em views.go.
//
// Erros vão para `httpserver.WriteError`, que delega a `errs.Render`.
// Os tipos `org.ErrTeamNotFound`, etc., já implementam HTTPProblem;
// não há switch de erro aqui.
//
// Imports permitidos: stdlib, `internal/entity/*`, `internal/modules/org`
// (irmão do controller, mesmo bounded context), `internal/platform/httpserver`.
// NÃO importa `internal/repository` direto (ADR-005).
package controller

import (
	"net/http"

	"costEngine/internal/modules/org"
)

// Controller agrega o `org.Service` e satisfaz `httpserver.Registrar`.
// Mesma struct para todos os handlers (teams.go, squads.go, people.go) —
// arquivos divididos por path apenas para legibilidade.
type Controller struct {
	svc org.Service
}

// New cria o Controller. svc nil dispara panic — fail-fast em wire-up.
func New(svc org.Service) *Controller {
	if svc == nil {
		panic("org/controller: New requires non-nil service")
	}
	return &Controller{svc: svc}
}

// Register satisfaz `httpserver.Registrar` registrando todos os
// endpoints do módulo no mux compartilhado.
//
// Padrões Go 1.22+ (`{rest...}`) capturam URNs com `/` e `:`. O
// dispatch interno (dispatchTeam etc.) reparte por sufixo conhecido
// (`/squads`, `/members`, `/reports`).
func (c *Controller) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/teams", c.handleListTeams)
	mux.HandleFunc("GET /v1/teams/{rest...}", c.dispatchTeam)
	mux.HandleFunc("GET /v1/squads/{rest...}", c.dispatchSquad)
	mux.HandleFunc("GET /v1/people/{rest...}", c.dispatchPerson)
}
