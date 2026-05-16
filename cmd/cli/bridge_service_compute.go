package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge/service_compute"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// runBridge é o roteador de `ce bridge <kind>`.
func runBridge(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce bridge <kind> [flags]", errBadUsage)
	}
	switch args[0] {
	case "service-compute":
		return runBridgeServiceCompute(ctx, args[1:])
	default:
		return fmt.Errorf("bridge não suportado: %q (suportados: service-compute)", args[0])
	}
}

// runBridgeServiceCompute é o handler de F-009.
//
// Backend:
//   - Sem `--neo4j`: backend memory (apenas valida flags + dry-run vazio).
//   - Com `--neo4j`: lê Compute/Service correntes, calcula diff e aplica.
//
// `--dry-run` lista o plano sem gravar edges.
func runBridgeServiceCompute(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bridge service-compute", flag.ContinueOnError)
	account := fs.String("account", "", "URN da conta (urn:ce:aws:<id>:account/<id>) — obrigatório")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (opcional; vazio → backend memory in-process)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	dryRun := fs.Bool("dry-run", false, "calcula o plano mas não grava edges")
	runID := fs.String("run-id", "", "identificador da execução (default: timestamp)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *account == "" {
		return fmt.Errorf("%w: --account obrigatório", errBadUsage)
	}
	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("bridge-%d", time.Now().Unix())
	}

	var (
		nodeRepo repository.NodeRepository
		edgeRepo repository.EdgeRepository
	)
	if *neoURI == "" {
		m := memory.New()
		nodeRepo, edgeRepo = m, m.AsEdgeRepo()
		fmt.Println("backend: memory (in-process — nenhum dado pré-existente)")
	} else {
		nc, err := n4j.Connect(ctx, n4j.Config{
			URI: *neoURI, Username: *neoUser, Password: *neoPass, Database: *neoDB,
		})
		if err != nil {
			return fmt.Errorf("neo4j connect: %w", err)
		}
		defer nc.Close(ctx)
		if err := nc.Migrate(ctx); err != nil {
			return fmt.Errorf("neo4j migrate: %w", err)
		}
		nodeRepo = n4j.NewNodeRepo(nc)
		edgeRepo = n4j.NewEdgeRepo(nc)
	}

	src := node.Source{Collector: "bridge.service_compute", RunID: rid, Method: node.MethodInferred}
	adapter := service_compute.NewRepoAdapter(nodeRepo, edgeRepo, src)

	byName, ambig, err := adapter.ListServicesByName(ctx)
	if err != nil {
		return fmt.Errorf("services: %w", err)
	}
	comps, err := adapter.ListComputeCurrent(ctx, node.URN(*account))
	if err != nil {
		return fmt.Errorf("computes: %w", err)
	}
	existing, err := adapter.ListRunsOnCurrent(ctx, node.URN(*account))
	if err != nil {
		return fmt.Errorf("existing edges: %w", err)
	}

	out := service_compute.Apply(service_compute.Input{
		Account:        node.URN(*account),
		Computes:       comps,
		ServicesByName: byName,
		AmbiguousNames: ambig,
		ExistingEdges:  existing,
		Now:            time.Now().UTC(),
	})

	if err := adapter.ApplyEdgeChanges(ctx, out.Opens, out.Closes, *dryRun); err != nil {
		return fmt.Errorf("apply: %w", err)
	}

	mode := "applied"
	if *dryRun {
		mode = "dry-run"
	}
	fmt.Printf("bridge service-compute (%s): account=%s run=%s\n", mode, *account, rid)
	fmt.Printf("  Computes scanned:    %d\n", out.Stats.ComputesScanned)
	fmt.Printf("  Resolved by tag:     %d\n", out.Stats.ResolvedTag)
	fmt.Printf("  Resolved by name:    %d\n", out.Stats.ResolvedName)
	fmt.Printf("  No signal (orphan):  %d\n", out.Stats.NoSignal)
	fmt.Printf("  Ambiguous services:  %d\n", out.Stats.AmbiguousCount)
	fmt.Printf("  Edges opened:        %d\n", out.Stats.Opens)
	fmt.Printf("  Edges closed:        %d\n", out.Stats.Closes)

	if n := len(out.Ambiguous); n > 0 {
		fmt.Printf("\nambíguos (%d) — use tag service.urn=<urn> para desambiguar:\n", n)
		for _, a := range out.Ambiguous {
			fmt.Printf("  %s → %q (candidatos: %v)\n", a.ComputeURN, a.Name, a.Candidates)
		}
	}
	return nil
}
