package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// License representa um identificador SPDX de licença (F-023).
//
// URN: urn:ce:code:_global:license/<spdx>
//
// `<spdx>` é o identificador canônico SPDX (`MIT`, `Apache-2.0`,
// `GPL-3.0-only`, ...). Coletor consulta tabela SPDX para preencher
// atributos auxiliares.
type License struct {
	Base
	SPDX             string `json:"spdx"` // "MIT", "Apache-2.0"
	Name             string `json:"name,omitempty"`
	URL              string `json:"url,omitempty"`
	IsOSIApproved    bool   `json:"is_osi_approved,omitempty"`
	IsFSFLibre       bool   `json:"is_fsf_libre,omitempty"`
	IsCopyleft       bool   `json:"is_copyleft,omitempty"`
	IsCommercialFree bool   `json:"is_commercial_free,omitempty"`
}

// NewLicenseURN produz a URN canônica para uma License.
func NewLicenseURN(spdx string) URN {
	return NewURN(ProviderCode, "_global", KindLicense, spdx)
}

// ContentHash dos campos significativos.
func (l License) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(l.SPDX)
	sb.WriteByte('|')
	sb.WriteString(l.Name)
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
