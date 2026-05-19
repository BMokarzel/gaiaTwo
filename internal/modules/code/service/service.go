// Package service é o orquestrador do code plane (F-007).
//
// Recebe um `golang.Result` (do coletor) e o escreve no grafo via
// `NodeRepository` + `EdgeRepository`. Mantém-se ignorante de quem é o
// backend (memory, n4j) — opera só sobre as interfaces de repository.
package service

import (
	"context"
	"fmt"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/code/collector/golang"
	"costEngine/internal/repository"
)

// Stats reporta quantos itens foram escritos pelo Writer.
type Stats struct {
	Services   int
	Modules    int
	Endpoints  int
	Functions  int
	Types      int
	Variables  int
	Frameworks int
	Calls      int
	Edges      int
}

// Writer escreve um `golang.Result` em (Node|Edge)Repository.
//
// Pipeline:
//   - Upsert nós em ordem de dependência (Services → Modules →
//     Endpoints/Functions → Types/Variables → Calls → Frameworks).
//   - Upsert edges estruturais com fromKind/toKind explícitos para o
//     registry validar topologia (Contains, DependsOn, Invokes, Targets,
//     Uses, Extends, Aliases, DefinedIn).
type Writer struct {
	Nodes repository.NodeRepository
	Edges repository.EdgeRepository
}

// Apply persiste o Result. Retorna na primeira falha — coletores devem
// ser idempotentes, então repetir após corrigir o erro é seguro.
func (w *Writer) Apply(ctx context.Context, res golang.Result) (Stats, error) {
	var st Stats

	// Index URN → Kind a partir de todos os nós do Result.
	// Usado pelas edges para validação no registry sem precisar
	// fazer round-trip no repository.
	kinds := buildKindIndex(res)

	if err := w.upsertNodes(ctx, res, &st); err != nil {
		return st, err
	}

	type edgeBatch struct {
		name  string
		edges []edge.Edge
	}
	batches := []edgeBatch{
		{"contains", asEdges(res.Contains)},
		{"depends_on", asEdges(res.DependsOn)},
		{"invokes", asEdges(res.Invokes)},
		{"targets", asEdges(res.Targets)},
		{"uses", asEdges(res.Uses)},
		{"extends", asEdges(res.Extends)},
		{"aliases", asEdges(res.Aliases)},
		{"defined_in", asEdges(res.DefinedIn)},
	}
	for _, b := range batches {
		for _, e := range b.edges {
			fromKind, ok := kinds[e.From()]
			if !ok {
				return st, fmt.Errorf(
					"upsert edge %s (%s→%s): from kind unknown — node not in result",
					b.name, e.From(), e.To(),
				)
			}
			toKind, ok := kinds[e.To()]
			if !ok {
				return st, fmt.Errorf(
					"upsert edge %s (%s→%s): to kind unknown — node not in result",
					b.name, e.From(), e.To(),
				)
			}
			if err := w.Edges.Upsert(ctx, e, fromKind, toKind); err != nil {
				return st, fmt.Errorf("upsert %s %s→%s: %w", b.name, e.From(), e.To(), err)
			}
			st.Edges++
		}
	}

	return st, nil
}

// upsertNodes grava todos os nós do Result. Ordem: containers antes
// de contidos (Service → Module → Endpoint/Function → Type/Variable →
// Call → Framework). A ordem reduz a chance de o registry rejeitar uma
// aresta apontando para um nó ainda inexistente, embora o memory
// backend não exija — útil pra backends futuros mais estritos.
func (w *Writer) upsertNodes(ctx context.Context, res golang.Result, st *Stats) error {
	for i := range res.Services {
		s := res.Services[i]
		if err := w.Nodes.Upsert(ctx, s); err != nil {
			return fmt.Errorf("upsert service %s: %w", s.URN(), err)
		}
		st.Services++
	}
	for i := range res.Modules {
		m := res.Modules[i]
		if err := w.Nodes.Upsert(ctx, m); err != nil {
			return fmt.Errorf("upsert module %s: %w", m.URN(), err)
		}
		st.Modules++
	}
	for i := range res.Endpoints {
		e := res.Endpoints[i]
		if err := w.Nodes.Upsert(ctx, e); err != nil {
			return fmt.Errorf("upsert endpoint %s: %w", e.URN(), err)
		}
		st.Endpoints++
	}
	for i := range res.Functions {
		f := res.Functions[i]
		if err := w.Nodes.Upsert(ctx, f); err != nil {
			return fmt.Errorf("upsert function %s: %w", f.URN(), err)
		}
		st.Functions++
	}
	for i := range res.Types {
		t := res.Types[i]
		if err := w.Nodes.Upsert(ctx, t); err != nil {
			return fmt.Errorf("upsert type %s: %w", t.URN(), err)
		}
		st.Types++
	}
	for i := range res.Variables {
		v := res.Variables[i]
		if err := w.Nodes.Upsert(ctx, v); err != nil {
			return fmt.Errorf("upsert variable %s: %w", v.URN(), err)
		}
		st.Variables++
	}
	for i := range res.Calls {
		c := res.Calls[i]
		if err := w.Nodes.Upsert(ctx, c); err != nil {
			return fmt.Errorf("upsert call %s: %w", c.URN(), err)
		}
		st.Calls++
	}
	for i := range res.Frameworks {
		f := res.Frameworks[i]
		if err := w.Nodes.Upsert(ctx, f); err != nil {
			return fmt.Errorf("upsert framework %s: %w", f.URN(), err)
		}
		st.Frameworks++
	}
	return nil
}

// buildKindIndex mapeia URN → Kind para cada nó do Result. Mais barato
// que parsing de URN e mais correto (Kind é canonical no nó).
func buildKindIndex(res golang.Result) map[node.URN]node.Kind {
	m := make(map[node.URN]node.Kind,
		len(res.Services)+len(res.Modules)+len(res.Endpoints)+
			len(res.Functions)+len(res.Types)+len(res.Variables)+
			len(res.Calls)+len(res.Frameworks))
	for _, n := range res.Services {
		m[n.URN()] = node.KindService
	}
	for _, n := range res.Modules {
		m[n.URN()] = node.KindModule
	}
	for _, n := range res.Endpoints {
		m[n.URN()] = node.KindEndpoint
	}
	for _, n := range res.Functions {
		m[n.URN()] = node.KindFunction
	}
	for _, n := range res.Types {
		m[n.URN()] = node.KindType
	}
	for _, n := range res.Variables {
		m[n.URN()] = node.KindVariable
	}
	for _, n := range res.Calls {
		m[n.URN()] = node.KindCall
	}
	for _, n := range res.Frameworks {
		m[n.URN()] = node.KindFramework
	}
	return m
}

// asEdges converte um slice de tipo concreto para []edge.Edge sem alocar
// para cada elemento individualmente. Generic helper local porque Go
// não tem variância em slices.
func asEdges[T edge.Edge](xs []T) []edge.Edge {
	out := make([]edge.Edge, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// Compile-time check: garante que `edge.DefinedIn` satisfaz `edge.Edge`.
var _ edge.Edge = edge.DefinedIn{}
