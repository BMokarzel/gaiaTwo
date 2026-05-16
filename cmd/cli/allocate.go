package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
	"costEngine/internal/modules/cost/allocate"
	"costEngine/internal/repository"
	chrepo "costEngine/internal/repository/clickhouse"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"

	"flag"
)

// runAllocate é o handler do `ce allocate` (F-005 S-007).
//
// Pipeline:
//  1. Conecta no ClickHouse (e Neo4j se fornecido).
//  2. Roda allocate.Engine.Run(period).
//  3. Imprime Report + invariante TotalCUR ≈ TotalAllocated + TotalUnallocated.
//
// Casos:
//   - Sem `--neo4j`: usa repository.memory (vazio) → TUDO vai pra
//     fct_unallocated_cost com reason=not_found. Útil para validar
//     pipeline sem o grafo populado.
//   - Com `--neo4j`: usa bridge.New(n4j) → resolução real.
func runAllocate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("allocate", flag.ContinueOnError)
	periodStr := fs.String("period", "", "BillingPeriod YYYY-MM (obrigatório)")
	chDSN := fs.String("clickhouse", "", "DSN ClickHouse — obrigatório (a menos de --dry-run sem source)")
	chDB := fs.String("database", "default", "database ClickHouse")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (opcional; vazio → tudo unallocated)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	dimsStr := fs.String("dimensions", "service,account,region", "dimensões a alocar")
	batchSize := fs.Int("batch-size", 5000, "batch size do sink")
	dryRun := fs.Bool("dry-run", false, "calcula tudo mas não escreve em fct_cost_by_urn / fct_unallocated_cost")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *periodStr == "" {
		return fmt.Errorf("%w: --period obrigatório", errBadUsage)
	}
	period, err := time.Parse("2006-01", *periodStr)
	if err != nil {
		return fmt.Errorf("%w: --period %q: %v", errBadUsage, *periodStr, err)
	}
	if *chDSN == "" {
		return fmt.Errorf("%w: --clickhouse obrigatório", errBadUsage)
	}

	dims, err := parseDimensions(*dimsStr)
	if err != nil {
		return err
	}

	// ClickHouse — origem (fct_cur_lines) + destino (fct_cost_by_urn / fct_unallocated_cost).
	ch, err := chrepo.Open(ctx, chrepo.Config{Addrs: []string{*chDSN}, Database: *chDB})
	if err != nil {
		return fmt.Errorf("clickhouse: %w", err)
	}
	defer ch.Close()
	if err := ch.Migrate(ctx); err != nil {
		return fmt.Errorf("clickhouse migrate: %w", err)
	}

	// Resolver — Neo4j (se fornecido) ou memory vazio.
	var nodeRepo repository.NodeRepository
	if *neoURI != "" {
		nc, err := n4j.Connect(ctx, n4j.Config{
			URI: *neoURI, Username: *neoUser, Password: *neoPass, Database: *neoDB,
		})
		if err != nil {
			return fmt.Errorf("neo4j: %w", err)
		}
		defer nc.Close(ctx)
		nodeRepo = n4j.NewNodeRepo(nc)
	} else {
		fmt.Fprintln(os.Stderr, "warning: --neo4j vazio → resolver retorna NotFound para tudo (debug/seed)")
		nodeRepo = memory.New()
	}
	resolver := bridge.New(nodeRepo)

	engine := &allocate.Engine{
		Source:   &allocate.ClickHouseSource{Conn: ch.Conn},
		Resolver: resolver,
		Provider: node.ProviderAWS,
		Dims:     dims,
		DryRun:   *dryRun,
	}
	if !*dryRun {
		engine.Sink = &allocate.Sink{Conn: ch.Conn, BatchSize: *batchSize}
	}

	rep, err := engine.Run(ctx, period)
	if err != nil {
		return err
	}

	printAllocateReport(rep, *dryRun)

	// Invariante macro: detecta drift > 1 cent.
	delta := math.Abs(rep.TotalCUR - (rep.TotalAllocated + rep.TotalUnallocated))
	if delta > 0.01 {
		fmt.Fprintf(os.Stderr, "warning: invariante violada (delta=%.4f)\n", delta)
		return errPartial
	}
	return nil
}

func parseDimensions(s string) ([]allocate.DimensionResolver, error) {
	parts := strings.Split(s, ",")
	dims := make([]allocate.Dimension, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		switch allocate.Dimension(p) {
		case allocate.DimensionService, allocate.DimensionAccount, allocate.DimensionRegion:
			dims = append(dims, allocate.Dimension(p))
		default:
			return nil, fmt.Errorf("%w: dimensão desconhecida: %q", errBadUsage, p)
		}
	}
	return allocate.ResolversFor(dims), nil
}

func printAllocateReport(r allocate.Report, dryRun bool) {
	mode := ""
	if dryRun {
		mode = " (dry-run, nada escrito)"
	}
	fmt.Printf("ok: allocate %s%s em %s\n",
		r.BillingPeriod.Format("2006-01"), mode, r.Duration().Round(time.Millisecond))
	fmt.Printf("  CUR lines:      %d (unique res_id=%d)\n", r.CURLinesRead, r.UniqueResIDs)
	fmt.Printf("  Allocated:      %d rows  %.4f\n", r.Allocated, r.TotalAllocated)
	fmt.Printf("  Unallocated:    %d rows  %.4f\n", r.Unallocated, r.TotalUnallocated)
	fmt.Printf("  TotalCUR:       %.4f  (delta=%.4f)\n",
		r.TotalCUR, r.TotalCUR-(r.TotalAllocated+r.TotalUnallocated))
}
