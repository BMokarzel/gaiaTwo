package httpserver

import (
	"log/slog"
	"net/http"
)

// Config parametriza o Server. Campos zero ganham defaults sensatos.
type Config struct {
	// TenantRequired controla o modo do auth middleware (ADR-001).
	// false: header ausente injeta tenant "default".
	// true:  header ausente devolve 401.
	TenantRequired bool

	// Logger usado por middlewares (panic recovery etc.). nil → slog.Default().
	Logger *slog.Logger
}

// Registrar é implementada por cada controller (de módulo ou de feature
// em app/*) para registrar suas rotas no mux compartilhado.
//
// Convenção: usar padrões Go 1.22+ ("GET /v1/teams", "GET /v1/foo/{rest...}").
// Padrões duplicados causam panic no http.ServeMux — Registrars não
// devem registrar /healthz nem o catch-all "/" (são reservados ao Server).
type Registrar interface {
	Register(mux *http.ServeMux)
}

// Server agrega config e mux. Construído via New, exposto via Routes().
type Server struct {
	cfg Config
	log *slog.Logger
	mux *http.ServeMux
}

// New monta o Server. Cada Registrar.Register é chamado em ordem; o
// Server registra /healthz e o catch-all 404 DEPOIS, para que padrões
// específicos vindos dos Registrars tenham precedência natural.
//
// Panica se algum Registrar tentar registrar pattern duplicado — é o
// comportamento padrão do http.ServeMux e queremos visibilidade do erro
// no startup, não em runtime.
func New(cfg Config, regs ...Registrar) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	s := &Server{cfg: cfg, log: cfg.Logger, mux: mux}

	for _, r := range regs {
		r.Register(mux)
	}

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("/", s.notFound)

	return s
}

// Routes devolve o handler completo, com a cadeia de middlewares
// aplicada (RequestID → Recover → Auth → mux).
func (s *Server) Routes() http.Handler {
	return chain(s.mux,
		requestIDMiddleware,
		recoverMiddleware(s.log),
		authMiddleware(s.cfg.TenantRequired),
	)
}

// healthz é trivial. Em modo TenantRequired, a probe HTTP precisa enviar
// X-Tenant-ID — costuma-se expor /healthz fora do servidor v1 (em cmd/api)
// para liveness probes que não conhecem tenancy.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": "v1",
	})
}

// notFound devolve 404 RFC 7807 consistente.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, Errorf(http.StatusNotFound, "not_found", "route not found"))
}
