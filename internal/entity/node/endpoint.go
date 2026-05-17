package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// BodyRef descreve o corpo de uma request/response em um Endpoint
// (ADR-008). `TypeRef` aceita URN de Type/Schema ou literal primitivo.
type BodyRef struct {
	TypeRef     string `json:"type_ref"`
	ContentType string `json:"content_type,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Endpoint representa um handler HTTP detectado no AST (F-007).
// Cada Endpoint é definido por um único `(METHOD, ROUTE)` dentro de um
// Service.
//
// URN: urn:ce:code:<repo>:endpoint/<service-module-path>!<METHOD>:<route>
//
// O separador `!` entre service-id e method:route evita ambiguidade
// com "/" presentes no module-path e no route.
type Endpoint struct {
	Base
	ServiceURN URN    `json:"service_urn"`
	ModuleURN  URN    `json:"module_urn,omitempty"` // F-018, opcional durante transição
	Method     string `json:"method"`               // GET, POST, PUT, DELETE, PATCH, ...
	Route      string `json:"route"`                // "/v1/users/{id}"
	Handler    string `json:"handler_symbol"`       // "package.Symbol"
	Framework  string `json:"framework"`            // "net/http", "chi", "fastapi"…
	Location   Location `json:"location,omitempty"`

	Request   *BodyRef            `json:"request,omitempty"`
	Responses map[string]BodyRef  `json:"responses,omitempty"` // chave = status code ("200", "default")

	FeatureTags []string `json:"feature_tags,omitempty"` // F-028

	// Deprecated: usar Location.File. Mantido durante migração F-017.
	File string `json:"file,omitempty"`
	// Deprecated: usar Location.LineInit. Mantido durante migração F-017.
	Line int `json:"line,omitempty"`
}

// NewEndpointURN produz a URN canônica para um Endpoint sob um Service.
// `serviceModulePath` é o `Service.ModulePath`.
func NewEndpointURN(repo, serviceModulePath, method, route string) URN {
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	id := serviceModulePath + "!" + strings.ToUpper(method) + ":" + route
	return NewURN(ProviderCode, repo, KindEndpoint, id)
}

// ContentHash dos campos significativos. File/Line propositalmente
// fora — refatorar arquivo de rotas não cria versão nova se o
// (method,route,handler) não mudou.
func (e Endpoint) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(e.ServiceURN))
	sb.WriteByte('|')
	sb.WriteString(strings.ToUpper(e.Method))
	sb.WriteByte('|')
	sb.WriteString(e.Route)
	sb.WriteByte('|')
	sb.WriteString(e.Handler)
	sb.WriteByte('|')
	sb.WriteString(e.Framework)
	sb.WriteByte('|')

	if e.Request != nil {
		sb.WriteString(e.Request.TypeRef)
		sb.WriteByte('@')
		sb.WriteString(e.Request.ContentType)
	}
	sb.WriteByte('|')

	codes := make([]string, 0, len(e.Responses))
	for k := range e.Responses {
		codes = append(codes, k)
	}
	sort.Strings(codes)
	for _, c := range codes {
		r := e.Responses[c]
		sb.WriteString(c)
		sb.WriteByte('=')
		sb.WriteString(r.TypeRef)
		sb.WriteByte('@')
		sb.WriteString(r.ContentType)
		sb.WriteByte(',')
	}
	sb.WriteByte('|')

	tags := append([]string(nil), e.FeatureTags...)
	sort.Strings(tags)
	for _, t := range tags {
		sb.WriteString(t)
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
