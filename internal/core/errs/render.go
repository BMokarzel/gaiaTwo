package errs

import (
	"errors"
	"net/http"

	"costEngine/internal/repository"
)

// Render mapeia qualquer error para (status HTTP, Problem).
//
// Ordem de precedência:
//  1. errors.As encontra HTTPProblem na cadeia → usa-o (o mais
//     "interno" / específico que satisfaz vence — semântica padrão de
//     errors.As).
//  2. errors.Is matchea um sentinel conhecido do repository → fallback
//     com code legado ("not_found", "ambiguous", "bad_request",
//     "conflict").
//  3. Caso contrário: 500 "internal", sem vazar err.Error().
//
// Render(nil) devolve 500 "internal" — é defesa, não cenário esperado;
// chamar com nil é bug de quem chama, mas não deve panicar.
func Render(err error) (int, Problem) {
	if err == nil {
		return http.StatusInternalServerError, problem(
			http.StatusInternalServerError,
			"internal",
			"nil error passed to Render",
		)
	}

	// Caminho preferencial: erro tipado.
	var hp HTTPProblem
	if errors.As(err, &hp) {
		status := hp.HTTPStatus()
		code := hp.Code()

		title := DefaultTitle(status)
		if t, ok := any(hp).(Titler); ok {
			if s := t.Title(); s != "" {
				title = s
			}
		}

		var extras map[string]any
		if d, ok := any(hp).(Detailer); ok {
			extras = d.Details()
		}

		return status, Problem{
			Type:   "ce:err:" + code,
			Title:  title,
			Status: status,
			Code:   code,
			Detail: hp.Error(),
			Extras: extras,
		}
	}

	// Fallback: sentinels do repository (compatibilidade durante a
	// migração F-016; módulos que ainda não declararam erros tipados
	// continuam funcionando).
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound, problem(http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, repository.ErrAmbiguous):
		return http.StatusConflict, problem(http.StatusConflict, "ambiguous", "lookup matched multiple resources")
	case errors.Is(err, repository.ErrConflict):
		return http.StatusConflict, problem(http.StatusConflict, "conflict", "resource conflict")
	case errors.Is(err, repository.ErrInvalidArgument):
		// ErrInvalidArgument: caller já formatou mensagem segura — propagar.
		return http.StatusBadRequest, problem(http.StatusBadRequest, "bad_request", err.Error())
	}

	// Default: 500 sem leak.
	return http.StatusInternalServerError, problem(
		http.StatusInternalServerError,
		"internal",
		"internal error",
	)
}

// problem constrói um Problem com Type derivado de Code e Title padrão
// vindo do status.
func problem(status int, code, detail string) Problem {
	return Problem{
		Type:   "ce:err:" + code,
		Title:  DefaultTitle(status),
		Status: status,
		Code:   code,
		Detail: detail,
	}
}
