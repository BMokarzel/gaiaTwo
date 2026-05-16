package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// requestIDMiddleware injeta X-Request-ID no context e na resposta.
// Honra valor entrante se válido (hex/dash ≤ 64 chars) — defesa contra
// envenenamento de log via header arbitrário.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !isValidRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := withRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isValidRequestID aceita IDs hex/alfanumérico curtos.
func isValidRequestID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		case c == '-':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// recoverMiddleware captura panics, loga com stack e devolve 500
// genérico. Stack jamais sai para o corpo da resposta.
func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"err", rec,
						"path", r.URL.Path,
						"request_id", RequestIDFrom(r.Context()),
						"stack", string(debug.Stack()),
					)
					WriteError(w, r, Errorf(http.StatusInternalServerError, "internal", "internal error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// authMiddleware resolve o tenant a partir do X-Tenant-ID.
//
// Modos (ADR-001):
//   - required=false → header ausente vira tenant "default".
//   - required=true  → header ausente devolve 401.
//
// Não valida conteúdo do tenant (sem JWT/OIDC ainda). Substituir esta
// função é o ponto de entrada para auth real.
func authMiddleware(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := r.Header.Get(HeaderTenantID)
			if t == "" {
				if required {
					WriteError(w, r, Errorf(http.StatusUnauthorized, "unauthorized", "missing "+HeaderTenantID))
					return
				}
				t = DefaultTenant
			}
			ctx := withTenantID(r.Context(), t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// chain aplica middlewares de fora para dentro.
// chain(h, m1, m2) resulta em m1(m2(h)) — m1 vê a request primeiro.
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
