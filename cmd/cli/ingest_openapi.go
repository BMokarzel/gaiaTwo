package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/code/ingest/openapi"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// runIngestOpenAPI é o handler de `ce ingest openapi` (F-008).
//
// Pipeline:
//  1. Parse + Collect → Result{Endpoints, Edges}.
//  2. Writer.Apply → grava via repositório (memory ou Neo4j).
//
// Idempotência fica delegada ao repo bitemporal: re-rodar sem mudanças
// não cria versão nova.
func runIngestOpenAPI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest openapi", flag.ContinueOnError)
	specPath := fs.String("spec", "", "caminho local da spec OpenAPI (YAML ou JSON) — obrigatório")
	serviceURN := fs.String("service", "", "URN do Service alvo (urn:ce:<prov>:<acct>:service/<id>) — obrigatório")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (vazio → memory in-process)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	runID := fs.String("run-id", "", "identificador da execução (default: timestamp)")
	dryRun := fs.Bool("dry-run", false, "parseia mas não escreve")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *specPath == "" {
		return fmt.Errorf("%w: --spec obrigatório", errBadUsage)
	}
	if *serviceURN == "" {
		return fmt.Errorf("%w: --service obrigatório", errBadUsage)
	}

	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("openapi-%d", time.Now().Unix())
	}

	res, err := openapi.Collect(*specPath, openapi.Config{
		ServiceURN: node.URN(*serviceURN),
		RunID:      rid,
		ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}

	fmt.Printf("ingest openapi: spec=%s service=%s\n", *specPath, *serviceURN)
	fmt.Printf("  spec version: %s\n", res.SpecVersion)
	fmt.Printf("  Endpoints:    %d\n", len(res.Endpoints))
	fmt.Printf("  Schemas:      %d\n", len(res.Schemas))
	fmt.Printf("  Edges:        %d (DefinedIn=%d, Imports=%d)\n",
		len(res.Edges)+len(res.Imports), len(res.Edges), len(res.Imports))

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
		// Memory backend exige Service pré-existente para o writer; em
		// memory, esse uso normalmente é só smoke-test. Avise.
		fmt.Println("warn: memory backend não tem Service pré-cadastrado — apply falhará. Use --dry-run ou --neo4j.")
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

	w := openapi.Writer{Nodes: nodeRepo, Edges: edgeRepo}
	st, err := w.Apply(ctx, res, node.URN(*serviceURN))
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	fmt.Printf("ok: endpoints=%d schemas=%d edges=%d imports=%d\n",
		st.Endpoints, st.Schemas, st.Edges, st.Imports)
	return nil
}
