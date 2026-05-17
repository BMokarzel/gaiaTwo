package openapi

import (
	"context"
	"errors"
	"fmt"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Writer grava o resultado do ingest no repositório. A política é:
//   - Service alvo precisa pré-existir (lookup por URN) — ausência é erro.
//   - Endpoints e edges são Upsert-ados (bitemporal cuida da idempotência).
type Writer struct {
	Nodes repository.NodeRepository
	Edges repository.EdgeRepository
}

// ApplyStats descreve o output do Apply para logs/relatórios CLI.
type ApplyStats struct {
	Endpoints int
	Edges     int
	Schemas   int
	Imports   int
}

// Apply persiste os nós e arestas do Result.
//
// Pré-condição: o Service referenciado por `Result.Endpoints[0].ServiceURN`
// existe no grafo. Se não, retorna erro envolvendo ErrNotFound — caller
// pode reportar como uso inválido (CLI exit code 2).
func (w Writer) Apply(ctx context.Context, r Result, serviceURN node.URN) (ApplyStats, error) {
	if w.Nodes == nil || w.Edges == nil {
		return ApplyStats{}, errors.New("openapi/writer: Nodes and Edges are required")
	}
	if _, err := w.Nodes.GetByURN(ctx, serviceURN, repository.AsOf{}); err != nil {
		return ApplyStats{}, fmt.Errorf("openapi/writer: service %s: %w", serviceURN, err)
	}

	st := ApplyStats{}
	for i := range r.Endpoints {
		if err := w.Nodes.Upsert(ctx, r.Endpoints[i]); err != nil {
			return st, fmt.Errorf("upsert endpoint %s: %w", r.Endpoints[i].URN(), err)
		}
		st.Endpoints++
	}
	for i := range r.Schemas {
		if err := w.Nodes.Upsert(ctx, r.Schemas[i]); err != nil {
			return st, fmt.Errorf("upsert schema %s: %w", r.Schemas[i].URN(), err)
		}
		st.Schemas++
	}
	for i := range r.Edges {
		if err := w.Edges.Upsert(ctx, r.Edges[i], node.KindEndpoint, node.KindService); err != nil {
			return st, fmt.Errorf("upsert edge %s: %w", r.Edges[i].ID(), err)
		}
		st.Edges++
	}
	for i := range r.Imports {
		if err := w.Edges.Upsert(ctx, r.Imports[i], node.KindSchema, node.KindSchema); err != nil {
			return st, fmt.Errorf("upsert imports %s: %w", r.Imports[i].ID(), err)
		}
		st.Imports++
	}
	return st, nil
}
