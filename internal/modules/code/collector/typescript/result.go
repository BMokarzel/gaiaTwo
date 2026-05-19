package typescript

import (
	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Result agrupa o que `Collect()` produz para um repositório TS.
//
// Shape espelha `golang.Result` para permitir merge no dispatcher do
// CLI (`cmd/cli/extract_code.go`) e reaproveitar o Writer existente
// quando estendido (F-030 S-030.8).
//
// Quando dois collectors rodarem no mesmo path (raro em prática, mas
// possível em mono-repo poliglota), o merge é simples: append por
// slice — URNs e edge IDs são determinísticos e dedupam naturalmente
// no Upsert.
type Result struct {
	Services   []node.Service
	Modules    []node.Module
	Endpoints  []node.Endpoint
	Functions  []node.Function
	Frameworks []node.Framework
	Calls      []node.Call
	Types      []node.Type
	Variables  []node.Variable

	Contains  []edge.Contains
	DependsOn []edge.DependsOn
	Invokes   []edge.Invokes
	Targets   []edge.Targets
	Uses      []edge.Uses
	Extends   []edge.Extends
	Aliases   []edge.Aliases
	DefinedIn []edge.DefinedIn // legado F-007, deprecated
}
