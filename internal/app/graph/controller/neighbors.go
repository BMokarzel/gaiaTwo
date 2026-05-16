package controller

import (
	"net/http"
	"sort"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

// handleNeighbors atende GET /v1/architecture/nodes/{urn}/neighbors.
//
// Query:
//   - depth=N (1..3; default 1)
//   - edge_types=CSV (whitelist; ignora desconhecidos silenciosamente)
//   - dir=in|out|any (default any)
//   - as_of=RFC3339
//
// Resposta:
//
//	{
//	  "edges": [...],
//	  "nodes": [...]   // nós alcançados (não inclui o start)
//	}
//
// Cap rígido de 100 resultados (≤ edges + ≤ nodes) — F-014.
func (c *Controller) handleNeighbors(w http.ResponseWriter, r *http.Request, start node.URN) {
	q := r.URL.Query()

	depth, err := parseDepth(q.Get("depth"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	dir, err := parseDirection(q.Get("dir"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	as, err := parseAsOf(q.Get("as_of"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	types := parseEdgeTypes(q.Get("edge_types"))

	filter := repository.EdgeFilter{Types: types, AsOf: as}

	// BFS expandindo Neighbors a cada nível. Dedup por edge.ID e por
	// URN. Cap total de 100 edges; paramos cedo se estourar.
	seenEdges := map[string]edge.Edge{}
	seenNodes := map[node.URN]struct{}{start: {}}
	frontier := []node.URN{start}

	for hop := 0; hop < depth; hop++ {
		next := []node.URN{}
		for _, urn := range frontier {
			edges, err := c.edges.Neighbors(r.Context(), urn, dir, filter)
			if err != nil {
				httpserver.WriteError(w, r, err)
				return
			}
			for _, e := range edges {
				if _, ok := seenEdges[e.ID()]; ok {
					continue
				}
				seenEdges[e.ID()] = e
				if len(seenEdges) >= maxNeighborsResults {
					break
				}
				peer := otherEnd(e, urn)
				if peer == "" {
					continue
				}
				if _, ok := seenNodes[peer]; !ok {
					seenNodes[peer] = struct{}{}
					next = append(next, peer)
				}
			}
			if len(seenEdges) >= maxNeighborsResults {
				break
			}
		}
		frontier = next
	}

	// Resolve URNs (sem start) em ordem determinística.
	urns := make([]node.URN, 0, len(seenNodes))
	for u := range seenNodes {
		if u == start {
			continue
		}
		urns = append(urns, u)
	}
	sort.Slice(urns, func(i, j int) bool { return urns[i] < urns[j] })

	nodeViews := make([]any, 0, len(urns))
	for _, u := range urns {
		n, err := c.nodes.GetByURN(r.Context(), u, as)
		if err != nil {
			// Edge aponta para nó sem versão visível (race com close).
			// Não falhamos o endpoint — apenas omitimos.
			continue
		}
		nodeViews = append(nodeViews, nodeView(n))
	}

	// Edges em ordem determinística por ID.
	edgeIDs := make([]string, 0, len(seenEdges))
	for id := range seenEdges {
		edgeIDs = append(edgeIDs, id)
	}
	sort.Strings(edgeIDs)
	edgeViews := make([]any, 0, len(edgeIDs))
	for _, id := range edgeIDs {
		edgeViews = append(edgeViews, edgeView(seenEdges[id]))
	}

	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"edges": edgeViews,
		"nodes": nodeViews,
	})
}
