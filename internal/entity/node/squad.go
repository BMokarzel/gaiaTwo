package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Squad representa um squad (sub-unidade de Team) no org plane (F-010).
//
// URN: urn:ce:org:<tenant>:squad/<slug>
//
// Squad guarda referência ao Team pai via TeamURN.
type Squad struct {
	Base
	Tenant  string `json:"tenant"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	TeamURN URN    `json:"team_urn,omitempty"`
}

// NewSquadURN produz a URN canônica para um Squad.
func NewSquadURN(tenant, slug string) URN {
	return NewURN(ProviderOrg, tenant, KindSquad, slug)
}

// ContentHash dos campos significativos.
func (s Squad) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(s.Tenant)
	sb.WriteByte('|')
	sb.WriteString(s.Slug)
	sb.WriteByte('|')
	sb.WriteString(s.Name)
	sb.WriteByte('|')
	sb.WriteString(string(s.TeamURN))
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
