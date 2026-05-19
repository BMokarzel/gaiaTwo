package typescript

import (
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

type symKey struct {
	svc   node.URN
	short string
}

// resolveTargets executa um pós-pass best-effort que conecta:
//
//   - Call -> Function: via "last-segment match" — pega o último identifier de
//     TargetSymbol (ex.: `this.repo.findById` -> `findById`) e procura uma
//     Function cujo Symbol seja `findById` ou `(Receiver).findById` no mesmo
//     Service. Empate ambíguo (>1 match) é ignorado.
//
// Endpoint -> Function não é uma edge direta no ontológico atual; o BFS
// bidirecional de /flow conecta via `Module CONTAINS Endpoint` +
// `Module CONTAINS Function (handler)`.
//
// É heurística — o sidecar não faz resolução de símbolos. Slice futura
// (F-026): symbol resolver com ts-morph type checker.
func (d *decoder) resolveTargets() {
	now := d.cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	idx := map[symKey][]node.URN{}
	for _, fn := range d.res.Functions {
		short := fn.Symbol
		if i := strings.LastIndex(fn.Symbol, ")."); i >= 0 {
			short = fn.Symbol[i+2:]
		}
		idx[symKey{svc: fn.ServiceURN, short: short}] = append(
			idx[symKey{svc: fn.ServiceURN, short: short}], fn.NodeURN)
		if short != fn.Symbol {
			idx[symKey{svc: fn.ServiceURN, short: fn.Symbol}] = append(
				idx[symKey{svc: fn.ServiceURN, short: fn.Symbol}], fn.NodeURN)
		}
	}

	lookup := func(svc node.URN, name string) node.URN {
		hits := idx[symKey{svc: svc, short: name}]
		if len(hits) == 1 {
			return hits[0]
		}
		return ""
	}

	// Call -> Function (Targets) — via Caller's Service.
	callerToSvc := map[node.URN]node.URN{}
	for _, fn := range d.res.Functions {
		callerToSvc[fn.NodeURN] = fn.ServiceURN
	}
	for _, c := range d.res.Calls {
		if c.TargetSymbol == "" {
			continue
		}
		svc, ok := callerToSvc[c.CallerURN]
		if !ok {
			continue
		}
		match := lookup(svc, lastSegment(c.TargetSymbol))
		if match == "" {
			continue
		}
		id := edge.DeterministicID(c.NodeURN, edge.TypeTargets, match, now)
		d.res.Targets = append(d.res.Targets, edge.Targets{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeTargets, FromURN: c.NodeURN, ToURN: match, EdgeMeta: d.edgeMeta(),
		}})
	}
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}
