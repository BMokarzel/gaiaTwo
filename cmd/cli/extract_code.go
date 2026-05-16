package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"costEngine/internal/modules/code/collector/golang"
	codesvc "costEngine/internal/modules/code/service"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// runExtractCode é o handler de `ce extract code` (F-007 S-007).
//
// Pipeline:
//  1. golang.Collect(repoRoot) → walk + extract.
//  2. codesvc.Writer.Apply → grava Services/Endpoints/Functions/Edges.
//
// Backend:
//   - Sem `--neo4j`: usa `repository.memory` (visualizar apenas) — útil
//     em CI e desenvolvimento local. Equivalente a `--dry-run` se você
//     descarta o processo no fim.
//   - Com `--neo4j`: grava no grafo persistente.
//
// `--dry-run` roda Collect mas não chama Writer.Apply.
func runExtractCode(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract code", flag.ContinueOnError)
	repoName := fs.String("repo", "", "nome canônico do repositório (vai pro slot <account> da URN) — obrigatório")
	path := fs.String("path", ".", "caminho local do repositório")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (opcional; vazio → backend memory in-process)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	dryRun := fs.Bool("dry-run", false, "extrai mas não escreve no repository")
	runID := fs.String("run-id", "", "identificador da execução (default: timestamp)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repoName == "" {
		return fmt.Errorf("%w: --repo obrigatório", errBadUsage)
	}

	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("code-%d", time.Now().Unix())
	}

	cfg := golang.Config{Repo: *repoName, RunID: rid, ObservedAt: time.Now().UTC()}
	res, err := golang.Collect(*path, cfg)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}

	fmt.Printf("extract code: repo=%s path=%s run=%s\n", *repoName, *path, rid)
	fmt.Printf("  Services:  %d\n", len(res.Services))
	fmt.Printf("  Endpoints: %d\n", len(res.Endpoints))
	fmt.Printf("  Functions: %d\n", len(res.Functions))
	fmt.Printf("  Edges:     %d (DEFINED_IN)\n", len(res.Edges))

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

	w := codesvc.Writer{Nodes: nodeRepo, Edges: edgeRepo}
	st, err := w.Apply(ctx, res)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	fmt.Printf("ok: gravado services=%d endpoints=%d functions=%d edges=%d\n",
		st.Services, st.Endpoints, st.Functions, st.Edges)
	return nil
}
