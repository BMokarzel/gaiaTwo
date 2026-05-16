package code

import (
	"fmt"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Erros tipados do módulo code. Mesmo padrão do módulo org (F-016 S-004):
// cada tipo implementa `core/errs.HTTPProblem` (Error/HTTPStatus/Code) e
// Unwrap aponta para o sentinel apropriado de `repository`.

// ErrServiceNotFound: Service não existe (corrente ou em AsOf).
type ErrServiceNotFound struct {
	URN node.URN
}

func (e *ErrServiceNotFound) Error() string {
	return fmt.Sprintf("code: service not found: %s", e.URN)
}
func (e *ErrServiceNotFound) HTTPStatus() int { return 404 }
func (e *ErrServiceNotFound) Code() string    { return "code.service.not_found" }
func (e *ErrServiceNotFound) Title() string   { return "Service not found" }
func (e *ErrServiceNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrServiceNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrEndpointNotFound: Endpoint não existe.
type ErrEndpointNotFound struct {
	URN node.URN
}

func (e *ErrEndpointNotFound) Error() string {
	return fmt.Sprintf("code: endpoint not found: %s", e.URN)
}
func (e *ErrEndpointNotFound) HTTPStatus() int { return 404 }
func (e *ErrEndpointNotFound) Code() string    { return "code.endpoint.not_found" }
func (e *ErrEndpointNotFound) Title() string   { return "Endpoint not found" }
func (e *ErrEndpointNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrEndpointNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrFunctionNotFound: Function não existe.
type ErrFunctionNotFound struct {
	URN node.URN
}

func (e *ErrFunctionNotFound) Error() string {
	return fmt.Sprintf("code: function not found: %s", e.URN)
}
func (e *ErrFunctionNotFound) HTTPStatus() int { return 404 }
func (e *ErrFunctionNotFound) Code() string    { return "code.function.not_found" }
func (e *ErrFunctionNotFound) Title() string   { return "Function not found" }
func (e *ErrFunctionNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN}
}
func (e *ErrFunctionNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrInvalidURN: URN resolve mas referencia Kind incompatível com o
// endpoint (ex.: GET /v1/code/services/{urn} apontando para uma
// Function). Também usado para URNs malformadas ou Q de busca vazia.
type ErrInvalidURN struct {
	URN    node.URN
	Reason string
}

func (e *ErrInvalidURN) Error() string {
	return fmt.Sprintf("code: invalid URN %q: %s", e.URN, e.Reason)
}
func (e *ErrInvalidURN) HTTPStatus() int { return 400 }
func (e *ErrInvalidURN) Code() string    { return "code.urn.invalid" }
func (e *ErrInvalidURN) Title() string   { return "Invalid URN" }
func (e *ErrInvalidURN) Details() map[string]any {
	return map[string]any{"urn": e.URN, "reason": e.Reason}
}
func (e *ErrInvalidURN) Unwrap() error { return repository.ErrInvalidArgument }
