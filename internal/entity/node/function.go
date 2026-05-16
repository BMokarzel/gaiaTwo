package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Function representa uma função exportada relevante (F-007). MVP cobre
// funções top-level em pacotes "handler/service/usecase" (filtrado pelo
// coletor).
//
// URN: urn:ce:code:<repo>:function/<service-module-path>!<package>.<symbol>
//
// `<symbol>` é o identificador exportado (`CreateUser`). Para métodos
// associados a tipo, o coletor encode como `(Type).Method`.
type Function struct {
	Base
	ServiceURN    URN    `json:"service_urn"`
	Package       string `json:"package"`        // short name do pacote
	Symbol        string `json:"symbol"`         // nome exportado
	File          string `json:"file"`           // path relativo ao repo
	Line          int    `json:"line"`           // linha da declaração
	SignatureHash string `json:"signature_hash"` // hash determinístico da assinatura
	Signature     string `json:"signature"`      // assinatura canônica legível (params/returns)
}

// NewFunctionURN produz a URN canônica para uma Function sob um Service.
func NewFunctionURN(repo, serviceModulePath, pkg, symbol string) URN {
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	id := serviceModulePath + "!" + pkg + "." + symbol
	return NewURN(ProviderCode, repo, KindFunction, id)
}

// ContentHash dos campos significativos.
func (f Function) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(f.ServiceURN))
	sb.WriteByte('|')
	sb.WriteString(f.Package)
	sb.WriteByte('|')
	sb.WriteString(f.Symbol)
	sb.WriteByte('|')
	sb.WriteString(f.SignatureHash)
	// File/Line propositalmente fora — mover o arquivo sem mudar
	// assinatura não rebumpa versão.

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
