package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

// Caps (F-014).
const (
	maxNeighborsDepth   = 3
	maxNeighborsResults = 100
	defaultHistoryLimit = 50
	maxHistoryLimit     = 500
	defaultPathsHops    = 3
	maxPathsHops        = 5
)

const codeBadRequest = "bad_request"

func parseAsOf(s string) (repository.AsOf, error) {
	if s == "" {
		return repository.AsOf{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return repository.AsOf{}, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid as_of: %s", err.Error())
	}
	return repository.AsOf(t), nil
}

func parseLimit(s string, def, max int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid limit: must be positive integer")
	}
	if n > max {
		n = max
	}
	return n, nil
}

func parseDepth(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid depth: must be positive integer")
	}
	if n > maxNeighborsDepth {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid depth: exceeds cap (max %d)", maxNeighborsDepth)
	}
	return n, nil
}

func parseDirection(s string) (repository.Direction, error) {
	switch strings.ToLower(s) {
	case "", "any":
		return repository.DirAny, nil
	case "in":
		return repository.DirIn, nil
	case "out":
		return repository.DirOut, nil
	default:
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid dir: must be in|out|any")
	}
}

func parseHops(s string) (int, error) {
	if s == "" {
		return defaultPathsHops, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid max_hops: must be positive integer")
	}
	if n > maxPathsHops {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid max_hops: exceeds cap (max %d)", maxPathsHops)
	}
	return n, nil
}

// knownEdgeTypes é o whitelist usado no parsing de ?edge_types=. Lista
// fechada — adições em `entity/edge` precisam refletir aqui (a evidência
// é uma rota de produto, não uma sentinela do core).
var knownEdgeTypes = []edge.Type{
	edge.TypeContains, edge.TypeDeployedOn, edge.TypeAttachedTo,
	edge.TypeRoutes, edge.TypePeers, edge.TypeDependsOn,
	edge.TypeCommunicatesWith, edge.TypeReplaces,
	edge.TypeDefinedIn, edge.TypeServiceRunsOn,
	edge.TypeMemberOf, edge.TypePartOf, edge.TypeReportsTo,
}

// parseEdgeTypes aceita CSV (CONTAINS, DEPENDS_ON, ...), case-insensitive.
// Tipos desconhecidos são silenciosamente ignorados — preserva
// forward-compat com clientes mais novos pedindo tipos que o servidor
// antigo ainda não conhece.
func parseEdgeTypes(s string) []edge.Type {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	knownSet := map[string]edge.Type{}
	for _, t := range knownEdgeTypes {
		knownSet[strings.ToLower(string(t))] = t
	}
	var out []edge.Type
	for _, raw := range strings.Split(s, ",") {
		k := strings.TrimSpace(strings.ToLower(raw))
		if t, ok := knownSet[k]; ok {
			out = append(out, t)
		}
	}
	return out
}
