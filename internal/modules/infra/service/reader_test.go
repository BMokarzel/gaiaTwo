package service_test

import (
	"context"
	"errors"
	"testing"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra"
	"costEngine/internal/modules/infra/service"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// buildInfraFixture popula um memory.Repo com uma topologia mínima:
//   - 1 Account (AWS) com 1 Region us-east-1, 1 Zone us-east-1a
//   - 1 Compute, 1 Persistence, 1 Network nessa Region/Account
//
// Devolve `infra.Service` apontando para o mesmo repo, pronto para reads.
func buildInfraFixture(t *testing.T) (infra.Service, fixtureURNs) {
	t.Helper()
	const account = "123456789012"
	const region = "us-east-1"
	const zone = "us-east-1a"

	accountURN := node.NewURN(node.ProviderAWS, account, node.KindAccount, account)
	regionURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	zoneURN := node.NewURN(node.ProviderAWS, account, node.KindZone, zone)
	computeURN := node.NewURN(node.ProviderAWS, account, node.KindCompute, "i-aaa")
	persistURN := node.NewURN(node.ProviderAWS, account, node.KindPersistence, "vol-bbb")
	networkURN := node.NewURN(node.ProviderAWS, account, node.KindNetwork, "vpc-ccc")

	meta := node.Meta{Version: 1, Confidence: 1}
	repo := memory.New()
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	must(repo.Upsert(ctx, node.Account{
		Base:        node.Base{NodeURN: accountURN, NodeKind: node.KindAccount, NodeMeta: meta},
		ProviderID:  node.ProviderAWS,
		ExternalID:  account,
		DisplayName: "acme-prod",
	}))
	must(repo.Upsert(ctx, node.Region{
		Base:       node.Base{NodeURN: regionURN, NodeKind: node.KindRegion, NodeMeta: meta},
		ProviderID: node.ProviderAWS,
		Code:       region,
	}))
	must(repo.Upsert(ctx, node.Zone{
		Base:      node.Base{NodeURN: zoneURN, NodeKind: node.KindZone, NodeMeta: meta},
		RegionURN: regionURN,
		Code:      zone,
	}))
	must(repo.Upsert(ctx, node.Compute{
		Base:         node.Base{NodeURN: computeURN, NodeKind: node.KindCompute, NodeMeta: meta},
		ProviderID:   node.ProviderAWS,
		Account:      accountURN,
		Region:       regionURN,
		ExtID:        "i-aaa",
		Flavor:       node.ComputeVM,
		InstanceType: "m6i.large",
	}))
	must(repo.Upsert(ctx, node.Persistence{
		Base:       node.Base{NodeURN: persistURN, NodeKind: node.KindPersistence, NodeMeta: meta},
		ProviderID: node.ProviderAWS,
		Account:    accountURN,
		Region:     regionURN,
		ExtID:      "vol-bbb",
		Flavor:     node.PersistenceBlock,
	}))
	must(repo.Upsert(ctx, node.Network{
		Base:       node.Base{NodeURN: networkURN, NodeKind: node.KindNetwork, NodeMeta: meta},
		ProviderID: node.ProviderAWS,
		Account:    accountURN,
		Region:     regionURN,
		ExtID:      "vpc-ccc",
		Flavor:     node.NetworkVPC,
	}))

	// Edge Contains Account→Region (necessário para countRegionsOfAccount).
	er := repo.AsEdgeRepo()
	must(er.Upsert(ctx, edge.Contains{Base: edge.Base{
		EdgeID:   "edge-ar",
		EdgeType: edge.TypeContains,
		FromURN:  accountURN, ToURN: regionURN,
		EdgeMeta: edge.Meta{Directional: true, Confidence: 1},
	}}, node.KindAccount, node.KindRegion))

	svc := service.NewReader(repo, er)
	return svc, fixtureURNs{
		Account: accountURN,
		Region:  regionURN,
		Zone:    zoneURN,
		Compute: computeURN,
		Persist: persistURN,
		Network: networkURN,
	}
}

type fixtureURNs struct {
	Account node.URN
	Region  node.URN
	Zone    node.URN
	Compute node.URN
	Persist node.URN
	Network node.URN
}

func TestReader_ListAccounts(t *testing.T) {
	svc, f := buildInfraFixture(t)
	p, err := svc.ListAccounts(context.Background(), infra.ListAccountsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(p.Items) != 1 {
		t.Fatalf("len=%d want=1", len(p.Items))
	}
	if p.Items[0].Account.URN() != f.Account {
		t.Errorf("URN=%s want=%s", p.Items[0].Account.URN(), f.Account)
	}
	if p.Items[0].RegionCount != 1 {
		t.Errorf("RegionCount=%d want=1", p.Items[0].RegionCount)
	}
}

func TestReader_ListAccounts_ProviderFilter(t *testing.T) {
	svc, _ := buildInfraFixture(t)
	p, err := svc.ListAccounts(context.Background(), infra.ListAccountsQuery{
		Provider: node.ProviderGCP, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(p.Items) != 0 {
		t.Errorf("expected zero GCP accounts in fixture, got %d", len(p.Items))
	}
}

func TestReader_GetAccount_NotFound_TypedError(t *testing.T) {
	svc, _ := buildInfraFixture(t)
	_, err := svc.GetAccount(
		context.Background(),
		node.NewURN(node.ProviderAWS, "999", node.KindAccount, "999"),
		infra.AsOfOptions{},
	)
	var typed *infra.ErrAccountNotFound
	if !errors.As(err, &typed) {
		t.Fatalf("expected *infra.ErrAccountNotFound, got %T", err)
	}
	if typed.HTTPStatus() != 404 || typed.Code() != "infra.account.not_found" {
		t.Errorf("status/code: %d / %q", typed.HTTPStatus(), typed.Code())
	}
}

func TestReader_GetAccount_WrongKind_InvalidURN(t *testing.T) {
	svc, f := buildInfraFixture(t)
	_, err := svc.GetAccount(context.Background(), f.Compute, infra.AsOfOptions{})
	var inv *infra.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *infra.ErrInvalidURN, got %T", err)
	}
}

func TestReader_GetAccount_DerivesCounts(t *testing.T) {
	svc, f := buildInfraFixture(t)
	d, err := svc.GetAccount(context.Background(), f.Account, infra.AsOfOptions{})
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if d.ComputeCount != 1 || d.PersistenceCount != 1 || d.NetworkCount != 1 {
		t.Errorf("counts = %+v, want all 1", d)
	}
}

func TestReader_ListRegions(t *testing.T) {
	svc, f := buildInfraFixture(t)
	p, err := svc.ListRegions(context.Background(), infra.ListRegionsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].Region.URN() != f.Region {
		t.Errorf("regions=%+v want one with URN %s", p.Items, f.Region)
	}
}

func TestReader_GetRegion_DerivesCounts(t *testing.T) {
	svc, f := buildInfraFixture(t)
	d, err := svc.GetRegion(context.Background(), f.Region, infra.AsOfOptions{})
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if d.ComputeCount != 1 || d.PersistenceCount != 1 || d.NetworkCount != 1 {
		t.Errorf("counts = %+v, want all 1", d)
	}
}

func TestReader_ListComputes_FilterByAccount(t *testing.T) {
	svc, f := buildInfraFixture(t)
	p, err := svc.ListComputes(context.Background(), infra.ListComputesQuery{
		AccountURN: f.Account, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListComputes: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].Compute.URN() != f.Compute {
		t.Errorf("computes=%+v want one with URN %s", p.Items, f.Compute)
	}
	// Filtro com Account inexistente → vazio.
	p2, _ := svc.ListComputes(context.Background(), infra.ListComputesQuery{
		AccountURN: node.URN("urn:ce:infra:account:aws:nope:nope"), Limit: 10,
	})
	if len(p2.Items) != 0 {
		t.Errorf("expected empty when filtering by absent account, got %d", len(p2.Items))
	}
}

func TestReader_ListComputes_FilterByRegion(t *testing.T) {
	svc, f := buildInfraFixture(t)
	p, _ := svc.ListComputes(context.Background(), infra.ListComputesQuery{
		RegionURN: f.Region, Limit: 10,
	})
	if len(p.Items) != 1 {
		t.Errorf("expected 1 compute in region, got %d", len(p.Items))
	}
}

func TestReader_GetCompute_NotFound(t *testing.T) {
	svc, _ := buildInfraFixture(t)
	_, err := svc.GetCompute(
		context.Background(),
		node.NewURN(node.ProviderAWS, "123", node.KindCompute, "i-nope"),
		infra.AsOfOptions{},
	)
	var typed *infra.ErrComputeNotFound
	if !errors.As(err, &typed) {
		t.Fatalf("expected *infra.ErrComputeNotFound, got %T", err)
	}
}

func TestReader_GetPersistence_WrongKind(t *testing.T) {
	svc, f := buildInfraFixture(t)
	_, err := svc.GetPersistence(context.Background(), f.Compute, infra.AsOfOptions{})
	var inv *infra.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *infra.ErrInvalidURN, got %T", err)
	}
}

func TestReader_ListPersistences(t *testing.T) {
	svc, f := buildInfraFixture(t)
	p, err := svc.ListPersistences(context.Background(), infra.ListPersistencesQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListPersistences: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].Persistence.URN() != f.Persist {
		t.Errorf("got %+v want %s", p.Items, f.Persist)
	}
}

func TestReader_ListNetworks(t *testing.T) {
	svc, f := buildInfraFixture(t)
	p, err := svc.ListNetworks(context.Background(), infra.ListNetworksQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].Network.URN() != f.Network {
		t.Errorf("got %+v want %s", p.Items, f.Network)
	}
}

func TestReader_Search_EmptyQ_TypedError(t *testing.T) {
	svc, _ := buildInfraFixture(t)
	_, err := svc.Search(context.Background(), infra.SearchQuery{Q: "  ", Limit: 10})
	var inv *infra.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *infra.ErrInvalidURN, got %T", err)
	}
}

func TestReader_Search_FiltersToInfraPlane(t *testing.T) {
	svc, _ := buildInfraFixture(t)
	res, err := svc.Search(context.Background(), infra.SearchQuery{Q: "us-east", Limit: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, n := range res.Items {
		switch n.Kind() {
		case node.KindProvider, node.KindAccount, node.KindRegion, node.KindZone,
			node.KindEnvironment, node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork:
			// ok
		default:
			t.Errorf("Search returned non-infra Kind=%s", n.Kind())
		}
	}
}

func TestReader_NewReader_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil nodes repo")
		}
	}()
	_ = service.NewReader(nil, memory.New().AsEdgeRepo())
}

// silence unused-import warnings for repository in case future tests need it.
var _ repository.NodeRepository = (*memory.Repo)(nil)
