package golang

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
//
// Modules + Contains são populados quando F-018 está ativo (sempre, no
// MVP atual). DefinedIn é mantido por compat e ficará deprecated.
type Result struct {
	Services   []node.Service
	Modules    []node.Module
	Endpoints  []node.Endpoint
	Functions  []node.Function
	Frameworks []node.Framework // F-023, globais (deduplicar no caller se múltiplos services compartilham)
	Calls      []node.Call      // F-019/F-020, boundary + in-process calls reconhecidos
	Types      []node.Type      // F-021
	Variables  []node.Variable  // F-021

	Contains   []edge.Contains  // F-018: Service→Module, Module→Module, Module→Endpoint/Function
	DependsOn  []edge.DependsOn // F-023: Service→Framework
	Invokes    []edge.Invokes   // F-019: Function→Call
	Targets    []edge.Targets   // F-019/F-020: Call→<resolvido>
	Uses       []edge.Uses      // F-019: Call→Framework
	Extends    []edge.Extends   // F-021: Type→Type (embedding)
	Aliases    []edge.Aliases   // F-021: Type→Type (alias)
	DefinedIn  []edge.DefinedIn // legado F-007, deprecated
	Edges      []edge.DefinedIn // alias para compat com chamadores existentes — mesmo conteúdo de DefinedIn
}

