package codeowners

import (
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// EmitOptions agrupa metadados aplicados às arestas emitidas.
type EmitOptions struct {
	Tenant     string
	RunID      string
	ObservedAt time.Time
}

// Result agrega o que `Build` produz: arestas `Owns` candidatas + lista
// de handles não-resolvíveis (já filtrada pelo resolver, repassada para
// auditoria) + alvo Service.
type Result struct {
	Target     node.URN     // Service que recebe os edges
	Owns       []edge.Owns  // edges candidatos (writer pode pular se já existe)
	Unresolved []string     // handles do CODEOWNERS sem URN no grafo
}

// Build converte uma lista de owners resolvidos no resultado bruto. O
// `Writer` decide depois quais edges abrir / fechar / manter.
//
// Determinismo: ID = DeterministicID(from, OWNS, target, observedAt).
// Como `observedAt` muda entre extrações, o ID isolado não permite
// dedup — a igualdade lógica é por (from, target). O writer compara
// pelo `from` do edge corrente no grafo.
func Build(target node.URN, resolved []ResolvedOwner, unresolved []string, opts EmitOptions) Result {
	if opts.ObservedAt.IsZero() {
		opts.ObservedAt = time.Now().UTC()
	}
	res := Result{Target: target, Unresolved: unresolved}
	for _, o := range resolved {
		res.Owns = append(res.Owns, edge.Owns{Base: edge.Base{
			EdgeID:   edge.DeterministicID(o.URN, edge.TypeOwns, target, opts.ObservedAt),
			EdgeType: edge.TypeOwns,
			FromURN:  o.URN,
			ToURN:    target,
			EdgeMeta: edge.Meta{
				ValidFrom:   opts.ObservedAt,
				ObservedAt:  opts.ObservedAt,
				Source:      node.Source{Collector: "org/codeowners", RunID: opts.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
			},
		}})
	}
	return res
}
