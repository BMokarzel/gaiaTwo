package service_compute

import (
	"context"
	"time"

	"costEngine/internal/entity/node"
)

// BridgeRepo é a fronteira contra o grafo. F-009 trabalha sobre uma
// interface, não sobre Neo4j direto: facilita teste (in-memory) e
// segrega responsabilidade.
//
// Métodos lêem **estado corrente** (valid_to IS NULL) — bitemporalidade
// é responsabilidade da engine via Now nos diffs e do repo ao gravar.
type BridgeRepo interface {
	// ListComputeCurrent retorna todos os Computes correntes da conta.
	// Ordem não definida — engine ordena por URN para determinismo.
	ListComputeCurrent(ctx context.Context, account node.URN) ([]node.Compute, error)

	// ListServicesByName retorna mapa de nome → URN para Services
	// current. Nome é o último segmento do ModulePath (`.` → repo).
	// Quando 2+ Services compartilham o mesmo nome, o name NÃO entra
	// em byName e vai para ambiguous (caller decide).
	ListServicesByName(ctx context.Context) (byName map[string]node.URN, ambiguous map[string][]node.URN, err error)

	// ListRunsOnCurrent retorna edges RUNS_ON correntes (valid_to IS
	// NULL) que envolvem Computes da conta. Inclui edges órfãos cujo
	// Service tenha sido deletado — engine usa para fechar.
	ListRunsOnCurrent(ctx context.Context, account node.URN) ([]ExistingEdge, error)

	// ApplyEdgeChanges grava o diff em uma transação (close primeiro,
	// open depois — fecha antes de reabrir para não duplicar). dryRun
	// true ⇒ não escreve, só valida adjacency.
	ApplyEdgeChanges(ctx context.Context, opens []EdgeOpen, closes []EdgeClose, dryRun bool) error
}

// nowFunc é injetável para testes determinísticos.
type nowFunc func() time.Time
