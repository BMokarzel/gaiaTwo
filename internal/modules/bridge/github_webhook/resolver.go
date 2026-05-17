package github_webhook

import (
	"context"
	"fmt"
	"path"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// ServiceLookup é a porta minima que o resolver consome do
// NodeRepository. Existe para isolar testes.
type ServiceLookup interface {
	List(ctx context.Context, f repository.NodeFilter) ([]node.Node, error)
}

// pathResolver carrega os Services correntes do repo e resolve cada
// path por longest-prefix-match contra `Service.ModulePath`.
//
// O `Service` raiz (ModulePath="." ou "") é o fallback natural — vence
// quando nenhum submódulo prefixa o path.
type pathResolver struct {
	services []serviceEntry
}

type serviceEntry struct {
	urn        node.URN
	modulePath string // normalizado: "" representa raiz
	depth      int    // número de segmentos — desempate de prefixos
}

// newPathResolver lê todos os Services correntes do repo e os indexa
// para lookup eficiente. Se o repo não tem Services no grafo, retorna
// resolver vazio (todos os paths → não-resolvíveis).
func newPathResolver(ctx context.Context, lookup ServiceLookup, repo string) (*pathResolver, error) {
	// Filter by Kind only; `Provider` filter in repository assumes
	// `node.Resource` implementations, and `Service` is a code-plane
	// node (não-Resource). Repo discrimination acontece logo abaixo.
	nodes, err := lookup.List(ctx, repository.NodeFilter{
		Kind: node.KindService,
	})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	var entries []serviceEntry
	for _, n := range nodes {
		svc, ok := n.(node.Service)
		if !ok {
			// Suporta backends que devolvem ponteiro.
			if p, ok2 := n.(*node.Service); ok2 {
				svc = *p
			} else {
				continue
			}
		}
		if svc.Repo != repo {
			continue
		}
		mp := normalizeModulePath(svc.ModulePath)
		entries = append(entries, serviceEntry{
			urn:        node.NewServiceURN(svc.Repo, svc.ModulePath),
			modulePath: mp,
			depth:      pathDepth(mp),
		})
	}
	return &pathResolver{services: entries}, nil
}

// Resolve mapeia uma lista de paths para o conjunto de URNs distintas
// de Services tocados. Paths não-resolvíveis são silenciosamente
// descartados (o caller decide se loga warning).
func (r *pathResolver) Resolve(paths []string) []node.URN {
	seen := make(map[node.URN]struct{})
	for _, p := range paths {
		if urn, ok := r.resolveOne(p); ok {
			seen[urn] = struct{}{}
		}
	}
	out := make([]node.URN, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	return out
}

func (r *pathResolver) resolveOne(filePath string) (node.URN, bool) {
	clean := path.Clean(filePath)
	if clean == "." || clean == "/" {
		return r.matchRoot()
	}
	var best *serviceEntry
	for i := range r.services {
		e := &r.services[i]
		if !pathHasPrefix(clean, e.modulePath) {
			continue
		}
		if best == nil || e.depth > best.depth {
			best = e
		}
	}
	if best == nil {
		return "", false
	}
	return best.urn, true
}

func (r *pathResolver) matchRoot() (node.URN, bool) {
	for i := range r.services {
		if r.services[i].modulePath == "" {
			return r.services[i].urn, true
		}
	}
	return "", false
}

// normalizeModulePath padroniza o prefixo de comparação. `.` e `""`
// → "" (raiz). Sem barras finais.
func normalizeModulePath(mp string) string {
	mp = strings.TrimSpace(mp)
	if mp == "" || mp == "." {
		return ""
	}
	return strings.Trim(mp, "/")
}

func pathDepth(mp string) int {
	if mp == "" {
		return 0
	}
	return strings.Count(mp, "/") + 1
}

// pathHasPrefix devolve true se `path` está sob `prefix` (segment-aware).
// Prefix vazio é raiz e dá match em qualquer path.
func pathHasPrefix(path, prefix string) bool {
	if prefix == "" {
		return true
	}
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}
