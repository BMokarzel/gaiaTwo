package codeowners

import (
	"context"
	"fmt"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Stats reporta o efeito de uma aplicação no grafo.
type Stats struct {
	Opened     int // edges novos abertos
	Closed     int // edges antigos fechados
	Unchanged  int // edges já-correntes mantidos
	Unresolved int // handles do CODEOWNERS não-resolvidos
}

// Writer aplica um `Result` ao grafo com semântica close-and-reopen
// (F-011 D3): edges não-presentes na nova extração têm `ValidTo`
// fechado; edges já-correntes ficam inalterados; novos são abertos.
type Writer struct {
	Nodes repository.NodeRepository
	Edges repository.EdgeRepository
}

// Apply aplica `res` ao grafo. Determinismo:
//   - Identidade lógica de um Owns é o par (From, Target).
//   - Se a edge atual em `(from, target)` está corrente, é "unchanged".
//   - Se há current edge cujo `from` não aparece em `res`, é fechada.
//   - Se há owner em `res` sem edge current, é aberta.
//
// O kind do `from` (Person ou Team) é necessário para Validate; é
// resolvido pelo `Nodes.GetByURN` antes do upsert. Mantém a invariante
// de adjacência (F-011 S-001).
func (w *Writer) Apply(ctx context.Context, res Result) (Stats, error) {
	st := Stats{Unresolved: len(res.Unresolved)}

	current, err := w.currentOwnsByFrom(ctx, res.Target)
	if err != nil {
		return st, err
	}
	wanted := make(map[node.URN]edge.Owns, len(res.Owns))
	for _, e := range res.Owns {
		wanted[e.From()] = e
	}

	// Abrir ou manter.
	for from, owns := range wanted {
		if _, exists := current[from]; exists {
			st.Unchanged++
			continue
		}
		kind, kerr := w.fromKind(ctx, from)
		if kerr != nil {
			return st, kerr
		}
		if err := w.Edges.Upsert(ctx, owns, kind, node.KindService); err != nil {
			return st, fmt.Errorf("open Owns %s→%s: %w", from, res.Target, err)
		}
		st.Opened++
	}

	// Fechar os que sumiram.
	for from, e := range current {
		if _, keep := wanted[from]; keep {
			continue
		}
		if err := w.Edges.Delete(ctx, e.ID()); err != nil {
			return st, fmt.Errorf("close Owns %s→%s: %w", from, res.Target, err)
		}
		st.Closed++
	}
	return st, nil
}

// currentOwnsByFrom devolve um mapa {from-URN → edge} dos `Owns`
// correntes que entram em `target`. Usa Neighbors em DirIn filtrando
// por TypeOwns.
func (w *Writer) currentOwnsByFrom(ctx context.Context, target node.URN) (map[node.URN]edge.Edge, error) {
	neigh, err := w.Edges.Neighbors(ctx, target, repository.DirIn, repository.EdgeFilter{
		Types: []edge.Type{edge.TypeOwns},
	})
	if err != nil {
		return nil, fmt.Errorf("current Owns neighbors: %w", err)
	}
	out := make(map[node.URN]edge.Edge, len(neigh))
	for _, e := range neigh {
		out[e.From()] = e
	}
	return out, nil
}

// fromKind lê o Node atual para descobrir o kind exato (Person vs Team).
// Encurta: a URN também codifica o kind, mas usamos GetByURN para
// confirmar existência (recusar abrir Owns apontando para Person
// fechada/inexistente).
func (w *Writer) fromKind(ctx context.Context, urn node.URN) (node.Kind, error) {
	n, err := w.Nodes.GetByURN(ctx, urn, repository.AsOf{})
	if err != nil {
		return "", fmt.Errorf("lookup owner %s: %w", urn, err)
	}
	return n.Kind(), nil
}
