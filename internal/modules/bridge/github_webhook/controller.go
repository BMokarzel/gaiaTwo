package github_webhook

import (
	"errors"
	"io"
	"net/http"

	"costEngine/internal/platform/httpserver"
)

// Controller expõe `POST /v1/webhooks/github`. Satisfaz
// httpserver.Registrar.
type Controller struct {
	svc    *Service
	secret string
}

// NewController retorna o controller. `secret` é o HMAC compartilhado
// (vazio rejeita toda request — fail-closed).
func NewController(svc *Service, secret string) *Controller {
	if svc == nil {
		panic("github_webhook: NewController requires non-nil service")
	}
	return &Controller{svc: svc, secret: secret}
}

// Register implementa httpserver.Registrar.
func (c *Controller) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/webhooks/github", c.handle)
}

const maxBodyBytes = 5 << 20 // 5 MiB; PR payloads típicos < 100 KiB

func (c *Controller) handle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "bad_body", "read body: %v", err))
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if err := VerifySignature(body, sig, c.secret); err != nil {
		httpserver.WriteError(w, r, httpserver.Errorf(http.StatusUnauthorized, "bad_signature", "invalid hmac"))
		return
	}

	// Só processamos `pull_request`. Outros eventos (push, ping, etc.)
	// são 200 no-op para silenciar o GitHub.
	event := r.Header.Get("X-GitHub-Event")
	if event != "pull_request" {
		httpserver.WriteJSON(w, http.StatusOK, Stats{Ignored: true, Reason: "event " + event})
		return
	}

	payload, err := Parse(body)
	if err != nil {
		if errors.Is(err, ErrIgnored) {
			httpserver.WriteJSON(w, http.StatusOK, Stats{Ignored: true, Reason: "no-op"})
			return
		}
		if errors.Is(err, ErrMalformed) {
			httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "malformed", "%v", err))
			return
		}
		httpserver.WriteError(w, r, err)
		return
	}

	stats, err := c.svc.Ingest(r.Context(), payload)
	if err != nil {
		// Erros do tipo "feature URN não existe" são 400; falhas de
		// repositório (rede, etc.) viram 500 via core/errs default.
		if errors.Is(err, ErrFeatureNotFound) || errors.Is(err, ErrFeatureURNInvalid) {
			httpserver.WriteError(w, r, httpserver.Errorf(http.StatusBadRequest, "feature_unresolved", "%v", err))
			return
		}
		httpserver.WriteError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, stats)
}
