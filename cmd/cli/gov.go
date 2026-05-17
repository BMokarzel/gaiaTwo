package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/gov"
	govservice "costEngine/internal/modules/gov/service"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// runGov é o dispatcher de `ce gov` (F-024 backlog).
//
// Espelha os endpoints REST `/v1/gov/...` em CLI:
//
//	ce gov create <kind> --tenant=<t> [flags]
//	ce gov get    <kind> <urn> --tenant=<t>
//	ce gov list   <kind> --tenant=<t> [--limit] [--offset]
//	ce gov patch  <kind> <urn> --tenant=<t> [flags]
//	ce gov delete <kind> <urn> --tenant=<t>
//
// Onde <kind> ∈ {company, business-area, domain, capability, feature}.
// O tenant é injetado no contexto via `httpserver.ContextWithTenant`
// (ADR-001): CLI não bypassa a regra de "tenant fora do payload".
func runGov(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce gov <create|get|list|patch|delete> <kind> [...]", errBadUsage)
	}
	op := args[0]
	rest := args[1:]
	if op == "help" || op == "-h" || op == "--help" {
		fmt.Fprint(os.Stdout, govUsage)
		return nil
	}
	if len(rest) == 0 {
		return fmt.Errorf("%w: ce gov %s <kind> [...]", errBadUsage, op)
	}
	kind := rest[0]
	rest = rest[1:]

	switch op {
	case "create":
		return runGovCreate(ctx, kind, rest)
	case "get":
		return runGovGet(ctx, kind, rest)
	case "list":
		return runGovList(ctx, kind, rest)
	case "patch":
		return runGovPatch(ctx, kind, rest)
	case "delete":
		return runGovDelete(ctx, kind, rest)
	default:
		return fmt.Errorf("%w: operação desconhecida %q", errBadUsage, op)
	}
}

const govUsage = `ce gov — CRUD da taxonomia product-arch (espelha REST /v1/gov)

Uso:
  ce gov create  <kind> --tenant=<t> [flags do kind]
  ce gov get     <kind> <urn> --tenant=<t>
  ce gov list    <kind> --tenant=<t> [--limit=<n>] [--offset=<n>]
  ce gov patch   <kind> <urn> --tenant=<t> [flags do kind]
  ce gov delete  <kind> <urn> --tenant=<t>

Kinds: company | business-area | domain | capability | feature
Backend (todas as ops): [--neo4j=<uri>] [--neo4j-user=<u>] [--neo4j-pass=<p>]
                       [--neo4j-db=<d>]  — vazio = memory in-process.

Flags por kind:
  company:        --short-id  --name  [--domain]
  business-area:  --short-id  --name  --company-urn
  domain:         --short-id  --name  --parent-area-urn      [--description]
  capability:     --short-id  --name  --parent-domain-urn    [--description]
  feature:        --short-id  --name  --parent-capability-urn
                  [--parent-feature-urn] [--description]
                  [--status] [--sla] [--priority]

Patch aceita os mesmos campos (exceto short-id e parent*).
`

// ---------- backend wiring ----------

type govBackend struct {
	svc   gov.Service
	close func(context.Context) error
}

// buildGovBackend constrói o backend a partir das flags Neo4j (vazias =
// memory). Retorna função de cleanup (no-op em memory).
func buildGovBackend(ctx context.Context, neoURI, user, pass, db string) (govBackend, error) {
	if neoURI == "" {
		m := memory.New()
		return govBackend{
			svc:   govservice.New(m, m.AsEdgeRepo()),
			close: func(context.Context) error { return nil },
		}, nil
	}
	nc, err := n4j.Connect(ctx, n4j.Config{URI: neoURI, Username: user, Password: pass, Database: db})
	if err != nil {
		return govBackend{}, fmt.Errorf("neo4j connect: %w", err)
	}
	if err := nc.Migrate(ctx); err != nil {
		_ = nc.Close(ctx)
		return govBackend{}, fmt.Errorf("neo4j migrate: %w", err)
	}
	nodes := n4j.NewNodeRepo(nc)
	edges := n4j.NewEdgeRepo(nc)
	return govBackend{
		svc:   govservice.New(nodes, edges),
		close: func(ctx context.Context) error { return nc.Close(ctx) },
	}, nil
}

