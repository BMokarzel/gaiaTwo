// Package memory é uma implementação de repository thread-safe em memória.
//
// Não substitui um backend real (n4j), mas:
//   - serve como backend de dev e de testes determinísticos;
//   - é o "oráculo" do contrato — qualquer outra impl deve se comportar igual;
//   - documenta a semântica bitemporal em código executável.
package memory

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Repo implementa NodeRepository e EdgeRepository.
type Repo struct {
	mu sync.RWMutex

	// nodes: URN → versões ordenadas por ValidFrom asc.
	nodes map[node.URN][]node.Node

	// externalIndex: (provider|externalID) → conjunto de URNs.
	// Keyed só por (provider, externalID) — o filtro por account
	// acontece no lookup (suporta F-003/S-005 cross-account).
	// Map de URN→struct{} para idempotência ao indexar a mesma URN
	// múltiplas vezes (Upsert reinserindo a mesma versão, ou aliases
	// como ARN+short que apontam para a mesma URN).
	externalIndex map[string]map[node.URN]struct{}

	// edges: edgeID → versões ordenadas por ValidFrom asc.
	edges map[string][]edge.Edge

	// edgesByFrom/edgesByTo: índices secundários (URN → edgeIDs).
	edgesByFrom map[node.URN]map[string]struct{}
	edgesByTo   map[node.URN]map[string]struct{}

	// now permite injeção de relógio em testes.
	now func() time.Time
}

// New cria um Repo vazio com relógio padrão (time.Now).
func New() *Repo {
	return &Repo{
		nodes:         map[node.URN][]node.Node{},
		externalIndex: map[string]map[node.URN]struct{}{},
		edges:         map[string][]edge.Edge{},
		edgesByFrom:   map[node.URN]map[string]struct{}{},
		edgesByTo:     map[node.URN]map[string]struct{}{},
		now:           time.Now,
	}
}

// WithClock substitui o relógio (útil em testes determinísticos).
func (r *Repo) WithClock(now func() time.Time) *Repo {
	r.now = now
	return r
}

// ----------------------------------------------------------------------------
// NodeRepository
// ----------------------------------------------------------------------------

// Upsert insere ou versiona um nó.
func (r *Repo) Upsert(ctx context.Context, n node.Node) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n == nil || n.URN() == "" {
		return fmt.Errorf("%w: nil node or empty URN", repository.ErrInvalidArgument)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	versions := r.nodes[n.URN()]
	now := r.now()

	// Fecha versão corrente, se houver.
	if i := currentVersionIdx(versions); i >= 0 {
		closed := closeNode(versions[i], now)
		versions[i] = closed
	}

	versions = append(versions, n)
	r.nodes[n.URN()] = versions

	// Atualiza índice externo se for Resource. F-003: indexamos por
	// (provider, externalID) — o filtro por account acontece no lookup,
	// para suportar wildcard cross-account (S-005). Se o externalID for
	// um ARN, também indexamos pelo short ID derivado (alias bidirecional
	// CUR usa formas mistas; ver F-003/S-002).
	if res, ok := n.(node.Resource); ok && res.ExternalID() != "" {
		r.indexExternal(res.Provider(), res.ExternalID(), n.URN())
		if short := shortIDFromExternal(res.ExternalID()); short != "" && short != res.ExternalID() {
			r.indexExternal(res.Provider(), short, n.URN())
		}
	}

	return nil
}

// indexExternal adiciona urn ao set associado à chave (provider, externalID).
func (r *Repo) indexExternal(p node.ProviderID, externalID string, urn node.URN) {
	key := externalKey(p, externalID)
	set, ok := r.externalIndex[key]
	if !ok {
		set = map[node.URN]struct{}{}
		r.externalIndex[key] = set
	}
	set[urn] = struct{}{}
}

func (r *Repo) GetByURN(ctx context.Context, urn node.URN, as repository.AsOf) (node.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.nodes[urn]
	if !ok {
		return nil, fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
	}

	if as.IsZero() {
		if i := currentVersionIdx(versions); i >= 0 {
			return versions[i], nil
		}
		return nil, fmt.Errorf("%w: URN %q has no current version", repository.ErrNotFound, urn)
	}
	if v, ok := versionAsOf(versions, as.Time()); ok {
		return v, nil
	}
	return nil, fmt.Errorf("%w: URN %q at %s", repository.ErrNotFound, urn, as.Time())
}

