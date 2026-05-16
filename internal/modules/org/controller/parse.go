package controller

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

// Caps (F-015 / F-016).
const (
	defaultOrgListLimit = 50
	maxOrgListLimit     = 200
	defaultReportsDepth = 1
	maxReportsDepth     = 3
)

// splitOrgPath extrai (urn, suffix) do wildcard {rest...}. Retorna
// (..., false) se URN ausente. Suffixes são checados em ordem.
//
// URN contém `/` e `:`; PathValue desfaz percent-encoding; ainda assim
// normalizamos `%2F` extra para clientes que escolherem encodar.
func splitOrgPath(r *http.Request, suffixes []string) (node.URN, string, bool) {
	rest := r.PathValue("rest")
	if dec, err := url.PathUnescape(rest); err == nil {
		rest = dec
	}
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		return "", "", false
	}
	for _, sfx := range suffixes {
		if base, ok := strings.CutSuffix(rest, sfx); ok {
			return node.URN(base), sfx, true
		}
	}
	return node.URN(rest), "", true
}

// parseAsOfOptions é o conjunto AsOf+IncludeInactive que aparece em
// todos os endpoints de read do módulo. Erro tipado (HTTPProblem) para
// renderização direta via httpserver.WriteError.
func parseAsOfOptions(q url.Values) (org.AsOfOptions, error) {
	as, err := parseAsOf(q.Get("as_of"))
	if err != nil {
		return org.AsOfOptions{}, httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid as_of: %s", err.Error())
	}
	inc, err := parseIncludeInactive(q.Get("include_inactive"))
	if err != nil {
		return org.AsOfOptions{}, err
	}
	return org.AsOfOptions{AsOf: as, IncludeInactive: inc}, nil
}

func parseAsOf(s string) (repository.AsOf, error) {
	if s == "" {
		return repository.AsOf{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return repository.AsOf{}, err
	}
	return repository.AsOf(t), nil
}

func parseLimit(s string, def, max int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid limit: must be positive integer")
	}
	if n > max {
		n = max
	}
	return n, nil
}

func parseReportsDepth(s string) (int, error) {
	if s == "" {
		return defaultReportsDepth, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid depth: must be positive integer")
	}
	if n > maxReportsDepth {
		return 0, httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid depth: exceeds cap (max %d)", maxReportsDepth)
	}
	return n, nil
}

func parseIncludeInactive(s string) (bool, error) {
	switch s {
	case "", "false", "0":
		return false, nil
	case "true", "1":
		return true, nil
	default:
		return false, httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid include_inactive: must be true|false")
	}
}

// fingerprintOrg integra escopo + parent + AsOf no fingerprint do
// cursor. Mudar qualquer um deles invalida o cursor.
func fingerprintOrg(scope, parent string, as repository.AsOf) string {
	asStr := ""
	if !as.IsZero() {
		asStr = as.Time().UTC().Format("2006-01-02T15:04:05Z")
	}
	return httpserver.FingerprintSearch(scope+"|"+parent, asStr)
}