// govCommonFlags agrupa as flags compartilhadas: tenant + backend Neo4j.
type govCommonFlags struct {
	tenant  string
	neoURI  string
	neoUser string
	neoPass string
	neoDB   string
}

func attachGovCommon(fs *flag.FlagSet) *govCommonFlags {
	c := &govCommonFlags{}
	fs.StringVar(&c.tenant, "tenant", "", "tenant id (slot <account> da URN) — obrigatório")
	fs.StringVar(&c.neoURI, "neo4j", "", "URI bolt:// do Neo4j (vazio → memory in-process)")
	fs.StringVar(&c.neoUser, "neo4j-user", "", "usuário Neo4j")
	fs.StringVar(&c.neoPass, "neo4j-pass", "", "senha Neo4j")
	fs.StringVar(&c.neoDB, "neo4j-db", "", "database Neo4j")
	return c
}

func (c *govCommonFlags) requireTenant() error {
	if c.tenant == "" {
		return fmt.Errorf("%w: --tenant obrigatório", errBadUsage)
	}
	return nil
}

func (c *govCommonFlags) ctx(ctx context.Context) context.Context {
	return httpserver.ContextWithTenant(ctx, c.tenant)
}

// printJSON serializa um valor com indent para stdout. Erros de
// serialização derrubam a CLI (programa não tem nada a fazer).
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

// ---------- CREATE ----------

func runGovCreate(ctx context.Context, kind string, args []string) error {
	fs := flag.NewFlagSet("gov create "+kind, flag.ContinueOnError)
	common := attachGovCommon(fs)

	shortID := fs.String("short-id", "", "short-id kebab-case (obrigatório)")
	name := fs.String("name", "", "nome humano (obrigatório)")
	description := fs.String("description", "", "descrição opcional")
	domain := fs.String("domain", "", "domínio externo (company)")
	companyURN := fs.String("company-urn", "", "URN da Company pai (business-area)")
	parentAreaURN := fs.String("parent-area-urn", "", "URN da BusinessArea pai (domain)")
	parentDomainURN := fs.String("parent-domain-urn", "", "URN do Domain pai (capability)")
	parentCapURN := fs.String("parent-capability-urn", "", "URN da Capability pai (feature)")
	parentFeatureURN := fs.String("parent-feature-urn", "", "URN da Feature pai (sub-feature)")
	status := fs.String("status", "", "status (feature: active|deprecated)")
	sla := fs.String("sla", "", "SLA (feature)")
	priority := fs.String("priority", "", "priority (feature)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := common.requireTenant(); err != nil {
		return err
	}
	if *shortID == "" || *name == "" {
		return fmt.Errorf("%w: --short-id e --name obrigatórios", errBadUsage)
	}

	be, err := buildGovBackend(ctx, common.neoURI, common.neoUser, common.neoPass, common.neoDB)
	if err != nil {
		return err
	}
	defer be.close(ctx)
	c := common.ctx(ctx)

	switch kind {
	case "company":
		out, err := be.svc.CreateCompany(c, gov.CreateCompanyInput{ShortID: *shortID, Name: *name, Domain: *domain})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "business-area":
		if *companyURN == "" {
			return fmt.Errorf("%w: --company-urn obrigatório", errBadUsage)
		}
		out, err := be.svc.CreateBusinessArea(c, gov.CreateBusinessAreaInput{
			ShortID: *shortID, Name: *name, CompanyURN: node.URN(*companyURN),
		})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "domain":
		if *parentAreaURN == "" {
			return fmt.Errorf("%w: --parent-area-urn obrigatório", errBadUsage)
		}
		out, err := be.svc.CreateDomain(c, gov.CreateDomainInput{
			ShortID: *shortID, Name: *name, Description: *description,
			ParentAreaURN: node.URN(*parentAreaURN),
		})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "capability":
		if *parentDomainURN == "" {
			return fmt.Errorf("%w: --parent-domain-urn obrigatório", errBadUsage)
		}
		out, err := be.svc.CreateCapability(c, gov.CreateCapabilityInput{
			ShortID: *shortID, Name: *name, Description: *description,
			ParentDomainURN: node.URN(*parentDomainURN),
		})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "feature":
		if *parentCapURN == "" {
			return fmt.Errorf("%w: --parent-capability-urn obrigatório", errBadUsage)
		}
		out, err := be.svc.CreateFeature(c, gov.CreateFeatureInput{
			ShortID: *shortID, Name: *name, Description: *description,
			ParentCapURN:     node.URN(*parentCapURN),
			ParentFeatureURN: node.URN(*parentFeatureURN),
			Status:           *status, SLA: *sla, Priority: *priority,
		})
		if err != nil {
			return err
		}
		return printJSON(out)
	default:
		return fmt.Errorf("%w: kind desconhecido %q", errBadUsage, kind)
	}
}

