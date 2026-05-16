package golang

import (
	"fmt"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Config controla a coleta sobre um repositório.
//
// Defaults:
//   - SkipDirs: ["vendor", "node_modules"] (sempre); dirs com prefixo "."
//     também são pulados automaticamente.
//   - SkipPathSubstrings: ["vendor/", "gen/", "mocks/"].
//   - IncludePkgSubstrings: ["handler", "service", "usecase"].
type Config struct {
	Repo                 string
	RunID                string
	ObservedAt           time.Time
	ExtraSkipDirs        []string
	SkipPathSubstrings   []string
	IncludePkgSubstrings []string
}

// Result agrupa o que um Collect() produz para um repositório inteiro.
type Result struct {
	Services  []node.Service
	Endpoints []node.Endpoint
	Functions []node.Function
	Edges     []edge.DefinedIn
}

// Collect roda a pipeline completa sobre um repo:
//
//	walk → para cada Service: extract functions + endpoints → emite edges
//
// É idempotente — URNs e IDs de arestas são determinísticos.
func Collect(repoRoot string, cfg Config) (Result, error) {
	if cfg.Repo == "" {
		return Result{}, fmt.Errorf("collector: Repo is required")
	}
	if cfg.ObservedAt.IsZero() {
		cfg.ObservedAt = time.Now().UTC()
	}

	found, err := Walk(repoRoot, WalkOptions{ExtraSkipDirs: cfg.ExtraSkipDirs})
	if err != nil {
		return Result{}, err
	}

	emit := EmitOptions{Repo: cfg.Repo, RunID: cfg.RunID, ObservedAt: cfg.ObservedAt}
	services := EmitServices(found, emit)

	res := Result{Services: services}

	funcFilter := FuncFilterOptions{
		IncludePkgSubstrings: cfg.IncludePkgSubstrings,
		SkipPathSubstrings:   cfg.SkipPathSubstrings,
	}

	for i, svc := range services {
		fns, err := ExtractFunctions(
			repoRoot, found[i].AbsPath, svc.ModulePath,
			svc.URN(), cfg.Repo, funcFilter, emit,
		)
		if err != nil {
			return Result{}, fmt.Errorf("functions in %s: %w", svc.ModulePath, err)
		}
		eps, err := ExtractEndpoints(
			repoRoot, found[i].AbsPath, svc.ModulePath,
			svc.URN(), cfg.Repo, cfg.SkipPathSubstrings, emit,
		)
		if err != nil {
			return Result{}, fmt.Errorf("endpoints in %s: %w", svc.ModulePath, err)
		}
		res.Functions = append(res.Functions, fns...)
		res.Endpoints = append(res.Endpoints, eps...)

		// DefinedIn edges
		for _, fn := range fns {
			res.Edges = append(res.Edges, newDefinedIn(fn.URN(), svc.URN(), cfg, emit))
		}
		for _, ep := range eps {
			res.Edges = append(res.Edges, newDefinedIn(ep.URN(), svc.URN(), cfg, emit))
		}
	}

	return res, nil
}

func newDefinedIn(from, to node.URN, cfg Config, emit EmitOptions) edge.DefinedIn {
	return edge.DefinedIn{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeDefinedIn, to, cfg.ObservedAt),
			EdgeType: edge.TypeDefinedIn,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   cfg.ObservedAt,
				ObservedAt:  cfg.ObservedAt,
				Source:      node.Source{Collector: "code/golang", RunID: emit.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
			},
		},
	}
}
