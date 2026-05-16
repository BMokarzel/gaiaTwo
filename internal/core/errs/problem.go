// Package errs define o contrato de erros HTTP do CostEngine.
//
// Tipos de erro de domínio (em modules/<X>/errs.go) implementam a
// interface HTTPProblem para controlar como viram resposta HTTP. A
// função Render é o ponto único onde err → (status, body) acontece;
// handlers nunca fazem esse switch.
//
// O shape do body segue RFC 7807 (problem+json) com extensões:
//   - Code namespaced ("org.team.not_found") é o que clientes devem
//     matchear; estável entre versões.
//   - Extras vai achatado no top-level do JSON (ver Problem.MarshalJSON),
//     conforme o próprio RFC indica usar "extension members".
package errs

import (
	"encoding/json"
	"maps"
	"net/http"
)

// Problem é o envelope JSON de erro exposto na API.
//
// Convenções:
//   - Type é uma URI estável no formato "ce:err:<code>"; clientes podem
//     usá-la como link futuramente (mesmo sendo URN, não URL, hoje).
//   - Title é humano e curto; default vem de http.StatusText quando o
//     erro não fornece (interface Titler opcional).
//   - Code é o identificador machine-readable namespaced.
//   - Detail é livre; pode conter contexto adicional.
//   - TraceID é preenchido pelo httpserver, não pelo erro.
//   - Extras carrega campos específicos do erro (URN, paths, conflicts).
//     Convertidos em chaves top-level no JSON.
type Problem struct {
	Type    string         `json:"type"`
	Title   string         `json:"title"`
	Status  int            `json:"status"`
	Code    string         `json:"code"`
	Detail  string         `json:"detail,omitempty"`
	TraceID string         `json:"trace_id,omitempty"`
	Extras  map[string]any `json:"-"`
}

// MarshalJSON achata Extras no top-level do JSON. Conflito de chave
// entre campos fixos e Extras: campos fixos vencem (não pode-se renomear
// "code" via Details, por exemplo).
func (p Problem) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, 6+len(p.Extras))
	maps.Copy(out, p.Extras)
	out["type"] = p.Type
	out["title"] = p.Title
	out["status"] = p.Status
	out["code"] = p.Code
	if p.Detail != "" {
		out["detail"] = p.Detail
	}
	if p.TraceID != "" {
		out["trace_id"] = p.TraceID
	}
	return json.Marshal(out)
}

// ContentType é o media type oficial do RFC 7807 para responses de erro.
const ContentType = "application/problem+json"

// DefaultTitle retorna o título humano padrão para um status HTTP.
// Usado quando o erro não implementa Titler. Status fora da tabela
// padrão devolve "Error".
func DefaultTitle(status int) string {
	if t := http.StatusText(status); t != "" {
		return t
	}
	return "Error"
}
