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
	Services  int
	Endpoints int
	Functions int
	Edges     int
}

// Writer escreve um `golang.Result` em (Node|Edge)Repository.
//
// Pipeline:
//   - Upsert todos os Services (precedem dependências DefinedIn).
//   - Upsert Endpoints e Functions.
//   - Upsert edges DefinedIn (com fromKind/toKind explícitos para o
//     registry validar topologia).
type Writer struct {
	Nodes repository.NodeRepository
	Edges repository.EdgeRepository
}

// Apply persiste o Result. Retorna na primeira falha — coletores devem
// ser idempotentes, então repetir após corrigir o erro é seguro.
func (w *Writer) Apply(ctx context.Context, res golang.Result) (Stats, error) {
	var st Stats
	for i := range res.Services {
		s := res.Services[i]
		if err := w.Nodes.Upsert(ctx, s); err != nil {
			return st, fmt.Errorf("upsert service %s: %w", s.URN(), err)
		}
		st.Services++
	}
	for i := range res.Endpoints {
		e := res.Endpoints[i]
		if err := w.Nodes.Upsert(ctx, e); err != nil {
			return st, fmt.Errorf("upsert endpoint %s: %w", e.URN(), err)
		}
		st.Endpoints++
	}
	for i := range res.Functions {
		f := res.Functions[i]
		if err := w.Nodes.Upsert(ctx, f); err != nil {
			return st, fmt.Errorf("upsert function %s: %w", f.URN(), err)
		}
		st.Functions++
	}
	for i := range res.Edges {
		e := res.Edges[i]
		fromKind := kindFromURN(e.From(), res)
		if err := w.Edges.Upsert(ctx, e, fromKind, node.KindService); err != nil {
			return st, fmt.Errorf("upsert edge %s→%s: %w", e.From(), e.To(), err)
		}
		st.Edges++
	}
	return st, nil
}

// kindFromURN derive o Kind da fonte para validação de aresta. Como
// DefinedIn só sai de Endpoint/Function, basta verificar em qual slice
// está. Manter local evita acoplar `node` a um parser de URN aqui.
func kindFromURN(u node.URN, res golang.Result) node.Kind {
	for _, ep := range res.Endpoints {
		if ep.URN() == u {
			return node.KindEndpoint
		}
	}
	for _, fn := range res.Functions {
		if fn.URN() == u {
			return node.KindFunction
		}
	}
	return ""
}

// Compile-time check: garante que `edge.DefinedIn` satisfaz `edge.Edge`.
var _ edge.Edge = edge.DefinedIn{}