// GetByExternalID resolve um externalID (ARN ou short ID) → URN canônica.
//
// Semântica (ver repository.NodeRepository):
//   - account != "" → match estrito.
//   - account == "" → wildcard cross-account; ErrAmbiguous se mais de uma URN
//     matchea (provider, externalID).
//   - AsOf zero → versão corrente; caso contrário, exige versão visível em AsOf.
func (r *Repo) GetByExternalID(ctx context.Context, p node.ProviderID, account, externalID string, as repository.AsOf) (node.URN, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if externalID == "" {
		return "", fmt.Errorf("%w: empty externalID", repository.ErrInvalidArgument)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	set, ok := r.externalIndex[externalKey(p, externalID)]
	if !ok || len(set) == 0 {
		return "", fmt.Errorf("%w: external %s/%s/%s", repository.ErrNotFound, p, account, externalID)
	}

	// Coleta candidatos visíveis sob AsOf e filtrados por account, se dado.
	var matches []node.URN
	for urn := range set {
		versions, ok := r.nodes[urn]
		if !ok {
			continue
		}
		var pick node.Node
		if as.IsZero() {
			if i := currentVersionIdx(versions); i >= 0 {
				pick = versions[i]
			}
		} else {
			if v, ok := versionAsOf(versions, as.Time()); ok {
				pick = v
			}
		}
		if pick == nil {
			continue
		}
		if account != "" {
			res, ok := pick.(node.Resource)
			if !ok {
				continue
			}
			if accountIDFromURN(res.AccountURN()) != account {
				continue
			}
		}
		matches = append(matches, urn)
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: external %s/%s/%s", repository.ErrNotFound, p, account, externalID)
	case 1:
		return matches[0], nil
	default:
		if account != "" {
			// Caso raro: múltiplas URNs com o mesmo (provider, account, externalID).
			// Tratamos como ambíguo — o caller deve refinar a query.
			return "", fmt.Errorf("%w: external %s/%s/%s matched %d URNs",
				repository.ErrAmbiguous, p, account, externalID, len(matches))
		}
		return "", fmt.Errorf("%w: external %s/*/%s matched %d URNs (cross-account)",
			repository.ErrAmbiguous, p, externalID, len(matches))
	}
}

func (r *Repo) List(ctx context.Context, f repository.NodeFilter) ([]node.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]node.Node, 0, 16)
	for _, versions := range r.nodes {
		var (
			pick node.Node
			ok   bool
		)
		if f.AsOf.IsZero() {
			if i := currentVersionIdx(versions); i >= 0 {
				pick = versions[i]
				ok = true
			} else if f.IncludeInactive && len(versions) > 0 {
				// Sem versão corrente: cai para a última fechada.
				pick = versions[len(versions)-1]
				ok = true
			}
		} else {
			pick, ok = versionAsOf(versions, f.AsOf.Time())
		}
		if !ok {
			continue
		}
		if !matchesFilter(pick, f) {
			continue
		}
		out = append(out, pick)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })

	if f.Offset > 0 {
		if f.Offset >= len(out) {
			return nil, nil
		}
		out = out[f.Offset:]
	}
	if f.Limit > 0 && f.Limit < len(out) {
		out = out[:f.Limit]
	}
	return out, nil
}

func (r *Repo) History(ctx context.Context, urn node.URN) ([]node.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.nodes[urn]
	if !ok {
		return nil, fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
	}
	cp := make([]node.Node, len(versions))
	copy(cp, versions)
	return cp, nil
}

func (r *Repo) Delete(ctx context.Context, urn node.URN) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.nodes[urn]
	if !ok {
		return fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
	}
	i := currentVersionIdx(versions)
	if i < 0 {
		return fmt.Errorf("%w: URN %q has no current version", repository.ErrNotFound, urn)
	}
	versions[i] = closeNode(versions[i], r.now())
	r.nodes[urn] = versions
	return nil
}

