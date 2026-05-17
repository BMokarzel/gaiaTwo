package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// TypeKind classifica a natureza do tipo declarado (F-021).
type TypeKind string

const (
	TypeKindStruct    TypeKind = "struct"
	TypeKindClass     TypeKind = "class"
	TypeKindInterface TypeKind = "interface"
	TypeKindEnum      TypeKind = "enum"
	TypeKindAlias     TypeKind = "alias"
	TypeKindUnion     TypeKind = "union"
)

// FieldSlot é a representação estruturada de um campo de Type
// (ADR-008). `TypeRef` aceita URN de Type/Schema ou literal primitivo.
type FieldSlot struct {
	Name     string            `json:"name"`
	Position int               `json:"position"`
	TypeRef  string            `json:"type_ref"`
	Optional bool              `json:"optional,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"` // struct tags, decorators…
}

// MethodSlot referencia um método declarado em um Type. O método em
// si é um nó Function separado — aqui só persistimos a referência
// posicional (ADR-008).
type MethodSlot struct {
	Name        string `json:"name"`
	FunctionURN URN    `json:"function_urn"`
}

// Type representa um tipo declarado no código (struct, class,
// interface, enum, alias, union). Cross-language por desenho (ADR-006).
//
// URN: urn:ce:code:<repo>:type/<service-module-path>!<namespace>!<symbol>
type Type struct {
	Base
	ServiceURN URN      `json:"service_urn"`
	ModuleURN  URN      `json:"module_urn,omitempty"`
	Namespace  string   `json:"namespace"`
	Symbol     string   `json:"symbol"`
	Kind_      TypeKind `json:"type_kind"` // evita colisão com Node.Kind()
	Location   Location `json:"location,omitempty"`

	Fields  []FieldSlot  `json:"fields,omitempty"`
	Methods []MethodSlot `json:"methods,omitempty"`

	Exported    bool     `json:"exported,omitempty"`
	FeatureTags []string `json:"feature_tags,omitempty"`
}

// NewTypeURN produz a URN canônica para um Type sob um Service.
func NewTypeURN(repo, serviceModulePath, namespace, symbol string) URN {
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	return NewURN(ProviderCode, repo, KindType, serviceModulePath+"!"+namespace+"!"+symbol)
}

// ContentHash dos campos significativos.
func (t Type) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(t.ServiceURN))
	sb.WriteByte('|')
	sb.WriteString(t.Namespace)
	sb.WriteByte('|')
	sb.WriteString(t.Symbol)
	sb.WriteByte('|')
	sb.WriteString(string(t.Kind_))
	sb.WriteByte('|')

	for _, f := range t.Fields {
		sb.WriteString(f.Name)
		sb.WriteByte(':')
		sb.WriteString(f.TypeRef)
		sb.WriteByte(',')
	}
	sb.WriteByte('|')

	for _, m := range t.Methods {
		sb.WriteString(m.Name)
		sb.WriteByte('=')
		sb.WriteString(string(m.FunctionURN))
		sb.WriteByte(',')
	}
	sb.WriteByte('|')

	tags := append([]string(nil), t.FeatureTags...)
	sort.Strings(tags)
	for _, tag := range tags {
		sb.WriteString(tag)
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
