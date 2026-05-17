package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SecurityAdvisory representa uma vulnerabilidade conhecida (CVE/GHSA)
// que afeta um ou mais Frameworks (F-023).
//
// URN: urn:ce:code:_global:security_advisory/<id>
//
// `<id>` é o identificador canônico — CVE-YYYY-NNNN, GHSA-xxxx-yyyy-zzzz.
type SecurityAdvisory struct {
	Base
	ID            string   `json:"id"`             // "CVE-2024-12345"
	Aliases       []string `json:"aliases,omitempty"`
	Severity      string   `json:"severity,omitempty"`     // low|medium|high|critical
	CVSSScore     float32  `json:"cvss_score,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Reference     string   `json:"reference,omitempty"`    // URL canônica
	PublishedAt   string   `json:"published_at,omitempty"` // ISO 8601 (string)
	WithdrawnAt   string   `json:"withdrawn_at,omitempty"`
}

// NewSecurityAdvisoryURN produz a URN canônica para um SecurityAdvisory.
func NewSecurityAdvisoryURN(id string) URN {
	return NewURN(ProviderCode, "_global", KindSecurityAdvisory, id)
}

// ContentHash dos campos significativos.
func (a SecurityAdvisory) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(a.ID)
	sb.WriteByte('|')
	sb.WriteString(a.Severity)
	sb.WriteByte('|')
	sb.WriteString(a.Summary)
	sb.WriteByte('|')
	for _, al := range a.Aliases {
		sb.WriteString(al)
		sb.WriteByte(',')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