// Search retorna nós correntes cujo URN ou campos textuais visíveis
// contenham Q (case-insensitive). Filtra por Kind se preenchido.
// Determinismo: ordem por URN crescente. Limit zero → cap em 100.
func (r *Repo) Search(ctx context.Context, q repository.SearchQuery) ([]node.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	needle := strings.TrimSpace(q.Q)
	if needle == "" {
		return nil, fmt.Errorf("%w: empty Q", repository.ErrInvalidArgument)
	}
	needle = strings.ToLower(needle)

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]node.Node, 0, 16)
	for _, versions := range r.nodes {
		i := currentVersionIdx(versions)
		if i < 0 {
			continue
		}
		n := versions[i]
		if q.Kind != "" && n.Kind() != q.Kind {
			continue
		}
		if !nodeMatchesText(n, needle) {
			continue
		}
		out = append(out, n)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].URN() < out[j].URN() })

	if q.Offset > 0 {
		if q.Offset >= len(out) {
			return nil, nil
		}
		out = out[q.Offset:]
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

// nodeMatchesText testa substring (case-insensitive já aplicado em needle)
// contra URN e campos textuais visíveis do nó: Repo/ModulePath em Service,
// ExternalID + tag Name em Resource.
func nodeMatchesText(n node.Node, needle string) bool {
	if strings.Contains(strings.ToLower(string(n.URN())), needle) {
		return true
	}
	switch v := n.(type) {
	case node.Service:
		if strings.Contains(strings.ToLower(v.Repo), needle) {
			return true
		}
		if strings.Contains(strings.ToLower(v.ModulePath), needle) {
			return true
		}
	}
	if res, ok := n.(node.Resource); ok {
		if strings.Contains(strings.ToLower(res.ExternalID()), needle) {
			return true
		}
		if name := res.NativeTags()["Name"]; name != "" &&
			strings.Contains(strings.ToLower(name), needle) {
			return true
		}
	}
	return false
}

// Touch atualiza apenas ObservedAt da versão corrente sem versionar.
func (r *Repo) Touch(ctx context.Context, urn node.URN) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.nodes[urn]
	if !ok {
		return fmt.Errorf("%w: URN %q", repository.ErrNotFound, urn)
	}
	i := currentVersionIdx(versions)
	if i < 0 {
		return fmt.Errorf("%w: URN %q has no current version", repository.ErrNotFound, urn)
	}
	versions[i] = touchNode(versions[i], r.now())
	r.nodes[urn] = versions
	return nil
}

// ----------------------------------------------------------------------------
// EdgeRepository
// ----------------------------------------------------------------------------

