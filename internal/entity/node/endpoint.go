package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Endpoint representa um handler HTTP detectado no AST (F-007). Cada
// Endpoint é definido por um único `(METHOD, ROUTE)` dentro de um Service.
//
// URN: urn:ce:code:<repo>:endpoint/<service-module-path>!<METHOD>:<route>
//
// O separador `!` entre service-id e method:route evita ambiguidade com
// "/" presentes no module-path e no route — ParseURN faz SplitN("/", 2)
// no segmento final, então só o primeiro "/" separa kind do id; o resto
// do id é livre.
type Endpoint struct {
	Base
	ServiceURN URN    `json:"service_urn"`
	Method     string `json:"method"` // GET, POST, PUT, DELETE, PATCH, ...
	Route      string `json:"route"`  // "/v1/users/{id}"
	Handler    string `json:"handler_symbol"`     // "package.Symbol"
	File       string `json:"file"`               // path relativo ao repo
	Line       int    `json:"line"`               // linha onde a rota é registrada
	Framework  string `json:"framework"`          // "net/http" ou "chi"
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

// ContentHash dos campos significativos.
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
	// File/Line propositalmente fora — refatorar o arquivo de rotas
	// (mover handler) não cria versão nova se o (method,route,handler)
	// não mudou.

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
