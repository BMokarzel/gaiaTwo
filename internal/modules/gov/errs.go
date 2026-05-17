package gov

import (
	"fmt"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// ErrNotFound: nó governance não existe (corrente ou em AsOf). Um
// único erro tipado por kind seria repetitivo — o Kind fica em Details
// para os clientes que quiserem distinguir.
type ErrNotFound struct {
	URN  node.URN
	Kind node.Kind
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("gov: %s not found: %s", e.Kind, e.URN)
}
func (e *ErrNotFound) HTTPStatus() int { return 404 }
func (e *ErrNotFound) Code() string    { return "gov." + string(e.Kind) + ".not_found" }
func (e *ErrNotFound) Title() string   { return string(e.Kind) + " not found" }
func (e *ErrNotFound) Details() map[string]any {
	return map[string]any{"urn": e.URN, "kind": e.Kind}
}
func (e *ErrNotFound) Unwrap() error { return repository.ErrNotFound }

// ErrInvalidURN: URN resolve mas referencia outro Kind, ou é malformada.
type ErrInvalidURN struct {
	URN      node.URN
	Expected node.Kind
	Reason   string
}

func (e *ErrInvalidURN) Error() string {
	return fmt.Sprintf("gov: invalid URN %q: %s", e.URN, e.Reason)
}
func (e *ErrInvalidURN) HTTPStatus() int { return 400 }
func (e *ErrInvalidURN) Code() string    { return "gov.urn.invalid" }
func (e *ErrInvalidURN) Title() string   { return "Invalid URN" }
func (e *ErrInvalidURN) Details() map[string]any {
	return map[string]any{"urn": e.URN, "expected_kind": e.Expected, "reason": e.Reason}
}
func (e *ErrInvalidURN) Unwrap() error { return repository.ErrInvalidArgument }

// ErrValidation: payload de Create/Update inválido (campo obrigatório
// vazio, formato errado, etc).
type ErrValidation struct {
	Field  string
	Reason string
}

func (e *ErrValidation) Error() string {
	return fmt.Sprintf("gov: validation failed on %q: %s", e.Field, e.Reason)
}
func (e *ErrValidation) HTTPStatus() int { return 400 }
func (e *ErrValidation) Code() string    { return "gov.validation" }
func (e *ErrValidation) Title() string   { return "Validation failed" }
func (e *ErrValidation) Details() map[string]any {
	return map[string]any{"field": e.Field, "reason": e.Reason}
}
func (e *ErrValidation) Unwrap() error { return repository.ErrInvalidArgument }

// ErrConflict: short_id já em uso (urn alvo já tem versão corrente).
type ErrConflict struct {
	URN  node.URN
	Kind node.Kind
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("gov: %s already exists: %s", e.Kind, e.URN)
}
func (e *ErrConflict) HTTPStatus() int { return 409 }
func (e *ErrConflict) Code() string    { return "gov." + string(e.Kind) + ".conflict" }
func (e *ErrConflict) Title() string   { return string(e.Kind) + " already exists" }
func (e *ErrConflict) Details() map[string]any {
	return map[string]any{"urn": e.URN, "kind": e.Kind}
}
func (e *ErrConflict) Unwrap() error { return repository.ErrConflict }