// Collect roda a pipeline completa sobre um repo:
//
//	walk → para cada Service: extract functions + endpoints + modules → emite edges
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
			repoRoot, found[i].AbsPath, svc.ModulePath, found[i].GoModule,
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

		// F-018: extrair Modules a partir dos packages observados nos fns/eps
		// + descoberta passiva via walker de diretórios com .go files.
		mods, modIdx, err := ExtractModules(
			repoRoot, found[i].AbsPath, svc.ModulePath, found[i].GoModule,
			svc.URN(), cfg.Repo, cfg.SkipPathSubstrings, emit,
		)
		if err != nil {
			return Result{}, fmt.Errorf("modules in %s: %w", svc.ModulePath, err)
		}
		// Linka Functions/Endpoints ao Module folha.
		for j := range fns {
			if m, ok := modIdx[fns[j].Namespace]; ok {
				fns[j].ModuleURN = m
			}
		}
		for j := range eps {
			// Endpoint usa o namespace derivado do diretório do arquivo
			// onde a rota foi registrada.
			ns := goNamespaceFromFile(found[i].GoModule, found[i].AbsPath, eps[j].Location.File, repoRoot)
			if m, ok := modIdx[ns]; ok {
				eps[j].ModuleURN = m
			}
		}

		res.Functions = append(res.Functions, fns...)
		res.Endpoints = append(res.Endpoints, eps...)
		res.Modules = append(res.Modules, mods...)

		// Contains edges (F-018):
		//   Service → Module(raiz do service)
		//   Module pai → Module filho (aninhamento)
		//   Module → Function/Endpoint (folha)
		for _, m := range mods {
			if m.ParentURN == "" {
				res.Contains = append(res.Contains, newContains(svc.URN(), m.URN(), cfg, emit))
			} else {
				res.Contains = append(res.Contains, newContains(m.ParentURN, m.URN(), cfg, emit))
			}
		}
		for _, fn := range fns {
			if fn.ModuleURN != "" {
				res.Contains = append(res.Contains, newContains(fn.ModuleURN, fn.URN(), cfg, emit))
			}
		}
		for _, ep := range eps {
			if ep.ModuleURN != "" {
				res.Contains = append(res.Contains, newContains(ep.ModuleURN, ep.URN(), cfg, emit))
			}
		}

		// F-023 — Frameworks via go.mod + DEPENDS_ON Service→Framework.
		fws, deps, err := ExtractFrameworks(repoRoot, found[i].AbsPath, emit)
		if err != nil {
			return Result{}, fmt.Errorf("frameworks in %s: %w", svc.ModulePath, err)
		}
		res.Frameworks = append(res.Frameworks, fws...)
		for k, fw := range fws {
			res.DependsOn = append(res.DependsOn, newServiceDependsOnFramework(svc.URN(), fw.URN(), deps[k], cfg, emit))
		}

		// F-021 — Types + Variables.
		ts, vs, exts, als, err := ExtractTypes(
			repoRoot, found[i].AbsPath, svc.ModulePath, found[i].GoModule,
			svc.URN(), cfg.Repo, funcFilter, emit,
		)
		if err != nil {
			return Result{}, fmt.Errorf("types in %s: %w", svc.ModulePath, err)
		}
		res.Types = append(res.Types, ts...)
		res.Variables = append(res.Variables, vs...)
		res.Extends = append(res.Extends, exts...)
		res.Aliases = append(res.Aliases, als...)

		// F-019/F-020 — boundary + in-process calls dentro das funções já extraídas.
		calls, invokes, targets, uses, err := ExtractCalls(
			repoRoot, found[i].AbsPath, svc.ModulePath,
			svc.URN(), cfg.Repo, cfg.SkipPathSubstrings, fns, emit,
		)
		if err != nil {
			return Result{}, fmt.Errorf("calls in %s: %w", svc.ModulePath, err)
		}
		res.Calls = append(res.Calls, calls...)
		res.Invokes = append(res.Invokes, invokes...)
		res.Targets = append(res.Targets, targets...)
		res.Uses = append(res.Uses, uses...)

		// DefinedIn edges (legado F-007, mantido para compat com queries
		// existentes e chamadores que ainda iteram res.Edges).
		for _, fn := range fns {
			d := newDefinedIn(fn.URN(), svc.URN(), cfg, emit)
			res.DefinedIn = append(res.DefinedIn, d)
			res.Edges = append(res.Edges, d)
		}
		for _, ep := range eps {
			d := newDefinedIn(ep.URN(), svc.URN(), cfg, emit)
			res.DefinedIn = append(res.DefinedIn, d)
			res.Edges = append(res.Edges, d)
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

func newContains(from, to node.URN, cfg Config, emit EmitOptions) edge.Contains {
	return edge.Contains{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeContains, to, cfg.ObservedAt),
			EdgeType: edge.TypeContains,
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

// goNamespaceFromFile recomputa o namespace a partir do path do arquivo
// relativo ao repo. Usado quando só temos `Location.File`.
func goNamespaceFromFile(goModule, serviceAbsPath, relFile, repoRoot string) string {
	if relFile == "" {
		return goModule
	}
	abs := filepath.Join(repoRoot, relFile)
	dir := filepath.Dir(abs)
	return goNamespace(goModule, serviceAbsPath, dir)
}

// ExtractModules percorre o service e descobre namespaces (packages
// Go). Retorna a lista de Modules e um índice namespace→URN para o
// caller wirear ModuleURN nos Functions/Endpoints.
//
// Para Go, um Module corresponde a um diretório com pelo menos um
// arquivo `.go`. Aninhamento é derivado do path: se existem packages
// em `internal/api` e `internal/api/users`, ambos viram Module com
// ParentURN apontando do filho para o pai.
func ExtractModules(
	repoRoot, serviceAbsPath, serviceModulePath, goModule string,
	serviceURN node.URN,
	repo string,
	skipPathSubstrings []string,
	emit EmitOptions,
) ([]node.Module, map[string]node.URN, error) {
	if skipPathSubstrings == nil {
		skipPathSubstrings = []string{"vendor/", "gen/", "mocks/"}
	}
	now := emit.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Coletar diretórios contendo .go files.
	dirs := map[string]string{} // namespace → short package name
	werr := filepath.WalkDir(serviceAbsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") && path != serviceAbsPath {
				return filepath.SkipDir
			}
			if path != serviceAbsPath {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		slash := filepath.ToSlash(path)
		for _, s := range skipPathSubstrings {
			if strings.Contains(slash, s) {
				return nil
			}
		}
		// Read package short name.
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
		if perr != nil {
			return nil
		}
		ns := goNamespace(goModule, serviceAbsPath, filepath.Dir(path))
		dirs[ns] = file.Name.Name
		// Adicionar ancestrais até a raiz (namespace == goModule).
		for parent := parentNamespace(ns); parent != "" && parent != goModule; parent = parentNamespace(parent) {
			if _, ok := dirs[parent]; !ok {
				dirs[parent] = lastSegment(parent)
			}
		}
		// Garante raiz do service como Module.
		if goModule != "" {
			if _, ok := dirs[goModule]; !ok {
				dirs[goModule] = lastSegment(goModule)
			}
		}
		return nil
	})
	if werr != nil {
		return nil, nil, fmt.Errorf("extract modules: %w", werr)
	}

	// Ordena para determinismo.
	keys := make([]string, 0, len(dirs))
	for k := range dirs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]node.Module, 0, len(keys))
	idx := make(map[string]node.URN, len(keys))
	for _, ns := range keys {
		urn := node.NewModuleURN(repo, serviceModulePath, ns)
		// Path relativo ao repo (folder).
		path := ""
		if goModule != "" && strings.HasPrefix(ns, goModule) {
			rel := strings.TrimPrefix(ns, goModule)
			rel = strings.TrimPrefix(rel, "/")
			path = filepath.ToSlash(filepath.Join(strings.TrimPrefix(filepath.ToSlash(serviceAbsPath), filepath.ToSlash(repoRoot)+"/"), rel))
			path = strings.TrimPrefix(path, "/")
		}

		var parent node.URN
		p := parentNamespace(ns)
		if _, ok := dirs[p]; ok && p != "" {
			parent = node.NewModuleURN(repo, serviceModulePath, p)
		}

		mod := node.Module{
			Base: node.Base{
				NodeURN:  urn,
				NodeKind: node.KindModule,
				NodeMeta: node.Meta{
					Version:    1,
					ValidFrom:  now,
					ObservedAt: now,
					Source: node.Source{
						Collector: "code/golang",
						RunID:     emit.RunID,
						Method:    node.MethodDeclared,
					},
					Confidence: 1.0,
				},
			},
			ServiceURN: serviceURN,
			ParentURN:  parent,
			Namespace:  ns,
			ShortName:  dirs[ns],
			Path:       path,
			Language:   "go",
		}
		out = append(out, mod)
		idx[ns] = urn
	}
	return out, idx, nil
}

// parentNamespace devolve o namespace pai (corta o último segmento `/`).
// Em Go, `a/b/c` → `a/b`. Retorna "" quando não há pai.
func parentNamespace(ns string) string {
	idx := strings.LastIndex(ns, "/")
	if idx <= 0 {
		return ""
	}
	return ns[:idx]
}

// lastSegment devolve o último segmento de um namespace `/`-separado.
func lastSegment(ns string) string {
	idx := strings.LastIndex(ns, "/")
	if idx < 0 {
		return ns
	}
	return ns[idx+1:]
}
