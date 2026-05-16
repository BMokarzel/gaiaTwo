package codeowners

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// ResolvedOwner é o resultado de bater um handle bruto do CODEOWNERS
// contra o grafo: a URN canônica + o token original (para auditoria e
// para preservar ordem entre owners).
type ResolvedOwner struct {
	Handle  string   // token original do CODEOWNERS (ex.: "@alice")
	URN     node.URN // URN da Person ou do Team correspondente
	Kind    node.Kind
}

// Resolver mapeia handles do CODEOWNERS para URNs no grafo.
//
//   - `@org/team-slug` → `urn:ce:org:<tenant>:team/<slugified>` via
//     `GetByURN` (lookup direto).
//   - `@user` → URN da Person cujo `GithubHandle` (case-insensitive)
//     bate. Implementado com `List(Kind=Person)` + filtro em memória
//     porque a grafo do org é pequeno e a interface `Search` é
//     textual genérica (a busca via Search não garante match exato em
//     um campo específico — preferimos lookup determinístico).
//
// Estado: o índice de Persons é construído sob demanda no primeiro
// `Resolve` e reutilizado. Não compartilhe um Resolver entre tenants.
type Resolver struct {
	Nodes  repository.NodeRepository
	Tenant string

	personIdx map[string]node.URN // github_handle (lower) → URN
	personErr error
	indexed   bool
}

// Resolve bate cada owner em URN. Owners não-resolvíveis vão para
// `unresolved`. Ordem de saída segue a ordem de entrada, com dedup por
// URN: se dois handles distintos apontam para a mesma URN (improvável
// mas possível com aliases), apenas o primeiro entra.
func (r *Resolver) Resolve(ctx context.Context, owners []string) (resolved []ResolvedOwner, unresolved []string, err error) {
	seen := make(map[node.URN]bool, len(owners))
	for _, raw := range owners {
		handle := strings.TrimSpace(raw)
		if handle == "" {
			continue
		}
		owner, ok, lookupErr := r.resolveOne(ctx, handle)
		if lookupErr != nil {
			return nil, nil, lookupErr
		}
		if !ok {
			unresolved = append(unresolved, handle)
			continue
		}
		if seen[owner.URN] {
			continue
		}
		seen[owner.URN] = true
		resolved = append(resolved, owner)
	}
	return resolved, unresolved, nil
}

// resolveOne devolve (owner, true) se achou, (zero, false) se não-resolvível,
// (_, _, err) só em I/O fatal do repo.
func (r *Resolver) resolveOne(ctx context.Context, handle string) (ResolvedOwner, bool, error) {
	if !strings.HasPrefix(handle, "@") {
		return ResolvedOwner{}, false, nil
	}
	body := strings.TrimPrefix(handle, "@")
	if _, slug, ok := strings.Cut(body, "/"); ok {
		return r.resolveTeam(ctx, handle, slug)
	}
	return r.resolvePerson(ctx, handle, body)
}

func (r *Resolver) resolveTeam(ctx context.Context, handle, rawSlug string) (ResolvedOwner, bool, error) {
	slug := node.Slugify(rawSlug)
	if slug == "" {
		return ResolvedOwner{}, false, nil
	}
	urn := node.NewTeamURN(r.Tenant, slug)
	if _, err := r.Nodes.GetByURN(ctx, urn, repository.AsOf{}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ResolvedOwner{}, false, nil
		}
		return ResolvedOwner{}, false, fmt.Errorf("resolve team %q: %w", handle, err)
	}
	return ResolvedOwner{Handle: handle, URN: urn, Kind: node.KindTeam}, true, nil
}

func (r *Resolver) resolvePerson(ctx context.Context, handle, rawHandle string) (ResolvedOwner, bool, error) {
	if err := r.indexPersons(ctx); err != nil {
		return ResolvedOwner{}, false, err
	}
	norm := node.NormalizeGithubHandle(rawHandle)
	if norm == "" {
		return ResolvedOwner{}, false, nil
	}
	urn, ok := r.personIdx[norm]
	if !ok {
		return ResolvedOwner{}, false, nil
	}
	return ResolvedOwner{Handle: handle, URN: urn, Kind: node.KindPerson}, true, nil
}

// indexPersons carrega Persons correntes e indexa por GithubHandle
// normalizado. Pessoas sem handle não entram no índice (ficam
// não-resolvíveis no MVP — confere com F-011 critério "owner
// desconhecido vai para relatório").
func (r *Resolver) indexPersons(ctx context.Context) error {
	if r.indexed {
		return r.personErr
	}
	r.indexed = true
	persons, err := r.Nodes.List(ctx, repository.NodeFilter{Kind: node.KindPerson})
	if err != nil {
		r.personErr = fmt.Errorf("resolve: list persons: %w", err)
		return r.personErr
	}
	r.personIdx = make(map[string]node.URN, len(persons))
	for _, n := range persons {
		p, ok := n.(node.Person)
		if !ok {
			continue
		}
		if p.GithubHandle == "" {
			continue
		}
		h := node.NormalizeGithubHandle(p.GithubHandle)
		if h == "" {
			continue
		}
		// Primeiro a aparecer vence; conflito de handle é cenário
		// degenerado (HRIS deveria garantir unicidade) — não
		// abortamos, apenas mantemos determinismo simples.
		if _, exists := r.personIdx[h]; !exists {
			r.personIdx[h] = p.URN()
		}
	}
	return nil
}
