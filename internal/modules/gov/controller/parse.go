package controller

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/gov"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

const (
	defaultGovListLimit = 50
	maxGovListLimit     = 200
)

// splitGovPath extrai (urn, suffix) do wildcard `{rest...}`. URNs
// contêm `/` e `:`; suffixes são tentados em ordem.
//
// Retorna (..., false) se URN ausente.
func splitGovPath(r *http.Request, suffixes []string) (node.URN, string, bool) {
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

// parseAsOfOptions é o conjunto AsOf+IncludeInactive reusado em todos
// os reads do módulo.
func parseAsOfOptions(q url.Values) (gov.AsOfOptions, error) {
	as, err := parseAsOf(q.Get("as_of"))
	if err != nil {
		return gov.AsOfOptions{}, httpserver.Errorf(http.StatusBadRequest, "bad_request", "invalid as_of: %s", err.Error())
	}
	inc, err := parseIncludeInactive(q.Get("include_inactive"))
	if err != nil {
		return gov.AsOfOptions{}, err
	}
	return gov.AsOfOptions{AsOf: as, IncludeInactive: inc}, nil
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

// fingerprintGov integra escopo + parent + AsOf no fingerprint do cursor.
func fingerprintGov(scope, parent string, as repository.AsOf) string {
	asStr := ""
	if !as.IsZero() {
		asStr = as.Time().UTC().Format("2006-01-02T15:04:05Z")
	}
	return httpserver.FingerprintSearch(scope+"|"+parent, asStr)
}

// parseListQuery monta o ListQuery padrão. Retorna erro HTTP-friendly.
func (c *Controller) parseListQuery(r *http.Request, scope, parent string) (gov.ListQuery, string, error) {
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"), defaultGovListLimit, maxGovListLimit)
	if err != nil {
		return gov.ListQuery{}, "", err
	}
	opts, err := parseAsOfOptions(q)
	if err != nil {
		return gov.ListQuery{}, "", err
	}
	fp := fingerprintGov(scope, parent, opts.AsOf)
	cur, err := httpserver.DecodeCursor(q.Get("cursor"), fp)
	if err != nil {
		return gov.ListQuery{}, "", httpserver.Errorf(http.StatusBadRequest, "bad_request", "%s", err.Error())
	}
	return gov.ListQuery{AsOfOptions: opts, Limit: limit, Offset: cur.Offset}, fp, nil
}
