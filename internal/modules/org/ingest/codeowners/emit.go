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

// RejectedOwner é um owner resolvido (existe no grafo) mas que o lint
// ADR-010 (F-029) descarta — porque a bifurcação Team/Person não
// admite a aresta. Ex.: Person→Service. Mantém Handle/URN/Kind para
// auditoria.
type RejectedOwner struct {
	Handle string
	URN    node.URN
	Kind   node.Kind
	Reason string
}

// Result agrega o que `Build` produz: arestas `Owns` candidatas + lista
// de handles não-resolvíveis (já filtrada pelo resolver, repassada para
// auditoria) + owners rejeitados pelo lint de bifurcação + alvo Service.
type Result struct {
	Target     node.URN        // Service que recebe os edges
	Owns       []edge.Owns     // edges candidatos (writer pode pular se já existe)
	Unresolved []string        // handles do CODEOWNERS sem URN no grafo
	Rejected   []RejectedOwner // owners resolvidos mas reprovados pelo lint ADR-010
}

// Build converte uma lista de owners resolvidos no resultado bruto. O
// `Writer` decide depois quais edges abrir / fechar / manter.
//
// F-029 lint (ADR-010): aplica `edge.ValidateOwnership(owner.Kind,
// target.Kind)` antes de emitir cada edge. Person→Service e similares
// são descartados para `Rejected` — não viram OWNS no grafo.
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

	// Deriva o Kind do target a partir da URN — o coletor codeowners
	// sempre aponta para Service no MVP (F-011), mas mantemos genérico.
	var targetKind node.Kind
	if parts, err := node.ParseURN(target); err == nil {
		targetKind = parts.Kind
	}

	for _, o := range resolved {
		if err := edge.ValidateOwnership(o.Kind, targetKind); err != nil {
			res.Rejected = append(res.Rejected, RejectedOwner{
				Handle: o.Handle, URN: o.URN, Kind: o.Kind, Reason: err.Error(),
			})
			continue
		}
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
