package golang

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Dependency é um require parseado do `go.mod`.
type Dependency struct {
	Path     string // "github.com/go-chi/chi/v5"
	Version  string // "v5.0.10"
	Indirect bool   // marcador `// indirect`
}

// ExtractFrameworks lê o `go.mod` do service e devolve um Framework
// global por dependência declarada em `require`. Stdlib-only (sem
// `golang.org/x/mod/modfile`) — decisão F-007 mantida.
//
// Não há diferença Direct/Indirect em URN: ambos viram o mesmo nó
// Framework (idempotente). O caller decide se filtra indirect.
func ExtractFrameworks(
	repoRoot, serviceAbsPath string,
	emit EmitOptions,
) ([]node.Framework, []Dependency, error) {
	goModPath := filepath.Join(serviceAbsPath, "go.mod")
	deps, err := parseRequires(goModPath)
	if err != nil {
		return nil, nil, fmt.Errorf("framework parse %s: %w", goModPath, err)
	}
	now := emit.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := make([]node.Framework, 0, len(deps))
	for _, d := range deps {
		urn := node.NewFrameworkURN("go", d.Path)
		out = append(out, node.Framework{
			Base: node.Base{
				NodeURN:  urn,
				NodeKind: node.KindFramework,
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
			Ecosystem:     "go",
			Name:          d.Path,
			LatestVersion: d.Version,
			IsDevOnly:     d.Indirect,
		})
	}
	return out, deps, nil
}

// parseRequires parseia blocos `require` de um `go.mod`. Suporta as
// duas formas:
//
//	require foo v1.2.3
//	require (
//	    foo v1.2.3
//	    bar v0.5.0 // indirect
//	)
//
// `replace` e `exclude` são ignorados — não alteram a lista de
// dependências declaradas.
func parseRequires(goModPath string) ([]Dependency, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var deps []Dependency
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inBlock := false
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			if d, ok := parseRequireLine(line); ok {
				deps = append(deps, d)
			}
			continue
		}
		if strings.HasPrefix(line, "require") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "require"))
			if rest == "(" {
				inBlock = true
				continue
			}
			if d, ok := parseRequireLine(rest); ok {
				deps = append(deps, d)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return deps, nil
}

// parseRequireLine extrai `<path> <version> [// indirect]`. Comentário
// inline é tolerado em qualquer posição após a versão.
func parseRequireLine(line string) (Dependency, bool) {
	indirect := false
	if i := strings.Index(line, "//"); i >= 0 {
		comment := strings.TrimSpace(line[i+2:])
		if strings.HasPrefix(comment, "indirect") {
			indirect = true
		}
		line = strings.TrimSpace(line[:i])
	}
	if line == "" {
		return Dependency{}, false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Dependency{}, false
	}
	return Dependency{Path: fields[0], Version: fields[1], Indirect: indirect}, true
}

// newServiceDependsOnFramework monta a aresta DEPENDS_ON Service→Framework.
func newServiceDependsOnFramework(serviceURN, frameworkURN node.URN, d Dependency, cfg Config, emit EmitOptions) edge.DependsOn {
	return edge.DependsOn{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(serviceURN, edge.TypeDependsOn, frameworkURN, cfg.ObservedAt),
			EdgeType: edge.TypeDependsOn,
			FromURN:  serviceURN,
			ToURN:    frameworkURN,
			EdgeMeta: edge.Meta{
				ValidFrom:   cfg.ObservedAt,
				ObservedAt:  cfg.ObservedAt,
				Source:      node.Source{Collector: "code/golang", RunID: emit.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
			},
		},
		Declared: true,
		Hard:     !d.Indirect,
	}
}
