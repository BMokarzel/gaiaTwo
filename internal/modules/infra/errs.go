package infra

import (
	"fmt"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Erros tipados do módulo infra. Mesmo padrão de org/code (F-016 ADR-005):
// cada tipo implementa `core/errs.HTTPProblem` (Error/HTTPStatus/Code/
// Title/Details) e Unwrap aponta para o sentinel apropriado de
// `repository`.

// ErrAccountNotFound: Account não existe (corrente ou em AsOf).
type ErrAccountNotFound struct {
	URN node.URN
}

func (e *ErrAccountNotFound) Error() string {
	return fmt.Sprintf("infra: account not found: %s", e.URN)
}
func (e *ErrAccountNotFound) HTTPStatus() int { return 404 }
func (e *ErrAccountNotFound) Code() string    { return "infra.account.not_found" }
func (e *ErrAccountNotFound) Title() string   { return "Account not found" }
func (e *ErrAccountNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrAccountNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrRegionNotFound: Region não existe.
type ErrRegionNotFound struct {
	URN node.URN
}

func (e *ErrRegionNotFound) Error() string {
	return fmt.Sprintf("infra: region not found: %s", e.URN)
}
func (e *ErrRegionNotFound) HTTPStatus() int { return 404 }
func (e *ErrRegionNotFound) Code() string    { return "infra.region.not_found" }
func (e *ErrRegionNotFound) Title() string   { return "Region not found" }
func (e *ErrRegionNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrRegionNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrComputeNotFound: Compute não existe.
type ErrComputeNotFound struct {
	URN node.URN
}

func (e *ErrComputeNotFound) Error() string {
	return fmt.Sprintf("infra: compute not found: %s", e.URN)
}
func (e *ErrComputeNotFound) HTTPStatus() int { return 404 }
func (e *ErrComputeNotFound) Code() string    { return "infra.compute.not_found" }
func (e *ErrComputeNotFound) Title() string   { return "Compute not found" }
func (e *ErrComputeNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrComputeNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrPersistenceNotFound: Persistence não existe.
type ErrPersistenceNotFound struct {
	URN node.URN
}

func (e *ErrPersistenceNotFound) Error() string {
	return fmt.Sprintf("infra: persistence not found: %s", e.URN)
}
func (e *ErrPersistenceNotFound) HTTPStatus() int { return 404 }
func (e *ErrPersistenceNotFound) Code() string    { return "infra.persistence.not_found" }
func (e *ErrPersistenceNotFound) Title() string   { return "Persistence not found" }
func (e *ErrPersistenceNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrPersistenceNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrNetworkNotFound: Network não existe.
type ErrNetworkNotFound struct {
	URN node.URN
}

func (e *ErrNetworkNotFound) Error() string {
	return fmt.Sprintf("infra: network not found: %s", e.URN)
}
func (e *ErrNetworkNotFound) HTTPStatus() int { return 404 }
func (e *ErrNetworkNotFound) Code() string    { return "infra.network.not_found" }
func (e *ErrNetworkNotFound) Title() string   { return "Network not found" }
func (e *ErrNetworkNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrNetworkNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrInvalidURN: URN resolve mas referencia Kind incompatível com o
// endpoint (ex.: GET /v1/infra/computes/{urn} apontando para Region).
// Também usado para URNs malformadas ou Q de busca vazia.
type ErrInvalidURN struct {
	URN    node.URN
	Reason string
}

func (e *ErrInvalidURN) Error() string {
	return fmt.Sprintf("infra: invalid URN %q: %s", e.URN, e.Reason)
}
func (e *ErrInvalidURN) HTTPStatus() int { return 400 }
func (e *ErrInvalidURN) Code() string    { return "infra.urn.invalid" }
func (e *ErrInvalidURN) Title() string   { return "Invalid URN" }
func (e *ErrInvalidURN) Details() map[string]any {
	return map[string]any{"urn": e.URN, "reason": e.Reason}
}
func (e *ErrInvalidURN) Unwrap() error { return repository.ErrInvalidArgument }
