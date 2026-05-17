package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ManifestType identifica o ecossistema de pacotes do Service.
// Suporta o code plane cross-language (ADR-006, F-017).
type ManifestType string

const (
	ManifestGoMod    ManifestType = "go.mod"
	ManifestNpm      ManifestType = "package.json"
	ManifestPyProj   ManifestType = "pyproject.toml"
	ManifestPySetup  ManifestType = "setup.py"
	ManifestCargo    ManifestType = "Cargo.toml"
	ManifestPom      ManifestType = "pom.xml"
	ManifestGradle   ManifestType = "build.gradle"
	ManifestComposer ManifestType = "composer.json"
	ManifestGemfile  ManifestType = "Gemfile"
	ManifestNuget    ManifestType = "*.csproj"
	ManifestUnknown  ManifestType = "unknown"
)

// Service representa a raiz de um manifest de pacote (`go.mod`,
// `package.json`, `pyproject.toml`, …). É um nó do **code plane** —
// não é Resource (não tem account/region/spec do provedor).
//
// URN: urn:ce:code:<repo>:service/<module-path>
//
// Onde `<module-path>` é o caminho relativo da raiz do manifest dentro
// do repo (`.` se for a raiz, ou `cmd/cli`, `services/billing`, etc.).
//
// **Cross-language (ADR-006, F-017):**
//   - `Namespace` substitui o conceito Go-específico de "go module
//     path". Em Go é o valor de `module` em go.mod; em Python o nome
//     em `pyproject.toml`; em TS o `name` em `package.json`; etc.
//   - `Manifest` é o caminho relativo ao manifest dentro do repo
//     (`go.mod`, `services/api/package.json`).
//   - `ManifestType` discrimina o ecossistema.
//   - `GoModule` permanece deprecated por compat: populado pelo
//     coletor Go em paralelo com Namespace durante a migração.
type Service struct {
	Base
	Repo         string            `json:"repo"`        // canonical repo name
	ModulePath   string            `json:"module_path"` // "." ou caminho do submódulo
	Language     string            `json:"language"`    // "go" | "python" | "typescript" | ...
	Namespace    string            `json:"namespace"`   // identidade lógica cross-language
	Manifest     string            `json:"manifest,omitempty"`      // caminho do manifest no repo
	ManifestType ManifestType      `json:"manifest_type,omitempty"` // discriminador
	FeatureTags  []string          `json:"feature_tags,omitempty"`  // F-028 (ADR-009)
	Tags         map[string]string `json:"tags,omitempty"`

	// Deprecated: usar Namespace. Mantido durante migração F-017.
	GoModule string `json:"go_module,omitempty"`
}

// NewServiceURN produz a URN canônica para um Service.
func NewServiceURN(repo, modulePath string) URN {
	if modulePath == "" {
		modulePath = "."
	}
	return NewURN(ProviderCode, repo, KindService, modulePath)
}

// ContentHash dos campos significativos — exclui Meta. Usado pelo
// coletor para decidir Upsert (nova versão) vs Touch (mesmo conteúdo).
func (s Service) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(s.Repo)
	sb.WriteByte('|')
	sb.WriteString(s.ModulePath)
	sb.WriteByte('|')
	sb.WriteString(s.Language)
	sb.WriteByte('|')
	sb.WriteString(s.Namespace)
	sb.WriteByte('|')
	sb.WriteString(s.Manifest)
	sb.WriteByte('|')
	sb.WriteString(string(s.ManifestType))
	sb.WriteByte('|')

	tags := append([]string(nil), s.FeatureTags...)
	sort.Strings(tags)
	for _, t := range tags {
		sb.WriteString(t)
		sb.WriteByte(',')
	}
	sb.WriteByte('|')

	keys := make([]string, 0, len(s.Tags))
	for k := range s.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(s.Tags[k])
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
