// Command ce é o CLI de operação do CostEngine.
//
// Subcomandos (S-001 entrega apenas o scaffold de `extract aws`):
//
//	ce extract aws --account=<id> --region=<region>
//	    Valida credenciais AWS via STS GetCallerIdentity e confirma que
//	    a conta autenticada coincide com --account. Não escreve no grafo.
//
// Stories S-002..S-008 já acopladas: topologia (Account/Region/Zone),
// EC2, EBS, S3, RDS, Network (VPC, Subnet, SG, LB) e resiliência a
// erros parciais (warnings agregadas, exit code 5).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"costEngine/internal/modules/infra/collector"
	awscol "costEngine/internal/modules/infra/collector/aws"
	infrasvc "costEngine/internal/modules/infra/service"
	"costEngine/internal/repository/memory"
)

const usage = `ce — CostEngine CLI

Uso:
  ce extract aws  --account=<id> --region=<region>
  ce extract code --repo=<name> --path=<dir> [--neo4j=<uri>] [--neo4j-user=<u>]
                  [--neo4j-pass=<p>] [--neo4j-db=<d>] [--dry-run]
  ce ingest cur  --source=<local|s3> [--root=<dir> | --bucket=<b> --prefix=<p>]
                 --report=<id> --clickhouse=<dsn> [--since=<YYYY-MM>]
                 [--batch-size=<n>] [--dry-run]
  ce ingest hris --csv=<path> --tenant=<id> [--neo4j=<uri>] [--neo4j-user=<u>]
                 [--neo4j-pass=<p>] [--neo4j-db=<d>] [--dry-run]
  ce ingest openapi --spec=<path> --service=<service_urn> [--neo4j=<uri>]
                 [--neo4j-user=<u>] [--neo4j-pass=<p>] [--neo4j-db=<d>]
                 [--run-id=<id>] [--dry-run]
  ce extract codeowners --repo=<path> --tenant=<id> [--neo4j=<uri>]
                 [--neo4j-user=<u>] [--neo4j-pass=<p>] [--neo4j-db=<d>]
                 [--dry-run]
  ce allocate    --period=<YYYY-MM> --clickhouse=<dsn> [--neo4j=<uri>]
                 [--dimensions=service,account,region]
                 [--batch-size=<n>] [--dry-run]
  ce allocate-shared --period=<YYYY-MM> --clickhouse=<dsn>
                 [--rules=<dir>] [--batch-size=<n>] [--dry-run] [--reset]
  ce bridge service-compute --account=<urn> [--neo4j=<uri>] [--neo4j-user=<u>]
                 [--neo4j-pass=<p>] [--neo4j-db=<d>] [--dry-run]
  ce gov <create|get|list|patch|delete> <kind> [<urn>] --tenant=<t>
                 [flags do kind] [--neo4j=<uri>] [--neo4j-user=<u>]
                 [--neo4j-pass=<p>] [--neo4j-db=<d>]
                 (kind: company | business-area | domain | capability | feature)
  ce help

`

// errBadUsage sinaliza erro de uso (exit code 2, convenção POSIX).
var errBadUsage = errors.New("uso inválido")

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args[1:]); err != nil {
		// errPartial: o detalhe já foi impresso pelo próprio runExtract
		// (warnings + relatório); não duplicar com prefixo "erro:".
		if !errors.Is(err, errPartial) {
			fmt.Fprintln(os.Stderr, "erro:", err)
		}
		os.Exit(exitCode(err))
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errBadUsage
	}

	switch args[0] {
	case "extract":
		return runExtract(ctx, args[1:])
	case "ingest":
		return runIngest(ctx, args[1:])
	case "allocate":
		return runAllocate(ctx, args[1:])
	case "allocate-shared":
		return runAllocateShared(ctx, args[1:])
	case "bridge":
		return runBridge(ctx, args[1:])
	case "gov":
		return runGov(ctx, args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return nil
	default:
		fmt.Fprintf(os.Stderr, "subcomando desconhecido: %q\n%s", args[0], usage)
		return errBadUsage
	}
}

func runExtract(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce extract <provider> [flags]", errBadUsage)
	}
	switch args[0] {
	case "aws":
		return runExtractAWS(ctx, args[1:])
	case "code":
		return runExtractCode(ctx, args[1:])
	case "codeowners":
		return runExtractCodeowners(ctx, args[1:])
	default:
		return fmt.Errorf("provider não suportado: %q (suportados: aws, code, codeowners)", args[0])
	}
}