// ---------- GET ----------

func runGovGet(ctx context.Context, kind string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce gov get %s <urn> --tenant=<t>", errBadUsage, kind)
	}
	urn := node.URN(args[0])
	fs := flag.NewFlagSet("gov get "+kind, flag.ContinueOnError)
	common := attachGovCommon(fs)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := common.requireTenant(); err != nil {
		return err
	}
	be, err := buildGovBackend(ctx, common.neoURI, common.neoUser, common.neoPass, common.neoDB)
	if err != nil {
		return err
	}
	defer be.close(ctx)
	c := common.ctx(ctx)

	switch kind {
	case "company":
		out, err := be.svc.GetCompany(c, urn, gov.AsOfOptions{})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "business-area":
		out, err := be.svc.GetBusinessArea(c, urn, gov.AsOfOptions{})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "domain":
		out, err := be.svc.GetDomain(c, urn, gov.AsOfOptions{})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "capability":
		out, err := be.svc.GetCapability(c, urn, gov.AsOfOptions{})
		if err != nil {
			return err
		}
		return printJSON(out)
	case "feature":
		out, err := be.svc.GetFeature(c, urn, gov.AsOfOptions{})
		if err != nil {
			return err
		}
		return printJSON(out)
	default:
		return fmt.Errorf("%w: kind desconhecido %q", errBadUsage, kind)
	}
}

// ---------- LIST ----------

func runGovList(ctx context.Context, kind string, args []string) error {
	fs := flag.NewFlagSet("gov list "+kind, flag.ContinueOnError)
	common := attachGovCommon(fs)
	limit := fs.Int("limit", 0, "tamanho da página (0 = default do servidor)")
	offset := fs.Int("offset", 0, "offset para paginação")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := common.requireTenant(); err != nil {
		return err
	}
	be, err := buildGovBackend(ctx, common.neoURI, common.neoUser, common.neoPass, common.neoDB)
	if err != nil {
		return err
	}
	defer be.close(ctx)
	c := common.ctx(ctx)
	q := gov.ListQuery{Limit: *limit, Offset: *offset}

	switch kind {
	case "company":
		page, err := be.svc.ListCompanies(c, q)
		if err != nil {
			return err
		}
		return printJSON(page)
	case "business-area":
		page, err := be.svc.ListBusinessAreas(c, q)
		if err != nil {
			return err
		}
		return printJSON(page)
	case "domain":
		page, err := be.svc.ListDomains(c, q)
		if err != nil {
			return err
		}
		return printJSON(page)
	case "capability":
		page, err := be.svc.ListCapabilities(c, q)
		if err != nil {
			return err
		}
		return printJSON(page)
	case "feature":
		page, err := be.svc.ListFeatures(c, q)
		if err != nil {
			return err
		}
		return printJSON(page)
	default:
		return fmt.Errorf("%w: kind desconhecido %q", errBadUsage, kind)
	}
}

// ---------- PATCH ----------

// strPtrIfSet devolve um *string somente se a flag foi presente. Usa
// flag.Visit para distinguir "não enviado" de "enviado vazio" (PATCH
// semântico do REST).
func strPtrIfSet(fs *flag.FlagSet, name string, dest string) *string {
	var found bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	if !found {
		return nil
	}
	v := dest
	return &v
}

