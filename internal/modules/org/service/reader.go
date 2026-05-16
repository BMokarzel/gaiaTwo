package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
	"costEngine/internal/repository"
)

// reader é a implementação de `org.Service` (F-016 S-004). Convive com
// Writer no mesmo pacote: Writer é o caminho de ingestão (HRIS CSV →
// grafo) e reader é o caminho de leitura. Mantê-los em pacotes
// diferentes obrigaria duplicar (a) testes de fixture e (b)
// dependências do repo.
//
// O nome lowercase segue a convenção do plano F-016: a `New` retorna
// `org.Service`, não `*reader`.
type reader struct {
	nodes repository.NodeRepository
	edges repository.EdgeRepository
}

// New constrói uma `org.Service` operando sobre os repos passados.
// Pánica em repo nil — não há cenário válido para isso (fail-fast).
func New(nodes repository.NodeRepository, edges repository.EdgeRepository) org.Service {
	if nodes == nil || edges == nil {
		panic("org/service: New requires non-nil repos")
	}
	return &reader{nodes: nodes, edges: edges}
}

// ----------------------------------------------------------------------------
// Teams
// ----------------------------------------------------------------------------

func (r *reader) ListTeams(ctx context.Context, q org.ListTeamsQuery) (org.Page[org.TeamSummary], error) {
	// limit+1 para detectar próxima página sem count separado (mesma
	// técnica do controller atual).
	raw, err := r.nodes.List(ctx, repository.NodeFilter{
		Kind:            node.KindTeam,
		Limit:           q.Limit + 1,
		Offset:          q.Offset,
		AsOf:            q.AsOf,
		IncludeInactive: q.IncludeInactive,
	})
	if err != nil {
		return org.Page[org.TeamSummary]{}, err
	}
	nextOffset := 0
	if len(raw) > q.Limit {
		raw = raw[:q.Limit]
		nextOffset = q.Offset + q.Limit
	}
	items := make([]org.TeamSummary, 0, len(raw))
	for _, n := range raw {
		t, ok := n.(node.Team)
		if !ok {
			continue
		}
		sqCount, _ := r.countSquadsOfTeam(ctx, t.URN(), q.AsOfOptions)
		ppCount, _ := r.countPersonsOfTeam(ctx, t.URN(), q.AsOfOptions)
		items = append(items, org.TeamSummary{
			Team: t, SquadCount: sqCount, PersonCount: ppCount,
		})
	}
	return org.Page[org.TeamSummary]{
		Items: items, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

func (r *reader) GetTeam(ctx context.Context, urn node.URN, opts org.AsOfOptions) (org.TeamDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return org.TeamDetail{}, &org.ErrTeamNotFound{URN: urn}
		}
		return org.TeamDetail{}, err
	}
	t, ok := n.(node.Team)
	if !ok {
		return org.TeamDetail{}, &org.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Team"}
	}
	squads, err := r.squadsOfTeam(ctx, urn, opts)
	if err != nil {
		return org.TeamDetail{}, err
	}
	ppCount, err := r.countPersonsAcrossSquads(ctx, squads, opts)
	if err != nil {
		return org.TeamDetail{}, err
	}
	return org.TeamDetail{
		TeamSummary: org.TeamSummary{
			Team: t, SquadCount: len(squads), PersonCount: ppCount,
		},
		Squads: squads,
	}, nil
}

// ----------------------------------------------------------------------------
// Squads
// ----------------------------------------------------------------------------

func (r *reader) ListSquadsOfTeam(ctx context.Context, team node.URN, q org.ListSquadsQuery) (org.Page[org.SquadDetail], error) {
	// Verificar existência do team antes de listar — 404 vs página vazia.
	if _, err := r.fetchNode(ctx, team, q.AsOfOptions); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return org.Page[org.SquadDetail]{}, &org.ErrTeamNotFound{URN: team}
		}
		return org.Page[org.SquadDetail]{}, err
	}
	urns, err := r.squadsOfTeam(ctx, team, q.AsOfOptions)
	if err != nil {
		return org.Page[org.SquadDetail]{}, err
	}
	start := q.Offset
	if start > len(urns) {
		start = len(urns)
	}
	end := start + q.Limit
	nextOffset := 0
	if end < len(urns) {
		nextOffset = end
	} else {
		end = len(urns)
	}
	page := urns[start:end]
	items := make([]org.SquadDetail, 0, len(page))
	for _, u := range page {
		n, err := r.fetchNode(ctx, u, q.AsOfOptions)
		if err != nil {
			// Edge aponta para squad sem versão visível: omite.
			continue
		}
		sq, ok := n.(node.Squad)
		if !ok {
			continue
		}
		mc, _ := r.countPersonsAcrossSquads(ctx, []node.URN{sq.URN()}, q.AsOfOptions)
		items = append(items, org.SquadDetail{Squad: sq, MemberCount: mc})
	}
	return org.Page[org.SquadDetail]{
		Items: items, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

func (r *reader) GetSquad(ctx context.Context, urn node.URN, opts org.AsOfOptions) (org.SquadDetail, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return org.SquadDetail{}, &org.ErrSquadNotFound{URN: urn}
		}
		return org.SquadDetail{}, err
	}
	sq, ok := n.(node.Squad)
	if !ok {
		return org.SquadDetail{}, &org.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Squad"}
	}
	mc, err := r.countPersonsAcrossSquads(ctx, []node.URN{urn}, opts)
	if err != nil {
		return org.SquadDetail{}, err
	}
	return org.SquadDetail{Squad: sq, MemberCount: mc}, nil
}

