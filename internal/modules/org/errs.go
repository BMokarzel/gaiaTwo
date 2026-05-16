package org

import (
	"fmt"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Erros tipados do módulo org. Cada um implementa
// `core/errs.HTTPProblem` (Error, HTTPStatus, Code) — o controller
// nunca precisa de switch.
//
// Convenções:
//   - Code segue `<modulo>.<entidade>.<situacao>` lowercase com ponto.
//   - Unwrap aponta para o sentinel correspondente de `repository`
//     para preservar errors.Is em callers legados.
//   - Details() devolve os campos não-sensíveis como extras do RFC 7807
//     body (ex.: `urn`, `reason`).

// ErrTeamNotFound: Team não existe (corrente ou em AsOf).
type ErrTeamNotFound struct {
	URN node.URN
}

func (e *ErrTeamNotFound) Error() string {
	return fmt.Sprintf("org: team not found: %s", e.URN)
}
func (e *ErrTeamNotFound) HTTPStatus() int { return 404 }
func (e *ErrTeamNotFound) Code() string    { return "org.team.not_found" }
func (e *ErrTeamNotFound) Title() string   { return "Team not found" }
func (e *ErrTeamNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrTeamNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrSquadNotFound: Squad não existe.
type ErrSquadNotFound struct {
	URN node.URN
}

func (e *ErrSquadNotFound) Error() string {
	return fmt.Sprintf("org: squad not found: %s", e.URN)
}
func (e *ErrSquadNotFound) HTTPStatus() int { return 404 }
func (e *ErrSquadNotFound) Code() string    { return "org.squad.not_found" }
func (e *ErrSquadNotFound) Title() string   { return "Squad not found" }
func (e *ErrSquadNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrSquadNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrPersonNotFound: Person não existe.
type ErrPersonNotFound struct {
	URN node.URN
}

func (e *ErrPersonNotFound) Error() string {
	return fmt.Sprintf("org: person not found: %s", e.URN)
}
func (e *ErrPersonNotFound) HTTPStatus() int { return 404 }
func (e *ErrPersonNotFound) Code() string    { return "org.person.not_found" }
func (e *ErrPersonNotFound) Title() string   { return "Person not found" }
func (e *ErrPersonNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrPersonNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrInvalidURN: o URN resolve mas referencia um Kind incompatível com
// o endpoint (ex.: GET /v1/teams/{urn} apontando para uma Person).
// Também usado para URNs malformadas que não chegam ao repo.
type ErrInvalidURN struct {
	URN    node.URN
	Reason string
}

func (e *ErrInvalidURN) Error() string {
	return fmt.Sprintf("org: invalid URN %q: %s", e.URN, e.Reason)
}
func (e *ErrInvalidURN) HTTPStatus() int { return 400 }
func (e *ErrInvalidURN) Code() string    { return "org.urn.invalid" }
func (e *ErrInvalidURN) Title() string   { return "Invalid URN" }
func (e *ErrInvalidURN) Details() map[string]any {
	return map[string]any{"urn": e.URN, "reason": e.Reason}
}
func (e *ErrInvalidURN) Unwrap() error { return repository.ErrInvalidArgument }

// ErrCycleDetected: traversal em ReportsTo encontrou ciclo. Mantido
// como erro tipado para o caller decidir entre 422 (rejeitar) e
// "truncated=true" (degradar) — o controller atual escolhe truncar e
// NÃO retornar este erro; permanece exposto para futuros consumers que
// queiram tratamento estrito.
type ErrCycleDetected struct {
	Path []node.URN
}

func (e *ErrCycleDetected) Error() string {
	parts := make([]string, len(e.Path))
	for i, u := range e.Path {
		parts[i] = string(u)
	}
	return "org: cycle detected: " + strings.Join(parts, " → ")
}
func (e *ErrCycleDetected) HTTPStatus() int { return 422 }
func (e *ErrCycleDetected) Code() string    { return "org.reports.cycle" }
func (e *ErrCycleDetected) Title() string   { return "Reporting cycle detected" }
func (e *ErrCycleDetected) Details() map[string]any {
	return map[string]any{"path": e.Path}
}
func (e *ErrCycleDetected) Unwrap() error { return repository.ErrConflict }
