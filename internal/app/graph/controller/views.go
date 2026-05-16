package controller

import (
	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// nodeView é o shape JSON canônico de um node nos endpoints
// `/v1/architecture/*`. Mantido propositalmente fino: campos topo
// (urn/kind/version/...) + `data` com o struct concreto que serializa
// via tags próprias. Nunca expõe internals (cypher, repo paths).
func nodeView(n node.Node) map[string]any {
	m := n.Meta()
	v := map[string]any{
		"urn":         n.URN(),
		"kind":        n.Kind(),
		"version":     m.Version,
		"valid_from":  m.ValidFrom,
		"observed_at": m.ObservedAt,
		"confidence":  m.Confidence,
		"data":        n, // o concreto serializa via tags próprias.
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
	}
	if len(m.Labels) > 0 {
		v["labels"] = m.Labels
	}
	return v
}

func edgeView(e edge.Edge) map[string]any {
	m := e.Meta()
	v := map[string]any{
		"id":          e.ID(),
		"type":        e.Type(),
		"from":        e.From(),
		"to":          e.To(),
		"valid_from":  m.ValidFrom,
		"observed_at": m.ObservedAt,
		"confidence":  m.Confidence,
		"data":        e,
	}
	if m.ValidTo != nil {
		v["valid_to"] = *m.ValidTo
	}
	return v
}

// otherEnd devolve o nó "do outro lado" da aresta em relação a `cur`.
// Vazio se cur não é endpoint da edge (entrada inválida — caller ignora).
func otherEnd(e edge.Edge, cur node.URN) node.URN {
	if e.From() == cur {
		return e.To()
	}
	if e.To() == cur {
		return e.From()
	}
	return ""
}
