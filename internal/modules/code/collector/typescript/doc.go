// Package typescript coleta entidades de código TypeScript via sidecar
// Node.js + ts-morph (F-030).
//
// Por que sidecar Node:
//   - Resolução de tipos do TypeScript exige o compilador real
//     (`typescript` package). ts-morph é o wrapper de-facto.
//   - Type-info correta é o que diferencia este coletor das
//     heurísticas regex de `npm/` e `python/` (que só emitem
//     Framework). Sem ela, Calls/Targets ficam frágeis.
//   - Detalhes da escolha: ADR-012.
//
// Pipeline:
//
//	Collect(root, cfg)
//	  ├── extrai bundle embedded para tmpdir (cache por hash)
//	  ├── exec.CommandContext(ctx, node, sidecar.js)
//	  │     stdin: ConfigEnvelope
//	  │     stdout: NDJSON eventos (1 linha = 1 entidade)
//	  │     stderr: logs textuais (debug, --verbose)
//	  └── decodifica eventos → node.Service/Endpoint/Function/...
//
// Pré-requisito: Node ≥18 no PATH. Sem ele, Collect retorna
// ErrNodeMissing com mensagem instrutiva.
//
// Entidades emitidas (paridade com `golang.Collect`):
//   - node.Service (1 por package.json)
//   - node.Module  (1 por diretório com .ts files)
//   - node.Endpoint, node.Function, node.Call, node.Type, node.Variable
//   - node.Framework (delega a `npm.Collect`)
//   - Edges: Contains, Invokes, Targets, Uses, Extends, Aliases,
//     DependsOn, DefinedIn (legado).
//
// Limitações conhecidas (MVP):
//   - Sem `.tsx` (JSX) — slice futura.
//   - Sem resolução cross-workspace (`@workspace/foo`).
//   - Frameworks reconhecidos: Express, NestJS apenas.
//   - Sem manifesto `costengine.yaml` (deferido para ADR-011 slice).
package typescript
