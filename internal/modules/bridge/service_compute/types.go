package service_compute

import (
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// ExistingEdge é a projeção mínima de um edge RUNS_ON corrente
// (valid_to IS NULL) que a engine precisa para decidir
// noop/close/reopen sem carregar o objeto completo. EdgeID é o
// identificador determinístico no repo (necessário para Delete em
// fechamentos).
type ExistingEdge struct {
	EdgeID     string
	ComputeURN node.URN
	ServiceURN node.URN
	ValidFrom  time.Time
	Confidence float32
	Source     Source
}

// EdgeOpen descreve a abertura de um novo edge RUNS_ON.
type EdgeOpen struct {
	ServiceURN node.URN
	ComputeURN node.URN
	ValidFrom  time.Time
	Confidence float32
	Source     Source
}

// EdgeClose descreve o fechamento de um edge RUNS_ON existente
// (set valid_to = now). EdgeID identifica o edge a fechar no repo.
type EdgeClose struct {
	EdgeID     string
	ServiceURN node.URN
	ComputeURN node.URN
	ValidTo    time.Time
	// Reason é informativo: "owner_changed" (Service mudou) ou
	// "orphaned" (Compute perdeu sinal). Persistido em
	// Meta.Properties["close_reason"] quando o repo escrever.
	Reason string
}

// Ambiguity registra um Compute que não pode ser resolvido por causa
// de Service ambíguo (mesmo nome em 2+ URNs). Não emite edge — caller
// decide (relatório, fila unresolved).
type Ambiguity struct {
	ComputeURN node.URN
	Name       string     // valor da tag/nome que ficou ambíguo
	Candidates []node.URN // URNs concorrentes
}

// Input agrega o que a engine precisa para uma run determinística.
//
// ServicesByName: mapa nome→URN derivado dos Services current pelo
// caller (repo). Caller também decide ambiguidades antes de chamar:
// nomes ambíguos não entram no mapa; vêm em AmbiguousNames.
type Input struct {
	Account         node.URN
	Computes        []node.Compute
	ServicesByName  map[string]node.URN
	AmbiguousNames  map[string][]node.URN // nome → URNs concorrentes
	ExistingEdges   []ExistingEdge        // current edges RUNS_ON da conta
	Now             time.Time             // valid_from/valid_to dos diffs
}

// Output descreve o plano que o caller aplica no repo (idempotente).
type Output struct {
	Opens     []EdgeOpen
	Closes    []EdgeClose
	Orphans   []node.URN  // Computes sem sinal nem edge corrente
	Ambiguous []Ambiguity // Computes com Service ambíguo
	// Stats compactos para relatório CLI.
	Stats Stats
}

// Stats consolida totais para o relatório final de uma run.
type Stats struct {
	ComputesScanned int
	ResolvedTag     int
	ResolvedName    int
	NoSignal        int
	AmbiguousCount  int
	Opens           int
	Closes          int
}

// EdgeRunsOn é o struct concreto de RUNS_ON que o repo materializa
// a partir de um EdgeOpen. Não persistido aqui — engine emite apenas
// o diff; repo é quem instancia.
type EdgeRunsOn struct {
	edge.Base
}
