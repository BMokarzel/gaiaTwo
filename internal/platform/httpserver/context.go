// Package httpserver provê o Server HTTP compartilhado do CostEngine.
//
// Cada módulo/feature implementa Registrar e é wired em cmd/api/main.go.
// O Server cuida de middlewares (RequestID/Recover/Auth), shape de erro
// (delegado a core/errs.Render), /healthz e catch-all 404.
//
// Não conhece domínio. Não conhece módulos. É plataforma técnica pura.
package httpserver

import "context"

// HeaderRequestID propaga o request_id end-to-end. Se o caller já enviar,
// usamos o dele (útil em fluxos cliente→gateway→API); senão geramos.
const HeaderRequestID = "X-Request-ID"

// HeaderTenantID é o seam de tenancy (ADR-001). Substituir por JWT/OIDC
// é trocar o authMiddleware — o header continua sendo o ponto de entrada
// no domínio.
const HeaderTenantID = "X-Tenant-ID"

// DefaultTenant é o tenant injetado quando o header está ausente e o
// Server NÃO está em modo TenantRequired.
const DefaultTenant = "default"

// ctxKey é um tipo privado para evitar colisão com chaves externas no
// context.Context (padrão recomendado pela stdlib).
type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyTenantID
)

// RequestIDFrom devolve o request_id injetado pelo middleware. Vazio se
// a chamada não passou pela cadeia padrão (ex.: teste unitário chamando
// o handler direto).
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// TenantIDFrom devolve o tenant resolvido pelo middleware de auth.
// Vazio só se o handler for chamado fora da cadeia (teste direto).
func TenantIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyTenantID).(string); ok {
		return v
	}
	return ""
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

func withTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantID, id)
}

// ContextWithTenant injeta um tenant no contexto. Usado por callers
// que não passam pela cadeia HTTP (CLI, testes diretos, jobs internos).
// Em HTTP normal, prefira deixar o `authMiddleware` resolver via header.
func ContextWithTenant(ctx context.Context, tenant string) context.Context {
	return withTenantID(ctx, tenant)
}
