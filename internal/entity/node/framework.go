package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Framework é uma dependência declarada por um manifest de pacote
// (F-023). É global ao tenant — múltiplos Modules/Services apontam
// para o mesmo Framework via DEPENDS_ON.
//
// URN: urn:ce:code:_global:framework/<ecosystem>!<name>
//
// `<ecosystem>` segue ManifestType (go|npm|pypi|cargo|maven|nuget|...).
type Framework struct {
	Base
	Ecosystem      string   `json:"ecosystem"` // "go" | "npm" | "pypi" | "cargo" | "maven" …
	Name           string   `json:"name"`      // "github.com/go-chi/chi/v5"
	LatestVersion  string   `json:"latest_version,omitempty"`
	Homepage       string   `json:"homepage,omitempty"`
	Repository     string   `json:"repository,omitempty"`
	IsDevOnly      bool     `json:"is_dev_only,omitempty"`
	LicenseSPDX    string   `json:"license_spdx,omitempty"` // duplicado leve para query rápida
}

// NewFrameworkURN produz a URN canônica para um Framework.
func NewFrameworkURN(ecosystem, name string) URN {
	id := ecosystem + "!" + name
	return NewURN(ProviderCode, "_global", KindFramework, id)
}

// ContentHash dos campos significativos.
func (f Framework) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(f.Ecosystem)
	sb.WriteByte('|')
	sb.WriteString(f.Name)
	sb.WriteByte('|')
	sb.WriteString(f.LatestVersion)
	sb.WriteByte('|')
	sb.WriteString(f.LicenseSPDX)
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
