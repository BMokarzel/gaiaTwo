package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"costEngine/internal/core/errs"
)

// WriteJSON serializa v como JSON com status. Falha de encode é
// silenciosa — pressupõe que handlers não passam estruturas com tipos
// não-serializáveis. Em produção, encode errors são raríssimos.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError serializa o erro como application/problem+json (RFC 7807).
// Delega o mapeamento err → (status, body) para core/errs.Render.
// TraceID é preenchido a partir do request context.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, p := errs.Render(err)
	p.TraceID = RequestIDFrom(r.Context())
	w.Header().Set("Content-Type", errs.ContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// genericHTTPError é o tipo retornado por Errorf — implementa HTTPProblem
// sem precisar declarar um tipo novo por endpoint. Use módulos de domínio
// para erros do dia a dia; use Errorf para validação de query string,
// rota não encontrada, e similares "técnicos".
type genericHTTPError struct {
	status int
	code   string
	msg    string
}

func (e *genericHTTPError) Error() string   { return e.msg }
func (e *genericHTTPError) HTTPStatus() int { return e.status }
func (e *genericHTTPError) Code() string    { return e.code }

// Errorf cria um erro tipado genérico com status, code e mensagem
// formatada. Útil para erros HTTP que não merecem um tipo dedicado
// (rota não encontrada, validação leve, etc.).
//
// Para erros de domínio (org.team.not_found, infra.resource.not_found),
// declare tipos em modules/<X>/errs.go com Details() apropriado.
func Errorf(status int, code, format string, args ...any) error {
	return &genericHTTPError{
		status: status,
		code:   code,
		msg:    fmt.Sprintf(format, args...),
	}
}
