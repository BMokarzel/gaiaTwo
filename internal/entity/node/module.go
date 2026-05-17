package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Module representa um agregador físico/lógico do code plane (F-018):
// pasta, package, namespace. Permite aninhamento via aresta CONTAINS
// (Module→Module, Service→Module, Module→Endpoint/Function/Type/...).
//
// URN: urn:ce:code:<repo>:module/<service-module-path>!<namespace>
//
// `<namespace>` é o identificador cross-language do módulo (ADR-006):
//   - Go:     `<go.module-path>/<relative-pkg-path>`
//   - Python: `<package>.<sub>.<sub>`
//   - TS/JS:  `<pkg-name>/<sub>/<sub>`
//   - Rust:   `<crate>::<sub>::<sub>`
//
// Module também suporta `feature_tags` (F-028).
type Module struct {
	Base
	ServiceURN URN      `json:"service_urn"`
	ParentURN  URN      `json:"parent_urn,omitempty"` // Module pai (CONTAINS aninhável) ou vazio na raiz do service
	Namespace  string   `json:"namespace"`
	ShortName  string   `json:"short_name"` // último segmento do namespace
	Path       string   `json:"path,omitempty"` // diretório relativo ao repo
	Language   string   `json:"language,omitempty"`
	FeatureTags []string `json:"feature_tags,omitempty"`
}

// NewModuleURN produz a URN canônica para um Module sob um Service.
func NewModuleURN(repo, serviceModulePath, namespace string) URN {
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	return NewURN(ProviderCode, repo, KindModule, serviceModulePath+"!"+namespace)
}

// ContentHash dos campos significativos.
func (m Module) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(m.ServiceURN))
	sb.WriteByte('|')
	sb.WriteString(string(m.ParentURN))
	sb.WriteByte('|')
	sb.WriteString(m.Namespace)
	sb.WriteByte('|')
	sb.WriteString(m.ShortName)
	sb.WriteByte('|')
	sb.WriteString(m.Language)
	sb.WriteByte('|')

	tags := append([]string(nil), m.FeatureTags...)
	sort.Strings(tags)
	for _, t := range tags {
		sb.WriteString(t)
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
