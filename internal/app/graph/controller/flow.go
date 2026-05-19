package controller

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
)

func strconvAtoiPositive(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, errors.New("non-positive")
	}
	return n, nil
}

// maxFlowResults é o cap de nós/edges retornados por /flow.
// Mais largo que /neighbors porque o objetivo é mostrar o sub-grafo
// completo de um Endpoint (handler + chamadas transitivas + tipos +
// frameworks). 500 é confortável para o react-flow renderizar.
const (
	maxFlowResults = 500
	defaultFlowDepth = 4
	maxFlowDepth     = 6
)

// flowEdgeTypes é o whitelist de edges seguidas pelo BFS de /flow.
// Cobre o que faz sentido para visualizar o fluxo de um Endpoint:
//
//   - CONTAINS: navegar pra Service/Module pais e pra Function filhas
//   - INVOKES: Function → Call
//   - TARGETS: Call → Function/Endpoint/Persistence/...
//   - USES: Call|Module|Service → Framework
//   - DEPENDS_ON: Service → Framework
//   - DEFINED_IN: legado F-007 (Endpoint/Function → Service)
//   - IMPLEMENTS/EXTENDS/ALIASES: relações de tipo
//   - SERIALIZES_AS: Type → Schema
var flowEdgeTypes = []edge.Type{
	edge.TypeContains,
	edge.TypeInvokes,
	edge.TypeTargets,
	edge.TypeUses,
	edge.TypeDependsOn,
	edge.TypeDefinedIn,
	edge.TypeImplements,
	edge.TypeExtends,
	edge.TypeAliases,
	edge.TypeSerializesAs,
}

// handleFlow atende GET /v1/architecture/nodes/{urn}/flow.
//
// Faz BFS bidirecional (depth N) a partir do nó-alvo, seguindo um
// whitelist de edges relevantes para visualização de fluxo de código
// (CONTAINS/INVOKES/TARGETS/USES/...). A resposta agrupa os nós por
// `kind` para facilitar o consumo no front-end.
//
// Query:
//   - depth=N (1..maxFlowDepth; default 4)
//   - as_of=RFC3339
//
// Resposta:
//
//	{
//	  "root": "<urn>",
//	  "nodes": {
//	    "service":   [...], "module":   [...],
//	    "endpoint":  [...], "function": [...],
//	    "call":      [...], "type":     [...],
//	    "variable":  [...], "framework":[...],
//	    "other":     [...]
//	  },
//	  "edges": [...]
//	}
func (c *Controller) handleFlow(w http.ResponseWriter, r *http.Request, start node.URN) {
	q := r.URL.Query()

	depth, err := parseDepthCustom(q.Get("depth"), defaultFlowDepth, maxFlowDepth)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}
	as, err := parseAsOf(q.Get("as_of"))
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	// Confirma que o root existe — 404 cedo, antes do BFS.
	rootNode, err := c.nodes.GetByURN(r.Context(), start, as)
	if err != nil {
		httpserver.WriteError(w, r, err)
		return
	}

	filter := repository.EdgeFilter{Types: flowEdgeTypes, AsOf: as}

	seenEdges := map[string]edge.Edge{}
	seenNodes := map[node.URN]node.Node{start: rootNode}
	frontier := []node.URN{start}

	for hop := 0; hop < depth; hop++ {
		var next []node.URN
		for _, urn := range frontier {
			edges, err := c.edges.Neighbors(r.Context(), urn, repository.DirAny, filter)
			if err != nil {
				httpserver.WriteError(w, r, err)
				return
			}
			for _, e := range edges {
				if _, ok := seenEdges[e.ID()]; ok {
					continue
				}
				if len(seenEdges) >= maxFlowResults {
					break
				}
				peer := otherEnd(e, urn)
				if peer == "" {
					continue
				}
				if _, ok := seenNodes[peer]; ok {
					// peer já aceito no sub-grafo — registra a edge
					// que conecta dois nós conhecidos.
					seenEdges[e.ID()] = e
					continue
				}
				if len(seenNodes) >= maxFlowResults {
					continue
				}
				n, err := c.nodes.GetByURN(r.Context(), peer, as)
				if err != nil {
					// edge aponta pra nó sem versão visível — omitir.
					continue
				}
				// A view de detalhe é o sub-grafo de UM endpoint
				// específico. Endpoints irmãos (outros handlers do
				// mesmo Module/Service) não pertencem a esse fluxo:
				// sem o filtro abaixo, o BFS sobe via CONTAINS para
				// Module/Service e desce para os irmãos, misturando
				// handlers/calls/types de endpoints distintos.
				// Filtra também a edge para não deixar referência
				// órfã apontando para um peer omitido.
				if n.Kind() == node.KindEndpoint && peer != start {
					continue
				}
				seenEdges[e.ID()] = e
				seenNodes[peer] = n
				next = append(next, peer)
			}
			if len(seenEdges) >= maxFlowResults && len(seenNodes) >= maxFlowResults {
				break
			}
		}
		frontier = next
	}

	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"root":  start,
		"nodes": groupNodesByKind(seenNodes),
		"edges": collectEdgeViews(seenEdges),
	})
}

// groupNodesByKind separa os nós em buckets por `kind`. Buckets
// conhecidos têm chave estável; demais caem em "other".
func groupNodesByKind(seen map[node.URN]node.Node) map[string][]any {
	groups := map[string][]any{
		"service": {}, "module": {}, "endpoint": {}, "function": {},
		"call": {}, "type": {}, "variable": {}, "framework": {}, "other": {},
	}
	urns := make([]node.URN, 0, len(seen))
	for u := range seen {
		urns = append(urns, u)
	}
	sort.Slice(urns, func(i, j int) bool { return urns[i] < urns[j] })
	for _, u := range urns {
		n := seen[u]
		k := string(n.Kind())
		bucket, ok := groups[k]
		if !ok {
			groups["other"] = append(groups["other"], nodeView(n))
			continue
		}
		groups[k] = append(bucket, nodeView(n))
	}
	return groups
}

func collectEdgeViews(seen map[string]edge.Edge) []any {
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, edgeView(seen[id]))
	}
	return out
}

// parseDepthCustom replica a semântica de parseDepth mas com cap
// configurável (parseDepth global usa maxNeighborsDepth = 3).
func parseDepthCustom(s string, def, max int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconvAtoiPositive(s)
	if err != nil {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid depth: must be positive integer")
	}
	if n > max {
		return 0, httpserver.Errorf(http.StatusBadRequest, codeBadRequest, "invalid depth: exceeds cap (max %d)", max)
	}
	return n, nil
}
