// Package service é o orquestrador do org plane (F-010).
//
// Recebe um `hris.Result` (do parser/emitter) e o escreve no grafo via
// `NodeRepository` + `EdgeRepository`. Mantém-se ignorante do backend
// (memory, n4j) — opera só sobre as interfaces de repository.
package service

import (
	"context"
	"fmt"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org/ingest/hris"
	"costEngine/internal/repository"
)

// Stats reporta quantos itens foram escritos.
type Stats struct {
	Teams      int
	Squads     int
	Persons    int
	Edges      int
	Terminated int // pessoas com end_date preenchido — fechadas via Delete
}

// Writer escreve um `hris.Result` em (Node|Edge)Repository.
//
// Pipeline:
//  1. Upsert Teams (precedem Squads via PART_OF).
//  2. Upsert Squads (precedem Persons via MEMBER_OF).
//  3. Upsert Persons.
//  4. Upsert edges (PART_OF, MEMBER_OF, REPORTS_TO).
//  5. Delete Persons terminadas (fecha valid_to).
type Writer struct {
	Nodes repository.NodeRepository
	Edges repository.EdgeRepository
}

// Apply persiste o Result. Retorna na primeira falha — coletor é
// idempotente, então repetir após corrigir é seguro.
func (w *Writer) Apply(ctx context.Context, res hris.Result) (Stats, error) {
	var st Stats
	for i := range res.Teams {
		if err := w.Nodes.Upsert(ctx, res.Teams[i]); err != nil {
			return st, fmt.Errorf("upsert team %s: %w", res.Teams[i].URN(), err)
		}
		st.Teams++
	}
	for i := range res.Squads {
		if err := w.Nodes.Upsert(ctx, res.Squads[i]); err != nil {
			return st, fmt.Errorf("upsert squad %s: %w", res.Squads[i].URN(), err)
		}
		st.Squads++
	}
	for i := range res.Persons {
		if err := w.Nodes.Upsert(ctx, res.Persons[i]); err != nil {
			return st, fmt.Errorf("upsert person %s: %w", res.Persons[i].URN(), err)
		}
		st.Persons++
	}
	for i := range res.PartOf {
		if err := w.Edges.Upsert(ctx, res.PartOf[i], node.KindSquad, node.KindTeam); err != nil {
			return st, fmt.Errorf("upsert PART_OF %s→%s: %w", res.PartOf[i].From(), res.PartOf[i].To(), err)
		}
		st.Edges++
	}
	for i := range res.MemberOf {
		if err := w.Edges.Upsert(ctx, res.MemberOf[i], node.KindPerson, node.KindSquad); err != nil {
			return st, fmt.Errorf("upsert MEMBER_OF %s→%s: %w", res.MemberOf[i].From(), res.MemberOf[i].To(), err)
		}
		st.Edges++
	}
	for i := range res.ReportsTo {
		if err := w.Edges.Upsert(ctx, res.ReportsTo[i], node.KindPerson, node.KindPerson); err != nil {
			return st, fmt.Errorf("upsert REPORTS_TO %s→%s: %w", res.ReportsTo[i].From(), res.ReportsTo[i].To(), err)
		}
		st.Edges++
	}

	// Soft-delete pessoas terminadas. Fecha a Person; edges MEMBER_OF
	// saindo dela são fechadas em sequência via Neighbors→Delete.
	for _, urn := range res.Terminated {
		if err := w.Nodes.Delete(ctx, urn); err != nil {
			return st, fmt.Errorf("close terminated %s: %w", urn, err)
		}
		neigh, err := w.Edges.Neighbors(ctx, urn, repository.DirOut, repository.EdgeFilter{
			Types: []edge.Type{edge.TypeMemberOf},
		})
		if err != nil {
			return st, fmt.Errorf("neighbors for terminated %s: %w", urn, err)
		}
		for _, e := range neigh {
			if err := w.Edges.Delete(ctx, e.ID()); err != nil {
				return st, fmt.Errorf("close edge %s: %w", e.ID(), err)
			}
		}
		st.Terminated++
	}
	return st, nil
}
