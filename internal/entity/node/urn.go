package node

import (
	"errors"
	"fmt"
	"strings"
)

// Formato canônico: urn:ce:<provider>:<account>:<kind>/<id>
//
// Exemplos:
//   urn:ce:aws:123456789012:compute/i-0a1b2c3d
//   urn:ce:aws:123456789012:persistence/payments-db
//   urn:ce:gcp:my-project:network/vpc-prod

const (
	urnScheme    = "urn"
	urnNamespace = "ce"
)

// ErrInvalidURN é retornado quando ParseURN encontra um formato inesperado.
var ErrInvalidURN = errors.New("invalid URN format")

// URNParts é a forma estruturada de uma URN.
type URNParts struct {
	Provider ProviderID
	Account  string
	Kind     Kind
	ID       string
}

// NewURN constrói uma URN canônica. Não valida semanticamente os valores
// (existência do provider, kind etc.) — apenas garante o formato.
func NewURN(provider ProviderID, account string, kind Kind, id string) URN {
	return URN(fmt.Sprintf("%s:%s:%s:%s:%s/%s",
		urnScheme, urnNamespace, provider, account, kind, id))
}

// ParseURN decompõe uma URN canônica em suas partes. Retorna ErrInvalidURN
// se o formato não bate.
//
// Aceita exatamente o formato produzido por NewURN. IDs podem conter "/"
// (ARNs longos): o split do último segmento usa o primeiro "/" depois do kind.
func ParseURN(u URN) (URNParts, error) {
	s := string(u)
	if s == "" {
		return URNParts{}, fmt.Errorf("%w: empty", ErrInvalidURN)
	}

	// Esperamos 5 segmentos separados por ":" — o último carrega "kind/id".
	segments := strings.SplitN(s, ":", 5)
	if len(segments) != 5 {
		return URNParts{}, fmt.Errorf("%w: expected 5 colon-separated segments, got %d", ErrInvalidURN, len(segments))
	}
	if segments[0] != urnScheme {
		return URNParts{}, fmt.Errorf("%w: scheme must be %q, got %q", ErrInvalidURN, urnScheme, segments[0])
	}
	if segments[1] != urnNamespace {
		return URNParts{}, fmt.Errorf("%w: namespace must be %q, got %q", ErrInvalidURN, urnNamespace, segments[1])
	}
	if segments[2] == "" || segments[3] == "" {
		return URNParts{}, fmt.Errorf("%w: provider and account are required", ErrInvalidURN)
	}

	// O último segmento é "kind/id"; "id" pode conter "/" adicional.
	kindAndID := strings.SplitN(segments[4], "/", 2)
	if len(kindAndID) != 2 || kindAndID[0] == "" || kindAndID[1] == "" {
		return URNParts{}, fmt.Errorf("%w: last segment must be kind/id", ErrInvalidURN)
	}

	return URNParts{
		Provider: ProviderID(segments[2]),
		Account:  segments[3],
		Kind:     Kind(kindAndID[0]),
		ID:       kindAndID[1],
	}, nil
}

// IsValid verifica rapidamente se a URN tem formato canônico.
func (u URN) IsValid() bool {
	_, err := ParseURN(u)
	return err == nil
}
