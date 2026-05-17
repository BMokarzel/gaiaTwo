// Package npm coleta `node.Framework` a partir de arquivos
// `package.json` (npm/yarn/pnpm) — F-023.
//
// Por que um pacote separado:
//   - Cada ecossistema (Go, npm, PyPI, Cargo) tem semântica de
//     versão e categorização própria. Misturar gera switches ruins.
//   - Permite testar isolado e expandir cobertura incrementalmente.
//
// Limitações conhecidas (MVP):
//   - Não resolve `workspaces` recursivamente (cada `package.json`
//     vira sua própria coleção).
//   - Lockfiles (`package-lock.json`, `yarn.lock`) ignorados — só
//     declarado, não transitivo.
//   - SPDX license lookup é F-023 backlog.
package npm

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Dependency é uma entrada parseada do `package.json`.
type Dependency struct {
	Name    string // ex: "react", "@types/node"
	Version string // ex: "^18.2.0"
	DevOnly bool   // true se veio de devDependencies/peerDependencies
}

// Config controla a execução.
type Config struct {
	Repo       string
	ServiceURN node.URN // alvo do DEPENDS_ON; pode ser zero (só Frameworks)
	RunID      string
	ObservedAt time.Time
}

// Result agrega o que `Collect` produz.
type Result struct {
	Frameworks []node.Framework
	DependsOn  []edge.DependsOn // pode estar vazio se ServiceURN é zero
}

// packageJSON é o subset que o MVP lê.
type packageJSON struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

// Collect percorre `root` à procura de `package.json` e emite um
// Framework por dependência declarada. Dedup por URN (mesmo pacote
// em dois manifests vira um único Framework).
func Collect(root string, cfg Config) (Result, error) {
	now := cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
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
			if name == "node_modules" || name == "vendor" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}
		raw, oerr := os.ReadFile(path)
		if oerr != nil {
			return nil
		}
		var pkg packageJSON
		if jerr := json.Unmarshal(raw, &pkg); jerr != nil {
			return fmt.Errorf("parse %s: %w", path, jerr)
		}
		for _, ent := range allDeps(pkg) {
			urn := node.NewFrameworkURN("npm", ent.Name)
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
								Collector: "code/npm",
								RunID:     cfg.RunID,
								Method:    node.MethodDeclared,
							},
							Confidence: 1.0,
						},
					},
					Ecosystem:     "npm",
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
							Source:      node.Source{Collector: "code/npm", RunID: cfg.RunID, Method: node.MethodDeclared},
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

// allDeps consolida as 4 seções em ordem determinística. DevOnly cobre
// dev/peer/optional — só `dependencies` é runtime real.
func allDeps(p packageJSON) []Dependency {
	var out []Dependency
	for name, v := range p.Dependencies {
		out = append(out, Dependency{Name: name, Version: v, DevOnly: false})
	}
	for name, v := range p.DevDependencies {
		out = append(out, Dependency{Name: name, Version: v, DevOnly: true})
	}
	for name, v := range p.PeerDependencies {
		out = append(out, Dependency{Name: name, Version: v, DevOnly: true})
	}
	for name, v := range p.OptionalDependencies {
		out = append(out, Dependency{Name: name, Version: v, DevOnly: true})
	}
	// Sort estável por nome para idempotência.
	sortByName(out)
	return out
}

func sortByName(deps []Dependency) {
	// Insertion sort suficiente — listas curtas em manifests reais.
	for i := 1; i < len(deps); i++ {
		for j := i; j > 0 && strings.Compare(deps[j-1].Name, deps[j].Name) > 0; j-- {
			deps[j-1], deps[j] = deps[j], deps[j-1]
		}
	}
}