// ----------------------------------------------------------------------------
// People
// ----------------------------------------------------------------------------

func (r *reader) ListSquadMembers(ctx context.Context, squad node.URN, q org.ListMembersQuery) (org.Page[org.PersonProfile], error) {
	if _, err := r.fetchNode(ctx, squad, q.AsOfOptions); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return org.Page[org.PersonProfile]{}, &org.ErrSquadNotFound{URN: squad}
		}
		return org.Page[org.PersonProfile]{}, err
	}
	urns, err := r.personsOfSquad(ctx, squad, q.AsOfOptions)
	if err != nil {
		return org.Page[org.PersonProfile]{}, err
	}
	start := q.Offset
	if start > len(urns) {
		start = len(urns)
	}
	end := start + q.Limit
	nextOffset := 0
	if end < len(urns) {
		nextOffset = end
	} else {
		end = len(urns)
	}
	page := urns[start:end]
	items := make([]org.PersonProfile, 0, len(page))
	for _, u := range page {
		n, err := r.fetchNode(ctx, u, q.AsOfOptions)
		if err != nil {
			continue
		}
		p, ok := n.(node.Person)
		if !ok {
			continue
		}
		items = append(items, org.PersonProfile{Person: p})
	}
	return org.Page[org.PersonProfile]{
		Items: items, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

func (r *reader) GetPerson(ctx context.Context, urn node.URN, opts org.AsOfOptions) (org.PersonProfile, error) {
	n, err := r.fetchNode(ctx, urn, opts)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return org.PersonProfile{}, &org.ErrPersonNotFound{URN: urn}
		}
		return org.PersonProfile{}, err
	}
	p, ok := n.(node.Person)
	if !ok {
		return org.PersonProfile{}, &org.ErrInvalidURN{URN: urn, Reason: "URN does not reference a Person"}
	}
	teamURN := node.URN("")
	if p.SquadURN != "" {
		if sn, err := r.fetchNode(ctx, p.SquadURN, opts); err == nil {
			if sq, ok := sn.(node.Squad); ok {
				teamURN = sq.TeamURN
			}
		}
	}
	return org.PersonProfile{Person: p, TeamURN: teamURN}, nil
}

func (r *reader) ListReports(ctx context.Context, root node.URN, q org.ListReportsQuery) (org.ReportsTree, error) {
	n, err := r.fetchNode(ctx, root, q.AsOfOptions)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return org.ReportsTree{}, &org.ErrPersonNotFound{URN: root}
		}
		return org.ReportsTree{}, err
	}
	p, ok := n.(node.Person)
	if !ok {
		return org.ReportsTree{}, &org.ErrInvalidURN{URN: root, Reason: "URN does not reference a Person"}
	}

	visited := map[node.URN]struct{}{root: {}}
	truncated := false
	tree, err := r.buildReportsSubtree(ctx, root, q.Depth, q.AsOfOptions, visited, &truncated)
	if err != nil {
		return org.ReportsTree{}, err
	}
	return org.ReportsTree{
		Root: p, Reports: tree, Depth: q.Depth, Truncated: truncated,
	}, nil
}

