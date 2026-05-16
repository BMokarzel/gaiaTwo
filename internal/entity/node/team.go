package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// Team representa um time/área no org plane (F-010).
//
// URN: urn:ce:org:<tenant>:team/<slug>
//
// `<slug>` é derivado do nome via `Slugify` — idempotência por
// `(tenant, slug)`.
type Team struct {
	Base
	Tenant string `json:"tenant"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
}

// NewTeamURN produz a URN canônica para um Team.
func NewTeamURN(tenant, slug string) URN {
	return NewURN(ProviderOrg, tenant, KindTeam, slug)
}

// ContentHash dos campos significativos.
func (t Team) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(t.Tenant)
	sb.WriteByte('|')
	sb.WriteString(t.Slug)
	sb.WriteByte('|')
	sb.WriteString(t.Name)
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// Slugify produz um slug ASCII estável: lowercase, non-alphanumeric
// vira "-", múltiplos "-" colapsam, trim nos extremos. Usado por
// Team/Squad para chave determinística (F-010 D3).
//
// Ex.: "Platform Eng (Core)" → "platform-eng-core".
func Slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	return out
}
