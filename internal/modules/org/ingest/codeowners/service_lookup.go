package codeowners

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// ServiceLookup descreve a estratégia para mapear `--repo=<path>` para
// uma `Service` no grafo. Convenção MVP (F-011 D2): 1 repo = 1 Service
// no módulo raiz (`module_path = "."`); URN canônica derivada do
// basename do path.
//
// Mono-repo (múltiplos Services indexados com o mesmo `Repo`) é
// detectável e gera um sinal de ambiguidade. O caller decide se aborta
// ou apenas relata e segue.
type ServiceLookup struct {
	Nodes repository.NodeRepository
}

// ServiceResolution carrega o resultado do lookup.
//
//   - Found != "" → URN única; emit OK.
//   - Ambiguous != nil → mono-repo detectado; emit deve ser pulado.
//   - Both empty → Service não existe no grafo (caller emite erro/skip).
type ServiceResolution struct {
	Found     node.URN
	Ambiguous []node.URN // URNs candidatas em caso de mono-repo
	Repo      string     // basename usado no lookup (sem path)
}

// ErrServiceNotFound é a forma do "Service não existe" — distingue de
// erros de I/O. Não é fatal por si só; o caller decide.
var ErrServiceNotFound = errors.New("codeowners: service not found for repo")

// ResolveForRepo aceita um caminho local de repo (`--repo=<path>`) e
// devolve qual Service URN receberá os edges `Owns`.
//
// Algoritmo:
//  1. Extrai basename (`/home/u/repos/payments` → `payments`).
//  2. Tenta `GetByURN(NewServiceURN(basename, "."))`. Hit → Found.
//  3. Caso contrário, lista Services correntes e filtra por `Repo`.
//     1 match → Found. >1 → Ambiguous. 0 → ErrServiceNotFound.
func (l *ServiceLookup) ResolveForRepo(ctx context.Context, repoPath string) (ServiceResolution, error) {
	basename := repoBasename(repoPath)
	if basename == "" {
		return ServiceResolution{}, fmt.Errorf("%w: empty repo path", repository.ErrInvalidArgument)
	}

	// Atalho: módulo raiz.
	urn := node.NewServiceURN(basename, ".")
	if _, err := l.Nodes.GetByURN(ctx, urn, repository.AsOf{}); err == nil {
		return ServiceResolution{Found: urn, Repo: basename}, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return ServiceResolution{}, fmt.Errorf("lookup root service: %w", err)
	}

	// Fallback: varre Services e filtra por Repo (cobre módulos
	// aninhados — `internal/api`, `cmd/cli`, etc.).
	all, err := l.Nodes.List(ctx, repository.NodeFilter{Kind: node.KindService})
	if err != nil {
		return ServiceResolution{}, fmt.Errorf("list services: %w", err)
	}
	var matches []node.URN
	for _, n := range all {
		s, ok := n.(node.Service)
		if !ok {
			continue
		}
		if s.Repo == basename {
			matches = append(matches, s.URN())
		}
	}
	switch len(matches) {
	case 0:
		return ServiceResolution{Repo: basename}, fmt.Errorf("%w: %s", ErrServiceNotFound, basename)
	case 1:
		return ServiceResolution{Found: matches[0], Repo: basename}, nil
	default:
		return ServiceResolution{Ambiguous: matches, Repo: basename}, nil
	}
}

// repoBasename extrai o último segmento do path, normalizando
// separadores e descartando trailing slash.
func repoBasename(p string) string {
	cleaned := filepath.ToSlash(strings.TrimRight(p, "/\\"))
	if cleaned == "" {
		return ""
	}
	if i := strings.LastIndex(cleaned, "/"); i >= 0 {
		return cleaned[i+1:]
	}
	return cleaned
}