func (r *reader) buildReportsSubtree(
	ctx context.Context, manager node.URN, remaining int,
	opts org.AsOfOptions,
	visited map[node.URN]struct{}, truncated *bool,
) ([]org.ReportNode, error) {
	if remaining <= 0 {
		return []org.ReportNode{}, nil
	}
	edges, err := r.edges.Neighbors(ctx, manager, repository.DirIn, repository.EdgeFilter{
		Types:           []edge.Type{edge.TypeReportsTo},
		AsOf:            opts.AsOf,
		IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return nil, err
	}
	urns := make([]node.URN, 0, len(edges))
	seen := make(map[node.URN]struct{}, len(edges))
	for _, e := range edges {
		peer := e.From()
		if peer == "" || peer == manager {
			continue
		}
		if _, dup := seen[peer]; dup {
			continue
		}
		seen[peer] = struct{}{}
		if _, cyc := visited[peer]; cyc {
			*truncated = true
			continue
		}
		urns = append(urns, peer)
	}
	sort.Slice(urns, func(i, j int) bool { return urns[i] < urns[j] })

	out := make([]org.ReportNode, 0, len(urns))
	for _, u := range urns {
		visited[u] = struct{}{}
		n, err := r.fetchNode(ctx, u, opts)
		if err != nil {
			continue
		}
		p, ok := n.(node.Person)
		if !ok {
			continue
		}
		children, err := r.buildReportsSubtree(ctx, u, remaining-1, opts, visited, truncated)
		if err != nil {
			return nil, err
		}
		out = append(out, org.ReportNode{Person: p, Reports: children})
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Search
// ----------------------------------------------------------------------------

func (r *reader) Search(ctx context.Context, q org.SearchQuery) (org.SearchResults, error) {
	if strings.TrimSpace(q.Q) == "" {
		return org.SearchResults{}, &org.ErrInvalidURN{Reason: "missing q"}
	}
	// org.Search é restrito aos Kinds do plano org. Se Kind for vazio,
	// o caller (controller) decide se quer cross-plane (vai por
	// app/search) ou explicito org-only — aqui só repassa.
	results, err := r.nodes.Search(ctx, repository.SearchQuery{
		Q:      q.Q,
		Kind:   q.Kind,
		Limit:  q.Limit + 1,
		Offset: q.Offset,
	})
	if err != nil {
		return org.SearchResults{}, err
	}
	nextOffset := 0
	if len(results) > q.Limit {
		results = results[:q.Limit]
		nextOffset = q.Offset + q.Limit
	}
	return org.SearchResults{
		Items: results, Limit: q.Limit, NextOffset: nextOffset,
	}, nil
}

// ----------------------------------------------------------------------------
// Helpers internos (traversals)
// ----------------------------------------------------------------------------

// fetchNode busca um nó por URN, com fallback histórico quando
// IncludeInactive=true (mesma semântica do controller atual em v1).
func (r *reader) fetchNode(ctx context.Context, urn node.URN, opts org.AsOfOptions) (node.Node, error) {
	n, err := r.nodes.GetByURN(ctx, urn, opts.AsOf)
	if err == nil {
		return n, nil
	}
	if !opts.IncludeInactive || !errors.Is(err, repository.ErrNotFound) || !opts.AsOf.IsZero() {
		return nil, err
	}
	history, herr := r.nodes.History(ctx, urn)
	if herr != nil || len(history) == 0 {
		return nil, err
	}
	return history[len(history)-1], nil
}

// squadsOfTeam: URNs de Squad com PartOf(Squad → Team) corrente, ASC.
func (r *reader) squadsOfTeam(ctx context.Context, team node.URN, opts org.AsOfOptions) ([]node.URN, error) {
	edges, err := r.edges.Neighbors(ctx, team, repository.DirIn, repository.EdgeFilter{
		Types: []edge.Type{edge.TypePartOf}, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return nil, err
	}
	out := make([]node.URN, 0, len(edges))
	seen := make(map[node.URN]struct{}, len(edges))
	for _, e := range edges {
		peer := e.From()
		if peer == "" || peer == team {
			continue
		}
		if _, ok := seen[peer]; ok {
			continue
		}
		seen[peer] = struct{}{}
		out = append(out, peer)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *reader) countSquadsOfTeam(ctx context.Context, team node.URN, opts org.AsOfOptions) (int, error) {
	sq, err := r.squadsOfTeam(ctx, team, opts)
	if err != nil {
		return 0, err
	}
	return len(sq), nil
}

// personsOfSquad: URNs de Person ligadas via MemberOf, corrente, ASC.
func (r *reader) personsOfSquad(ctx context.Context, squad node.URN, opts org.AsOfOptions) ([]node.URN, error) {
	edges, err := r.edges.Neighbors(ctx, squad, repository.DirIn, repository.EdgeFilter{
		Types: []edge.Type{edge.TypeMemberOf}, AsOf: opts.AsOf, IncludeInactive: opts.IncludeInactive,
	})
	if err != nil {
		return nil, err
	}
	out := make([]node.URN, 0, len(edges))
	seen := make(map[node.URN]struct{}, len(edges))
	for _, e := range edges {
		peer := e.From()
		if peer == "" || peer == squad {
			continue
		}
		if _, ok := seen[peer]; ok {
			continue
		}
		seen[peer] = struct{}{}
		out = append(out, peer)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *reader) countPersonsOfTeam(ctx context.Context, team node.URN, opts org.AsOfOptions) (int, error) {
	sq, err := r.squadsOfTeam(ctx, team, opts)
	if err != nil {
		return 0, err
	}
	return r.countPersonsAcrossSquads(ctx, sq, opts)
}

func (r *reader) countPersonsAcrossSquads(ctx context.Context, squads []node.URN, opts org.AsOfOptions) (int, error) {
	seen := make(map[node.URN]struct{})
	for _, sq := range squads {
		pp, err := r.personsOfSquad(ctx, sq, opts)
		if err != nil {
			return 0, err
		}
		for _, p := range pp {
			seen[p] = struct{}{}
		}
	}
	return len(seen), nil
}