func runGovPatch(ctx context.Context, kind string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce gov patch %s <urn> --tenant=<t> [flags]", errBadUsage, kind)
	}
	urn := node.URN(args[0])
	fs := flag.NewFlagSet("gov patch "+kind, flag.ContinueOnError)
	common := attachGovCommon(fs)
	name := fs.String("name", "", "novo nome")
	description := fs.String("description", "", "nova descrição")
	domain := fs.String("domain", "", "novo domínio (company)")
	status := fs.String("status", "", "novo status (feature)")
	sla := fs.String("sla", "", "novo SLA (feature)")
	priority := fs.String("priority", "", "nova prioridade (feature)")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := common.requireTenant(); err != nil {
		return err
	}
	be, err := buildGovBackend(ctx, common.neoURI, common.neoUser, common.neoPass, common.neoDB)
	if err != nil {
		return err
	}
	defer be.close(ctx)
	c := common.ctx(ctx)

	switch kind {
	case "company":
		in := gov.UpdateCompanyInput{
			Name:   strPtrIfSet(fs, "name", *name),
			Domain: strPtrIfSet(fs, "domain", *domain),
		}
		out, err := be.svc.UpdateCompany(c, urn, in)
		if err != nil {
			return err
		}
		return printJSON(out)
	case "business-area":
		in := gov.UpdateBusinessAreaInput{Name: strPtrIfSet(fs, "name", *name)}
		out, err := be.svc.UpdateBusinessArea(c, urn, in)
		if err != nil {
			return err
		}
		return printJSON(out)
	case "domain":
		in := gov.UpdateDomainInput{
			Name:        strPtrIfSet(fs, "name", *name),
			Description: strPtrIfSet(fs, "description", *description),
		}
		out, err := be.svc.UpdateDomain(c, urn, in)
		if err != nil {
			return err
		}
		return printJSON(out)
	case "capability":
		in := gov.UpdateCapabilityInput{
			Name:        strPtrIfSet(fs, "name", *name),
			Description: strPtrIfSet(fs, "description", *description),
		}
		out, err := be.svc.UpdateCapability(c, urn, in)
		if err != nil {
			return err
		}
		return printJSON(out)
	case "feature":
		in := gov.UpdateFeatureInput{
			Name:        strPtrIfSet(fs, "name", *name),
			Description: strPtrIfSet(fs, "description", *description),
			Status:      strPtrIfSet(fs, "status", *status),
			SLA:         strPtrIfSet(fs, "sla", *sla),
			Priority:    strPtrIfSet(fs, "priority", *priority),
		}
		out, err := be.svc.UpdateFeature(c, urn, in)
		if err != nil {
			return err
		}
		return printJSON(out)
	default:
		return fmt.Errorf("%w: kind desconhecido %q", errBadUsage, kind)
	}
}

// ---------- DELETE ----------

func runGovDelete(ctx context.Context, kind string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ce gov delete %s <urn> --tenant=<t>", errBadUsage, kind)
	}
	urn := node.URN(args[0])
	fs := flag.NewFlagSet("gov delete "+kind, flag.ContinueOnError)
	common := attachGovCommon(fs)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := common.requireTenant(); err != nil {
		return err
	}
	be, err := buildGovBackend(ctx, common.neoURI, common.neoUser, common.neoPass, common.neoDB)
	if err != nil {
		return err
	}
	defer be.close(ctx)
	c := common.ctx(ctx)

	// Repository memory perde estado entre processos — delete é
	// inútil. Avise o usuário; ainda assim executa para consistência.
	if common.neoURI == "" {
		fmt.Fprintln(os.Stderr, "warn: backend memory — delete não persiste (use --neo4j para gravar)")
	}

	switch kind {
	case "company":
		err = be.svc.DeleteCompany(c, urn)
	case "business-area":
		err = be.svc.DeleteBusinessArea(c, urn)
	case "domain":
		err = be.svc.DeleteDomain(c, urn)
	case "capability":
		err = be.svc.DeleteCapability(c, urn)
	case "feature":
		err = be.svc.DeleteFeature(c, urn)
	default:
		return fmt.Errorf("%w: kind desconhecido %q", errBadUsage, kind)
	}
	if err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}
