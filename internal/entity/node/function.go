package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ParamSlot é a representação estruturada de um param/return/field
// (ADR-008). Slots NÃO são edges — são metadado embutido no nó. Tipos
// declarados (struct/class/interface/enum) são referenciados por URN
// no `TypeRef`; primitivos ficam como literal (`int`, `string`, etc.).
type ParamSlot struct {
	Name     string            `json:"name"`
	Position int               `json:"position"`
	TypeRef  string            `json:"type_ref"`           // URN de Type/Schema ou literal primitivo
	Optional bool              `json:"optional,omitempty"`
	Variadic bool              `json:"variadic,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`     // struct tags, decorators…
}

// Function representa uma função/método declarado no código (F-007 /
// F-017). Cobre top-level funcs e métodos de tipo. Closures são
// nomeadas como `OuterFunc$closure#N` no `Symbol` (ADR-006).
//
// URN: urn:ce:code:<repo>:function/<service-module-path>!<namespace>!<symbol>
//
// `<symbol>` é o identificador no escopo declarante. Para métodos, o
// coletor encode como `(Type).Method` ou `(*Type).Method`.
type Function struct {
	Base
	ServiceURN URN    `json:"service_urn"`
	ModuleURN  URN    `json:"module_urn,omitempty"` // F-018, opcional durante transição
	Namespace  string `json:"namespace"`            // cross-language (ADR-006)
	Symbol     string `json:"symbol"`               // identificador no escopo
	Location   Location `json:"location,omitempty"`

	SignatureHash string      `json:"signature_hash"` // hash determinístico
	Signature     string      `json:"signature"`      // assinatura canônica legível
	Params        []ParamSlot `json:"params,omitempty"`  // ADR-008
	Returns       []ParamSlot `json:"returns,omitempty"` // ADR-008

	Exported    bool     `json:"exported,omitempty"`
	FeatureTags []string `json:"feature_tags,omitempty"` // F-028

	// Deprecated: usar Namespace. Mantido durante migração F-017.
	Package string `json:"package,omitempty"`
	// Deprecated: usar Location.File. Mantido durante migração F-017.
	File string `json:"file,omitempty"`
	// Deprecated: usar Location.LineInit. Mantido durante migração F-017.
	Line int `json:"line,omitempty"`
}

// NewFunctionURN produz a URN canônica para uma Function sob um Service.
// `namespace` é o identificador do módulo lógico (Go: `<mod-path>/<pkg>`;
// Python: `pkg.subpkg`; TS: `pkg/subpath`). Ver ADR-006.
func NewFunctionURN(repo, serviceModulePath, namespace, symbol string) URN {
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	id := serviceModulePath + "!" + namespace + "!" + symbol
	return NewURN(ProviderCode, repo, KindFunction, id)
}

// ContentHash dos campos significativos. File/Line propositalmente
// fora — mover sem mudar assinatura/conteúdo não rebumpa versão
// (ADR-006).
func (f Function) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(f.ServiceURN))
	sb.WriteByte('|')
	sb.WriteString(f.Namespace)
	sb.WriteByte('|')
	sb.WriteString(f.Symbol)
	sb.WriteByte('|')
	sb.WriteString(f.SignatureHash)
	sb.WriteByte('|')

	tags := append([]string(nil), f.FeatureTags...)
	sort.Strings(tags)
	for _, t := range tags {
		sb.WriteString(t)
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
