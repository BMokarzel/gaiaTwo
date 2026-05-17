package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// VariableMutability classifica se a variável é constante ou mutável.
type VariableMutability string

const (
	VariableConst VariableMutability = "const"
	VariableMut   VariableMutability = "var"
)

// Variable representa um identifier de escopo de pacote/módulo
// (top-level `var`/`const`, em Go; `let`/`const` exportados em TS;
// variáveis de módulo em Python). Variáveis locais a funções NÃO são
// nós (não atendem o critério da promoção — ADR-006).
//
// URN: urn:ce:code:<repo>:variable/<service-module-path>!<namespace>!<symbol>
type Variable struct {
	Base
	ServiceURN  URN                `json:"service_urn"`
	ModuleURN   URN                `json:"module_urn,omitempty"`
	Namespace   string             `json:"namespace"`
	Symbol      string             `json:"symbol"`
	TypeRef     string             `json:"type_ref,omitempty"` // URN de Type ou primitivo
	Mutability  VariableMutability `json:"mutability"`
	HasInit     bool               `json:"has_init,omitempty"`
	Exported    bool               `json:"exported,omitempty"`
	Location    Location           `json:"location,omitempty"`
	FeatureTags []string           `json:"feature_tags,omitempty"`
}

// NewVariableURN produz a URN canônica para uma Variable.
func NewVariableURN(repo, serviceModulePath, namespace, symbol string) URN {
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	return NewURN(ProviderCode, repo, KindVariable, serviceModulePath+"!"+namespace+"!"+symbol)
}

// ContentHash dos campos significativos.
func (v Variable) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(v.ServiceURN))
	sb.WriteByte('|')
	sb.WriteString(v.Namespace)
	sb.WriteByte('|')
	sb.WriteString(v.Symbol)
	sb.WriteByte('|')
	sb.WriteString(v.TypeRef)
	sb.WriteByte('|')
	sb.WriteString(string(v.Mutability))
	sb.WriteByte('|')

	tags := append([]string(nil), v.FeatureTags...)
	sort.Strings(tags)
	for _, t := range tags {
		sb.WriteString(t)
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