func (r *Repo) UpsertEdge(ctx context.Context, e edge.Edge, fromKind, toKind node.Kind) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.ID() == "" {
		return fmt.Errorf("%w: nil edge or empty ID", repository.ErrInvalidArgument)
	}
	if err := edge.Validate(e, fromKind, toKind); err != nil {
		return fmt.Errorf("%w: %v", repository.ErrInvalidArgument, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	versions := r.edges[e.ID()]
	if i := currentEdgeIdx(versions); i >= 0 {
		versions[i] = closeEdge(versions[i], now)
	}
	versions = append(versions, e)
	r.edges[e.ID()] = versions

	r.indexEdgeEndpoints(e)
	return nil
}

func (r *Repo) Between(ctx context.Context, from, to node.URN, f repository.EdgeFilter) ([]edge.Edge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := []edge.Edge{}
	for id := range r.edgesByFrom[from] {
		versions := r.edges[id]
		pick, ok := pickEdgeWith(versions, f.AsOf, f.IncludeInactive)
		if !ok || pick.To() != to {
			continue
		}
		if !edgeMatchesFilter(pick, f) {
			continue
		}
		out = append(out, pick)
	}
	return paginateEdges(out, f.Offset, f.Limit), nil
}

func (r *Repo) Neighbors(ctx context.Context, urn node.URN, dir repository.Direction, f repository.EdgeFilter) ([]edge.Edge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make(map[string]struct{})
	if dir == repository.DirOut || dir == repository.DirAny {
		for id := range r.edgesByFrom[urn] {
			ids[id] = struct{}{}
		}
	}
	if dir == repository.DirIn || dir == repository.DirAny {
		for id := range r.edgesByTo[urn] {
			ids[id] = struct{}{}
		}
	}

	out := make([]edge.Edge, 0, len(ids))
	for id := range ids {
		pick, ok := pickEdgeWith(r.edges[id], f.AsOf, f.IncludeInactive)
		if !ok || !edgeMatchesFilter(pick, f) {
			continue
		}
		out = append(out, pick)
	}
	return paginateEdges(out, f.Offset, f.Limit), nil
}

func (r *Repo) Traverse(ctx context.Context, start node.URN, dir repository.Direction, depth int, f repository.EdgeFilter) ([]node.URN, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth <= 0 {
		return nil, fmt.Errorf("%w: depth must be > 0", repository.ErrInvalidArgument)
	}

	visited := map[node.URN]struct{}{start: {}}
	frontier := []node.URN{start}
	out := []node.URN{}

	for d := 0; d < depth && len(frontier) > 0; d++ {
		next := []node.URN{}
		for _, cur := range frontier {
			edges, err := r.Neighbors(ctx, cur, dir, f)
			if err != nil {
				return nil, err
			}
			for _, e := range edges {
				peer := otherEndpoint(e, cur, dir)
				if _, seen := visited[peer]; seen || peer == "" {
					continue
				}
				visited[peer] = struct{}{}
				next = append(next, peer)
				out = append(out, peer)
			}
		}
		frontier = next
	}
	return out, nil
}

// Paths enumera caminhos de `from` para `to` com no máximo maxHops saltos.
// Direção: DirOut (segue arestas From→To, alinhado com semântica do grafo
// dirigido). Determinismo: ordena vizinhos por URN antes de expandir e
// retorna caminhos ordenados por (tamanho asc, URNs lexicograficamente).
// Cap: 100 caminhos, maxHops > 5 retorna ErrInvalidArgument (proteção
// contra explosão combinatória — ver F-014).
func (r *Repo) Paths(ctx context.Context, from, to node.URN, maxHops int, f repository.EdgeFilter) ([][]node.URN, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxHops <= 0 {
		return nil, fmt.Errorf("%w: maxHops must be > 0", repository.ErrInvalidArgument)
	}
	if maxHops > 5 {
		return nil, fmt.Errorf("%w: maxHops capped at 5 (combinatorial blast)", repository.ErrInvalidArgument)
	}
	if from == "" || to == "" {
		return nil, fmt.Errorf("%w: from/to required", repository.ErrInvalidArgument)
	}

	if from == to {
		return [][]node.URN{{from}}, nil
	}

	const maxPaths = 100
	out := [][]node.URN{}

	// BFS por níveis; cada item da queue é um caminho parcial. visited
	// permite revisitar (caminhos distintos podem compartilhar nós
	// intermediários); para evitar ciclos, checamos só dentro do path.
	type frame struct{ path []node.URN }
	queue := []frame{{path: []node.URN{from}}}

	for len(queue) > 0 && len(out) < maxPaths {
		fr := queue[0]
		queue = queue[1:]
		if len(fr.path) > maxHops { // path com k arestas tem k+1 nós; maxHops é em arestas.
			continue
		}
		cur := fr.path[len(fr.path)-1]

		edges, err := r.Neighbors(ctx, cur, repository.DirOut, f)
		if err != nil {
			return nil, err
		}
		// Determinismo: Neighbors já ordena por ID; pegamos peers em
		// ordem de URN para o output ser estável.
		peers := make([]node.URN, 0, len(edges))
		for _, e := range edges {
			peers = append(peers, e.To())
		}
		sort.Slice(peers, func(i, j int) bool { return peers[i] < peers[j] })

		for _, peer := range peers {
			// Evita ciclo dentro do path.
			if containsURN(fr.path, peer) {
				continue
			}
			newPath := append(append([]node.URN{}, fr.path...), peer)
			if peer == to {
				out = append(out, newPath)
				if len(out) >= maxPaths {
					break
				}
				continue
			}
			if len(newPath) <= maxHops { // pode expandir mais um hop
				queue = append(queue, frame{path: newPath})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		for k := range out[i] {
			if out[i][k] != out[j][k] {
				return out[i][k] < out[j][k]
			}
		}
		return false
	})
	return out, nil
}

func containsURN(s []node.URN, u node.URN) bool {
	for _, v := range s {
		if v == u {
			return true
		}
	}
	return false
}

func (r *Repo) DeleteEdge(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.edges[id]
	if !ok {
		return fmt.Errorf("%w: edge %q", repository.ErrNotFound, id)
	}
	i := currentEdgeIdx(versions)
	if i < 0 {
		return fmt.Errorf("%w: edge %q has no current version", repository.ErrNotFound, id)
	}
	versions[i] = closeEdge(versions[i], r.now())
	r.edges[id] = versions
	return nil
}

// Compile-time interface checks.
var (
	_ repository.NodeRepository = (*Repo)(nil)
	_ repository.EdgeRepository = edgeRepoView{}
)

// edgeRepoView é um adaptador para satisfazer EdgeRepository sem renomear
// métodos: Upsert/Delete colidem entre as duas interfaces, então o lado
// "edge" expõe métodos via composição.
type edgeRepoView struct{ *Repo }

func (e edgeRepoView) Upsert(ctx context.Context, ed edge.Edge, fromKind, toKind node.Kind) error {
	return e.Repo.UpsertEdge(ctx, ed, fromKind, toKind)
}

func (e edgeRepoView) Delete(ctx context.Context, id string) error {
	return e.Repo.DeleteEdge(ctx, id)
}

// AsEdgeRepo retorna a view que satisfaz EdgeRepository.
func (r *Repo) AsEdgeRepo() repository.EdgeRepository { return edgeRepoView{r} }

// ----------------------------------------------------------------------------
// helpers internos
// ----------------------------------------------------------------------------

func currentVersionIdx(vs []node.Node) int {
	for i := len(vs) - 1; i >= 0; i-- {
		if vs[i].Meta().IsCurrent() {
			return i
		}
	}
	return -1
}

func currentEdgeIdx(vs []edge.Edge) int {
	for i := len(vs) - 1; i >= 0; i-- {
		if vs[i].Meta().IsCurrent() {
			return i
		}
	}
	return -1
}

// closeNode retorna uma cópia (concreta) com ValidTo = at.
// Como Node é interface, dispatch por tipo concreto.
func closeNode(n node.Node, at time.Time) node.Node {
	switch v := n.(type) {
	case node.Compute:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Persistence:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Messaging:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Network:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Provider:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Account:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Region:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Zone:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Environment:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Service:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Endpoint:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Function:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Person:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Team:
		v.NodeMeta.ValidTo = &at
		return v
	case node.Squad:
		v.NodeMeta.ValidTo = &at
		return v
	default:
		// fallback: respeitamos o contrato Node mas não conseguimos mutar.
		// Implementações concretas externas devem ser registradas aqui.
		return n
	}
}

// touchNode retorna uma cópia (concreta) com ObservedAt = at, sem mexer em
// ValidFrom/ValidTo nem em Version. Dispatch por tipo concreto.
func touchNode(n node.Node, at time.Time) node.Node {
	switch v := n.(type) {
	case node.Compute:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Persistence:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Messaging:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Network:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Provider:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Account:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Region:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Zone:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Environment:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Service:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Endpoint:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Function:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Person:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Team:
		v.NodeMeta.ObservedAt = at
		return v
	case node.Squad:
		v.NodeMeta.ObservedAt = at
		return v
	default:
		return n
	}
}

func closeEdge(e edge.Edge, at time.Time) edge.Edge {
	switch v := e.(type) {
	case edge.Contains:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.DeployedOn:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.AttachedTo:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.Routes:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.Peers:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.DependsOn:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.CommunicatesWith:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.Replaces:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.DefinedIn:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.ServiceRunsOn:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.MemberOf:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.PartOf:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.ReportsTo:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.Owns:
		v.EdgeMeta.ValidTo = &at
		return v
	case edge.Base:
		v.EdgeMeta.ValidTo = &at
		return v
	default:
		return e
	}
}

func versionAsOf(vs []node.Node, t time.Time) (node.Node, bool) {
	for _, v := range vs {
		m := v.Meta()
		if !t.Before(m.ValidFrom) && (m.ValidTo == nil || t.Before(*m.ValidTo)) {
			return v, true
		}
	}
	return nil, false
}

func pickEdge(vs []edge.Edge, as repository.AsOf) (edge.Edge, bool) {
	return pickEdgeWith(vs, as, false)
}

// pickEdgeWith é o picker bitemporal de aresta. Quando includeInactive
// é true e AsOf é zero, devolve a última versão da aresta mesmo que
// fechada (ValidTo != nil) — usado para reconstituir adjacências
// historicamente válidas (F-015 include_inactive).
func pickEdgeWith(vs []edge.Edge, as repository.AsOf, includeInactive bool) (edge.Edge, bool) {
	if as.IsZero() {
		if i := currentEdgeIdx(vs); i >= 0 {
			return vs[i], true
		}
		if includeInactive && len(vs) > 0 {
			return vs[len(vs)-1], true
		}
		return nil, false
	}
	t := as.Time()
	for _, v := range vs {
		m := v.Meta()
		if !t.Before(m.ValidFrom) && (m.ValidTo == nil || t.Before(*m.ValidTo)) {
			return v, true
		}
	}
	return nil, false
}

func matchesFilter(n node.Node, f repository.NodeFilter) bool {
	if f.Kind != "" && n.Kind() != f.Kind {
		return false
	}
	if f.Provider != "" || f.AccountURN != "" || f.RegionURN != "" {
		res, ok := n.(node.Resource)
		if !ok {
			return false
		}
		if f.Provider != "" && res.Provider() != f.Provider {
			return false
		}
		if f.AccountURN != "" && res.AccountURN() != f.AccountURN {
			return false
		}
		if f.RegionURN != "" && res.RegionURN() != f.RegionURN {
			return false
		}
	}
	if len(f.Labels) > 0 {
		have := n.Meta().Labels
		for _, want := range f.Labels {
			if !slices.Contains(have, want) {
				return false
			}
		}
	}
	return true
}

func edgeMatchesFilter(e edge.Edge, f repository.EdgeFilter) bool {
	if len(f.Types) == 0 {
		return true
	}
	return slices.Contains(f.Types, e.Type())
}

func paginateEdges(in []edge.Edge, offset, limit int) []edge.Edge {
	sort.Slice(in, func(i, j int) bool { return in[i].ID() < in[j].ID() })
	if offset > 0 {
		if offset >= len(in) {
			return nil
		}
		in = in[offset:]
	}
	if limit > 0 && limit < len(in) {
		in = in[:limit]
	}
	return in
}

func otherEndpoint(e edge.Edge, cur node.URN, dir repository.Direction) node.URN {
	switch dir {
	case repository.DirOut:
		if e.From() == cur {
			return e.To()
		}
	case repository.DirIn:
		if e.To() == cur {
			return e.From()
		}
	case repository.DirAny:
		if e.From() == cur {
			return e.To()
		}
		if e.To() == cur {
			return e.From()
		}
	}
	return ""
}

func (r *Repo) indexEdgeEndpoints(e edge.Edge) {
	if _, ok := r.edgesByFrom[e.From()]; !ok {
		r.edgesByFrom[e.From()] = map[string]struct{}{}
	}
	r.edgesByFrom[e.From()][e.ID()] = struct{}{}

	if _, ok := r.edgesByTo[e.To()]; !ok {
		r.edgesByTo[e.To()] = map[string]struct{}{}
	}
	r.edgesByTo[e.To()][e.ID()] = struct{}{}
}

// externalKey é a chave do índice externo. Não inclui account — o filtro
// por account acontece no lookup, para suportar wildcard cross-account
// (F-003/S-005).
func externalKey(p node.ProviderID, externalID string) string {
	return string(p) + "|" + externalID
}

// shortIDFromExternal extrai o "short ID" canônico de um externalID que
// pode ser ARN. Convenção AWS: o último segmento depois de "/" ou ":" é
// o ID nativo (ex.: "i-abc123", "vol-xyz", "my-bucket").
//
// Retorna "" se o input não parece ARN (já é short ID ou formato
// não-reconhecido — nesse caso o caller não deve indexar alias).
//
// Exemplos:
//
//	arn:aws:ec2:us-east-1:111:instance/i-abc          → "i-abc"
//	arn:aws:s3:::my-bucket                            → "my-bucket"
//	arn:aws:rds:us-east-1:111:db:prod-db              → "prod-db"
//	i-abc                                             → ""  (já é short)
func shortIDFromExternal(eid string) string {
	if !strings.HasPrefix(eid, "arn:") {
		return ""
	}
	if i := strings.LastIndex(eid, "/"); i >= 0 && i < len(eid)-1 {
		return eid[i+1:]
	}
	// Sem "/", o ID está depois do último ":".
	if i := strings.LastIndex(eid, ":"); i >= 0 && i < len(eid)-1 {
		return eid[i+1:]
	}
	return ""
}

// accountIDFromURN extrai o segmento "account" de uma URN canônica.
// Em URNs malformadas retorna a string original (idempotência defensiva).
func accountIDFromURN(u node.URN) string {
	parts, err := node.ParseURN(u)
	if err != nil {
		// Pode já ser um account ID nu (ex.: "123") — tenta heurística simples.
		if i := strings.Index(string(u), ":account/"); i >= 0 {
			return string(u)[i+len(":account/"):]
		}
		return string(u)
	}
	// Quando a URN é a do próprio Account, ID é o account ID externo.
	if parts.Kind == node.KindAccount {
		return parts.ID
	}
	return parts.Account
}
