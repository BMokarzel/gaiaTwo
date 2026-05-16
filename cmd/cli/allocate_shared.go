package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/cost/allocate"
	"costEngine/internal/modules/cost/shared"
	"costEngine/internal/modules/cost/shared/rules"
	chrepo "costEngine/internal/repository/clickhouse"
)

// runAllocateShared é o handler de `ce allocate-shared` (F-006 S-005).
//
// Pipeline:
//  1. Carrega regras YAML de --rules.
//  2. Lê unalloc + direct totals do período em ClickHouse.
//  3. Roda shared.Apply.
//  4. (--reset) ALTER DELETE período/shared, depois insere.
//  5. Imprime relatório + invariante macro.
//
// Invariante macro reportada (não-fatal — drift > 1 cent vira warning):
//
//	prev_unalloc ≈ shared_emitted + remaining_unalloc + drops
func runAllocateShared(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("allocate-shared", flag.ContinueOnError)
	periodStr := fs.String("period", "", "BillingPeriod YYYY-MM (obrigatório)")
	chDSN := fs.String("clickhouse", "", "DSN ClickHouse — obrigatório")
	chDB := fs.String("database", "default", "database ClickHouse")
	rulesDir := fs.String("rules", "config/shared_rules", "diretório com YAMLs de regras")
	batchSize := fs.Int("batch-size", 5000, "batch size do sink")
	dryRun := fs.Bool("dry-run", false, "calcula e relata, sem escrever")
	reset := fs.Bool("reset", false, "ALTER DELETE shared no período antes de escrever (limpa órfãs)")
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

	rs, err := rules.Load(*rulesDir)
	if err != nil {
		return fmt.Errorf("rules: %w", err)
	}
	fmt.Printf("rules: carregadas %d de %s\n", len(rs), *rulesDir)
	if len(rs) == 0 {
		fmt.Println("nenhuma regra — nada a ratear")
		return nil
	}

	ch, err := chrepo.Open(ctx, chrepo.Config{Addrs: []string{*chDSN}, Database: *chDB})
	if err != nil {
		return fmt.Errorf("clickhouse: %w", err)
	}
	defer ch.Close()
	if err := ch.Migrate(ctx); err != nil {
		return fmt.Errorf("clickhouse migrate: %w", err)
	}

	unalloc, err := loadUnalloc(ctx, ch, period)
	if err != nil {
		return fmt.Errorf("load unalloc: %w", err)
	}
	totals, err := loadDirectTotals(ctx, ch, period)
	if err != nil {
		return fmt.Errorf("load direct totals: %w", err)
	}
	fmt.Printf("input: %d unalloc rows; %d direct totals\n", len(unalloc), len(totals))

	allocatedAt := time.Now().UTC()
	out, err := shared.Apply(shared.Input{
		Period:          period,
		Unalloc:         unalloc,
		AllocatedTotals: totals,
		Rules:           rs,
		AllocatedAt:     allocatedAt,
	})
	if err != nil {
		return fmt.Errorf("engine: %w", err)
	}

	printSharedReport(unalloc, out)

	if *dryRun {
		fmt.Println("dry-run: nada gravado")
		return nil
	}

	if *reset {
		if err := shared.ResetShared(ctx, ch.Conn, period); err != nil {
			return err
		}
		fmt.Println("reset: ALTER DELETE issued (async — pode levar segundos)")
	}

	if err := shared.Run(ctx, ch.Conn, out, *batchSize); err != nil {
		return fmt.Errorf("sink: %w", err)
	}

	// Invariante macro: drift > 1 cent vira warning não-fatal.
	prev := sumUnallocAmount(unalloc)
	emitted := sumRowAmount(out.Shared) + sumUnallocAmount(out.RemainingUnalloc)
	for _, d := range out.Drops {
		emitted += d.Amount
	}
	delta := math.Abs(prev - emitted)
	if delta > 0.01 {
		fmt.Fprintf(os.Stderr, "warning: invariante violada (prev=%.4f emitted=%.4f delta=%.4f)\n",
			prev, emitted, delta)
		return errPartial
	}
	return nil
}

func loadUnalloc(ctx context.Context, ch *chrepo.Client, period time.Time) ([]allocate.UnallocatedRow, error) {
	rows, err := ch.Conn.Query(ctx, chrepo.SelectUnallocatedSQL, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []allocate.UnallocatedRow
	for rows.Next() {
		var u allocate.UnallocatedRow
		var reason string
		if err := rows.Scan(&u.BillingPeriod, &u.AccountID, &u.Service, &u.ResourceID, &reason,
			&u.Amount, &u.Currency, &u.LineageCount, &u.AllocatedAt); err != nil {
			return nil, err
		}
		u.Reason = allocate.UnallocatedReason(reason)
		out = append(out, u)
	}
	return out, rows.Err()
}

func loadDirectTotals(ctx context.Context, ch *chrepo.Client, period time.Time) ([]shared.TotalByDim, error) {
	rows, err := ch.Conn.Query(ctx, chrepo.SelectDirectTotalsSQL, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []shared.TotalByDim
	for rows.Next() {
		var t shared.TotalByDim
		var urn string
		if err := rows.Scan(&urn, &t.Dimension, &t.DimensionValue, &t.Amount, &t.Currency); err != nil {
			return nil, err
		}
		t.URN = node.URN(urn)
		out = append(out, t)
	}
	return out, rows.Err()
}

func sumUnallocAmount(rs []allocate.UnallocatedRow) float64 {
	var s float64
	for _, r := range rs {
		s += r.Amount
	}
	return s
}

func sumRowAmount(rs []allocate.Row) float64 {
	var s float64
	for _, r := range rs {
		s += r.Amount
	}
	return s
}

func printSharedReport(unalloc []allocate.UnallocatedRow, out shared.Output) {
	prev := sumUnallocAmount(unalloc)
	shareSum := sumRowAmount(out.Shared)
	remSum := sumUnallocAmount(out.RemainingUnalloc)
	var dropSum float64
	for _, d := range out.Drops {
		dropSum += d.Amount
	}
	fmt.Printf("  prev unalloc: %d rows  %.4f\n", len(unalloc), prev)
	fmt.Printf("  shared:       %d rows  %.4f\n", len(out.Shared), shareSum)
	fmt.Printf("  remaining:    %d rows  %.4f\n", len(out.RemainingUnalloc), remSum)
	if len(out.Drops) > 0 {
		fmt.Printf("  drops:        %d  %.4f\n", len(out.Drops), dropSum)
		for _, d := range out.Drops {
			fmt.Fprintf(os.Stderr, "  drop rule=%s: %s (amount=%.4f)\n", d.RuleID, d.Reason, d.Amount)
		}
	}
}
