// Package python coleta `node.Framework` a partir de `requirements.txt`
// (pip / pip-tools) — F-023.
//
// Por que `requirements.txt` antes de `pyproject.toml`:
//   - Maioria dos repos Python ainda mantém um `requirements.txt`
//     (mesmo quando geram via Poetry / pip-compile).
//   - Stdlib basta — pyproject.toml exigiria parser TOML.
//
// Limitações conhecidas (MVP):
//   - Sem resolução de versões (`>=`, `~=`, `==` preservados como
//     literal em `LatestVersion`).
//   - `-r outro.txt` (include) NÃO é seguido — cada arquivo é uma
//     unidade.
//   - URLs / `-e .` editable installs ignorados (não são pacotes
//     PyPI canônicos).
//   - `extras_require` (e.g., `requests[security]`) — o `[security]`
//     é descartado, o pacote é `requests`.
package python

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Dependency é uma linha parseada do `requirements.txt`.
type Dependency struct {
	Name    string // ex: "django", "requests"
	Version string // ex: "==4.2.0", ">=2.28,<3" ou ""
	DevOnly bool   // sempre false aqui — requirements.txt não distingue
}

// Config controla a execução.
type Config struct {
	Repo       string
	ServiceURN node.URN // alvo do DEPENDS_ON; pode ser zero
	RunID      string
	ObservedAt time.Time

	// DevFiles lista filenames extras considerados dev-only
	// (e.g., "requirements-dev.txt", "requirements-test.txt"). Quando
	// uma dep vem desses, `IsDevOnly`/`!Hard` são setados.
	DevFiles []string
}

// Result agrega o que `Collect` produz.
type Result struct {
	Frameworks []node.Framework
	DependsOn  []edge.DependsOn
}

// Collect varre `root` por arquivos `requirements*.txt`. Dedup por URN.
func Collect(root string, cfg Config) (Result, error) {
	now := cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	devSet := map[string]bool{}
	for _, n := range cfg.DevFiles {
		devSet[n] = true
	}

	seen := map[node.URN]bool{}
	var fwks []node.Framework
	var deps []edge.DependsOn

	werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".venv" || name == "venv" {
				return filepath.SkipDir
			}
			return nil
		}
		base := d.Name()
		if !isRequirementsFile(base) {
			return nil
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return nil
		}
		defer f.Close()

		isDev := devSet[base]
		entries, perr := parseRequirements(f)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		for _, ent := range entries {
			ent.DevOnly = ent.DevOnly || isDev
			urn := node.NewFrameworkURN("pypi", ent.Name)
			if !seen[urn] {
				seen[urn] = true
				fwks = append(fwks, node.Framework{
					Base: node.Base{
						NodeURN:  urn,
						NodeKind: node.KindFramework,
						NodeMeta: node.Meta{
							Version:    1,
							ValidFrom:  now,
							ObservedAt: now,
							Source: node.Source{
								Collector: "code/python",
								RunID:     cfg.RunID,
								Method:    node.MethodDeclared,
							},
							Confidence: 1.0,
						},
					},
					Ecosystem:     "pypi",
					Name:          ent.Name,
					LatestVersion: ent.Version,
					IsDevOnly:     ent.DevOnly,
				})
			}
			if cfg.ServiceURN != "" {
				deps = append(deps, edge.DependsOn{
					Base: edge.Base{
						EdgeID:   edge.DeterministicID(cfg.ServiceURN, edge.TypeDependsOn, urn, now),
						EdgeType: edge.TypeDependsOn,
						FromURN:  cfg.ServiceURN,
						ToURN:    urn,
						EdgeMeta: edge.Meta{
							ValidFrom:   now,
							ObservedAt:  now,
							Source:      node.Source{Collector: "code/python", RunID: cfg.RunID, Method: node.MethodDeclared},
							Confidence:  1.0,
							Directional: true,
						},
					},
					Declared: true,
					Hard:     !ent.DevOnly,
				})
			}
		}
		return nil
	})
	if werr != nil {
		return Result{}, werr
	}
	return Result{Frameworks: fwks, DependsOn: deps}, nil
}

// isRequirementsFile reconhece variações comuns: requirements.txt,
// requirements-dev.txt, requirements-test.txt, dev-requirements.txt.
func isRequirementsFile(base string) bool {
	if !strings.HasSuffix(base, ".txt") {
		return false
	}
	lower := strings.ToLower(base)
	return strings.Contains(lower, "requirements")
}

// parseRequirements percorre o arquivo e devolve entradas válidas.
// Linhas com `-r`, `-e`, `--hash`, URLs (`git+`, `http`) e options
// (`--index-url`) são puladas com confidence implícita reduzida.
func parseRequirements(r io.Reader) ([]Dependency, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []Dependency
	for scanner.Scan() {
		line := stripComment(scanner.Text())
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "git+") || strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			continue
		}
		if d, ok := parseRequirementLine(line); ok {
			out = append(out, d)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseRequirementLine extrai (name, version) de formas como:
//   django==4.2.0
//   requests>=2.28,<3
//   numpy
//   requests[security]==2.31.0
func parseRequirementLine(line string) (Dependency, bool) {
	// Remove markers PEP 508 ("; python_version >= '3.8'").
	if i := strings.Index(line, ";"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if line == "" {
		return Dependency{}, false
	}
	// Encontra o primeiro operador de versão.
	idx := -1
	for _, op := range []string{"==", ">=", "<=", "~=", "!=", ">", "<"} {
		if i := strings.Index(line, op); i >= 0 {
			if idx == -1 || i < idx {
				idx = i
			}
		}
	}
	var name, version string
	if idx >= 0 {
		name = strings.TrimSpace(line[:idx])
		version = strings.TrimSpace(line[idx:])
	} else {
		name = strings.TrimSpace(line)
	}
	// Remove extras `[xxx]`.
	if i := strings.Index(name, "["); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	name = strings.ToLower(name)
	// Normaliza separadores `_` `.` → `-` (PEP 503 canonical name).
	name = strings.NewReplacer("_", "-", ".", "-").Replace(name)
	if name == "" {
		return Dependency{}, false
	}
	return Dependency{Name: name, Version: version}, true
}

// stripComment remove `#...` no fim da linha (não suporta # dentro de
// strings quotadas, raríssimo em requirements).
func stripComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}
