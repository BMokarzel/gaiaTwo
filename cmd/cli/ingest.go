package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"costEngine/internal/entity/cost"
	"costEngine/internal/modules/cost/sink"
	"costEngine/internal/modules/cost/source/local"
	"costEngine/internal/modules/cost/source/s3"
	chrepo "costEngine/internal/repository/clickhouse"
)

func runIngest(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce ingest <kind> [flags]", errBadUsage)
	}
	switch args[0] {
	case "cur":
		return runIngestCUR(ctx, args[1:])
	case "hris":
		return runIngestHRIS(ctx, args[1:])
	case "openapi":
		return runIngestOpenAPI(ctx, args[1:])
	default:
		return fmt.Errorf("kind não suportado: %q (suportados: cur, hris, openapi)", args[0])
	}
}

func runIngestCUR(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest cur", flag.ContinueOnError)
	source := fs.String("source", "local", "origem: local|s3")
	root := fs.String("root", "", "diretório raiz (source=local)")
	bucket := fs.String("bucket", "", "bucket S3 (source=s3)")
	prefix := fs.String("prefix", "", "prefixo S3 do report (source=s3)")
	report := fs.String("report", "", "identificador do relatório (obrigatório)")
	chDSN := fs.String("clickhouse", "", "DSN ClickHouse (ex.: tcp://localhost:9000) — obrigatório a menos de --dry-run")
	chDB := fs.String("database", "default", "database ClickHouse")
	since := fs.String("since", "", "filtra partitions a partir de YYYY-MM")
	batchSize := fs.Int("batch-size", 5000, "tamanho do batch de INSERT")
	dryRun := fs.Bool("dry-run", false, "lê e conta linhas, mas não escreve")
	region := fs.String("region", "", "região AWS (source=s3); default usa cadeia padrão SDK")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *report == "" {
		return fmt.Errorf("%w: --report obrigatório", errBadUsage)
	}
	if !*dryRun && *chDSN == "" {
		return fmt.Errorf("%w: --clickhouse obrigatório (ou use --dry-run)", errBadUsage)
	}

	var sinceT time.Time
	if *since != "" {
		t, err := time.Parse("2006-01", *since)
		if err != nil {
			return fmt.Errorf("%w: --since %q: %v", errBadUsage, *since, err)
		}
		sinceT = t
	}

	importer, err := buildImporter(ctx, *source, *root, *bucket, *prefix, *report, *region)
	if err != nil {
		return err
	}

	parts, err := importer.Discover(ctx, sinceT)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	if len(parts) == 0 {
		fmt.Println("ok: nenhuma partition encontrada — nada a fazer")
		return nil
	}

	var snk sink.ClickHouseSink
	var chClient *chrepo.Client
	if !*dryRun {
		c, err := chrepo.Open(ctx, chrepo.Config{
			Addrs:    []string{*chDSN},
			Database: *chDB,
		})
		if err != nil {
			return fmt.Errorf("clickhouse: %w", err)
		}
		defer c.Close()
		if err := c.Migrate(ctx); err != nil {
			return fmt.Errorf("clickhouse migrate: %w", err)
		}
		chClient = c
		snk = sink.ClickHouseSink{Conn: c.Conn, BatchSize: *batchSize}
		_ = chClient
	}

	totalLines := int64(0)
	totalErrors := int64(0)
	totalBatches := 0
	start := time.Now()

	for _, p := range parts {
		fmt.Printf("→ partition %s (%d objects)\n", p.BillingMonth.Format("2006-01"), len(p.ObjectKeys))
		res, err := importer.Read(ctx, p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  erro abrindo partition: %v\n", err)
			continue
		}

		if *dryRun {
			lines, errs := drainDryRun(res)
			totalLines += lines
			totalErrors += errs
			continue
		}

		rep, err := snk.Run(ctx, res.Lines, res.Errors)
		totalLines += rep.LinesInserted
		totalErrors += rep.ErrorsInserted
		totalBatches += rep.Batches
		if err != nil {
			return fmt.Errorf("sink: %w", err)
		}
	}

	fmt.Println()
	fmt.Printf("ok: %d linhas, %d erros, %d batches em %s\n",
		totalLines, totalErrors, totalBatches, time.Since(start).Round(time.Millisecond))
	if totalErrors > 0 {
		return errPartial
	}
	return nil
}

// importerIface é o subset de `cost.CURImporter` usado pelo runner.
type importerIface interface {
	Discover(ctx context.Context, since time.Time) ([]cost.Partition, error)
	Read(ctx context.Context, p cost.Partition) (cost.ReadResult, error)
}

func buildImporter(ctx context.Context, kind, root, bucket, prefix, report, region string) (importerIface, error) {
	switch strings.ToLower(kind) {
	case "local":
		if root == "" {
			return nil, fmt.Errorf("%w: --root obrigatório para source=local", errBadUsage)
		}
		return &local.Importer{Root: root, ReportID: report}, nil
	case "s3":
		if bucket == "" || prefix == "" {
			return nil, fmt.Errorf("%w: --bucket e --prefix obrigatórios para source=s3", errBadUsage)
		}
		opts := []func(*config.LoadOptions) error{}
		if region != "" {
			opts = append(opts, config.WithRegion(region))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("aws config: %w", err)
		}
		return &s3.Importer{
			Client:   awss3.NewFromConfig(cfg),
			Bucket:   bucket,
			Prefix:   prefix,
			ReportID: report,
		}, nil
	default:
		return nil, fmt.Errorf("source não suportada: %q", kind)
	}
}

func drainDryRun(res cost.ReadResult) (int64, int64) {
	var lines, errs int64
	doneL, doneE := false, false
	for !doneL || !doneE {
		select {
		case _, ok := <-res.Lines:
			if !ok {
				doneL = true
				res.Lines = nil
				continue
			}
			lines++
		case _, ok := <-res.Errors:
			if !ok {
				doneE = true
				res.Errors = nil
				continue
			}
			errs++
		}
	}
	return lines, errs
}