func runExtractAWS(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract aws", flag.ContinueOnError)
	account := fs.String("account", "", "AWS account ID alvo (12 dígitos) — obrigatório")
	region := fs.String("region", "", "AWS region alvo (ex.: us-east-1) — obrigatório")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *account == "" || *region == "" {
		return fmt.Errorf("%w: --account e --region são obrigatórios", errBadUsage)
	}

	col, err := awscol.New(ctx, *region)
	if err != nil {
		return fmt.Errorf("init AWS collector: %w", err)
	}

	scope := collector.Scope{Account: *account, Region: *region}
	if err := col.Validate(ctx, scope); err != nil {
		return err
	}

	// S-002..S-007b: hierarquia Account → Region → Zone, Compute (EC2),
	// Persistence (EBS + S3 + RDS) e Network (VPC + Subnet + SG + LB).
	// Backend memory por enquanto (ephemeral); wiring de Neo4j vem em
	// story dedicada após S-008 — basta trocar o repo aqui.
	repo := memory.New()
	topoDisc := awscol.NewTopology(col.Config())
	ec2Disc := awscol.NewEC2(col.Config())
	ebsDisc := awscol.NewEBS(col.Config())
	s3Disc := awscol.NewS3(col.Config())
	rdsDisc := awscol.NewRDS(col.Config())
	vpcDisc := awscol.NewVPC(col.Config())
	sgDisc := awscol.NewSG(col.Config())
	lbDisc := awscol.NewLB(col.Config())
	svc := infrasvc.NewDiscoverService(repo, repo.AsEdgeRepo(), topoDisc).
		WithComputeDiscoverer(ec2Disc).
		WithPersistenceDiscoverer("ebs", ebsDisc).
		WithPersistenceDiscoverer("s3", s3Disc).
		WithPersistenceDiscoverer("rds", rdsDisc).
		WithNetworkDiscoverer("vpc", vpcDisc).
		WithNetworkDiscoverer("sg", sgDisc).
		WithNetworkDiscoverer("lb", lbDisc)

	// Topologia é pré-requisito de todo o resto: falha aqui aborta.
	scopeRep, topo, err := svc.EnsureScopeAncestorsWithTopology(ctx, scope)
	if err != nil {
		return err
	}

	// S-008: a partir daqui, capturamos cada erro de estágio mas seguimos
	// para os próximos. Compute é fonte única (1 erro = tudo zero);
	// Persistence/Network são slices (erro parcial sobrevive).
	var warnings []infrasvc.DiscoveryError
	compRep, err := svc.DiscoverCompute(ctx, scope, topo)
	if err != nil {
		warnings = append(warnings, infrasvc.DiscoveryError{Stage: "compute", Err: err})
	}
	persRep, err := svc.DiscoverPersistence(ctx, scope, topo)
	if err != nil {
		warnings = append(warnings, infrasvc.DiscoveryError{Stage: "persistence", Err: err})
	}
	warnings = append(warnings, persRep.Errors...)
	netRep, err := svc.DiscoverNetwork(ctx, scope, topo)
	if err != nil {
		warnings = append(warnings, infrasvc.DiscoveryError{Stage: "network", Err: err})
	}
	warnings = append(warnings, netRep.Errors...)

	if len(warnings) == 0 {
		fmt.Printf("ok: conta=%s região=%s\n", *account, *region)
	} else {
		fmt.Printf("parcial: conta=%s região=%s (%d falha(s) — ver warnings)\n",
			*account, *region, len(warnings))
	}
	fmt.Printf("  Scope:       %d accounts, %d regions, %d zones, %d edges\n",
		scopeRep.Accounts, scopeRep.Regions, scopeRep.Zones, scopeRep.Edges)
	fmt.Printf("  Compute:     %d total (%d new, %d updated, %d unchanged), %d edges\n",
		compRep.New+compRep.Updated+compRep.Unchanged, compRep.New, compRep.Updated, compRep.Unchanged, compRep.Edges)
	fmt.Printf("  Persistence: %d total (%d new, %d updated, %d unchanged), %d edges\n",
		persRep.New+persRep.Updated+persRep.Unchanged, persRep.New, persRep.Updated, persRep.Unchanged, persRep.Edges)
	fmt.Printf("  Network:     %d total (%d new, %d updated, %d unchanged), %d edges\n",
		netRep.New+netRep.Updated+netRep.Unchanged, netRep.New, netRep.Updated, netRep.Unchanged, netRep.Edges)
	if len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "\nwarnings (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", w.Stage, w.Err)
		}
		return errPartial
	}
	return nil
}

// errPartial sinaliza que a execução completou mas com erros parciais
// (algum discoverer falhou; outros estágios prosseguiram). CLI usa para
// retornar exit code 5 — scripts distinguem auth (3), account (4) e
// partial (5) de erros fatais (1).
var errPartial = errors.New("descoberta concluída com erros parciais")

// exitCode mapeia erros para códigos de saída úteis em scripts.
func exitCode(err error) int {
	switch {
	case errors.Is(err, errBadUsage):
		return 2
	case errors.Is(err, awscol.ErrAuth):
		return 3
	case errors.Is(err, awscol.ErrAccount):
		return 4
	case errors.Is(err, errPartial):
		return 5
	default:
		return 1
	}
}
