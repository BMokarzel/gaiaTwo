package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"costEngine/internal/modules/org/ingest/hris"
	orgsvc "costEngine/internal/modules/org/service"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// runIngestHRIS é o handler de `ce ingest hris` (F-010 S-005).
//
// Pipeline:
//  1. Parse CSV → linhas válidas + RowErrors (não-fatal).
//  2. Build → Result (Persons/Teams/Squads + edges); ciclo → fatal.
//  3. Writer.Apply → grava no repository (memory ou Neo4j).
//
// Erros de linha são reportados via stderr mas não derrubam o ingest
// (F-010 D7).
func runIngestHRIS(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest hris", flag.ContinueOnError)
	csvPath := fs.String("csv", "", "caminho do CSV — obrigatório")
	tenant := fs.String("tenant", "", "tenant id (slot <account> da URN) — obrigatório")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (opcional; vazio → memory in-process)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	dryRun := fs.Bool("dry-run", false, "parseia mas não escreve")
	runID := fs.String("run-id", "", "identificador da execução (default: timestamp)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *csvPath == "" {
		return fmt.Errorf("%w: --csv obrigatório", errBadUsage)
	}
	if *tenant == "" {
		return fmt.Errorf("%w: --tenant obrigatório", errBadUsage)
	}

	f, err := os.Open(*csvPath)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	rows, rowErrs, err := hris.Parse(f)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	fmt.Printf("ingest hris: csv=%s tenant=%s rows=%d errors=%d\n",
		*csvPath, *tenant, len(rows), len(rowErrs))
	if len(rowErrs) > 0 {
		fmt.Fprintf(os.Stderr, "\nrow errors (%d):\n", len(rowErrs))
		for _, e := range rowErrs {
			fmt.Fprintf(os.Stderr, "  %s\n", e.Error())
		}
	}

	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("hris-%d", time.Now().Unix())
	}
	res, err := hris.Build(rows, hris.EmitOptions{
		Tenant: *tenant, RunID: rid, ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	fmt.Printf("  Teams:     %d\n", len(res.Teams))
	fmt.Printf("  Squads:    %d\n", len(res.Squads))
	fmt.Printf("  Persons:   %d (%d terminadas)\n", len(res.Persons), len(res.Terminated))
	fmt.Printf("  Edges:     %d (MemberOf=%d, PartOf=%d, ReportsTo=%d)\n",
		len(res.MemberOf)+len(res.PartOf)+len(res.ReportsTo),
		len(res.MemberOf), len(res.PartOf), len(res.ReportsTo))

	if *dryRun {
		fmt.Println("dry-run: nada gravado")
		return nil
	}

	var (
		nodeRepo repository.NodeRepository
		edgeRepo repository.EdgeRepository
	)
	if *neoURI == "" {
		m := memory.New()
		nodeRepo, edgeRepo = m, m.AsEdgeRepo()
		fmt.Println("backend: memory (in-process — nada persiste após o processo)")
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

	w := orgsvc.Writer{Nodes: nodeRepo, Edges: edgeRepo}
	st, err := w.Apply(ctx, res)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	fmt.Printf("ok: gravado teams=%d squads=%d persons=%d edges=%d terminated=%d\n",
		st.Teams, st.Squads, st.Persons, st.Edges, st.Terminated)
	return nil
}
