package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

// fakeDiscoverer retorna uma ScopeTopology pré-construída (ou erro).
type fakeDiscoverer struct {
	topo collector.ScopeTopology
	err  error
}

func (f fakeDiscoverer) DiscoverScope(ctx context.Context, scope collector.Scope) (collector.ScopeTopology, error) {
	return f.topo, f.err
}

// makeTopology constrói uma ScopeTopology mínima válida.
func makeTopology(t *testing.T) collector.ScopeTopology {
	t.Helper()
	const account = "123456789012"
	const region = "us-east-1"
	const zone = "us-east-1a"

	accountURN := node.NewURN(node.ProviderAWS, account, node.KindAccount, account)
	regionURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	zoneURN := node.NewURN(node.ProviderAWS, account, node.KindZone, zone)

	meta := node.Meta{Version: 1, Confidence: 1}

	return collector.ScopeTopology{
		Account: node.Account{
			Base:        node.Base{NodeURN: accountURN, NodeKind: node.KindAccount, NodeMeta: meta},
			ProviderID:  node.ProviderAWS,
			ExternalID:  account,
			DisplayName: account,
		},
		Region: node.Region{
			Base:       node.Base{NodeURN: regionURN, NodeKind: node.KindRegion, NodeMeta: meta},
			ProviderID: node.ProviderAWS,
			Code:       region,
		},
		Zones: []node.Zone{{
			Base:      node.Base{NodeURN: zoneURN, NodeKind: node.KindZone, NodeMeta: meta},
			RegionURN: regionURN,
			Code:      zone,
		}},
		Edges: []edge.Contains{
			{Base: edge.Base{
				EdgeID:   "edge-ar",
				EdgeType: edge.TypeContains,
				FromURN:  accountURN, ToURN: regionURN,
				EdgeMeta: edge.Meta{Directional: true, Confidence: 1},
			}},
			{Base: edge.Base{
				EdgeID:   "edge-rz",
				EdgeType: edge.TypeContains,
				FromURN:  regionURN, ToURN: zoneURN,
				EdgeMeta: edge.Meta{Directional: true, Confidence: 1},
			}},
		},
	}
}

func TestEnsureScopeAncestors_PersistsAllAndReports(t *testing.T) {
	repo := memory.New()
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})

	rep, err := svc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rep.Accounts != 1 || rep.Regions != 1 || rep.Zones != 1 || rep.Edges != 2 {
		t.Fatalf("report = %+v, want {Accounts:1 Regions:1 Zones:1 Edges:2}", rep)
	}

	// Confere persistência via List.
	got, err := repo.List(context.Background(), repository.NodeFilter{Kind: node.KindAccount})
	if err != nil || len(got) != 1 {
		t.Fatalf("expected 1 Account, got %d (err=%v)", len(got), err)
	}
	got, _ = repo.List(context.Background(), repository.NodeFilter{Kind: node.KindRegion})
	if len(got) != 1 {
		t.Fatalf("expected 1 Region, got %d", len(got))
	}
	got, _ = repo.List(context.Background(), repository.NodeFilter{Kind: node.KindZone})
	if len(got) != 1 {
		t.Fatalf("expected 1 Zone, got %d", len(got))
	}

	// Confere edges via Neighbors do Account.
	accountURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindAccount, "123456789012")
	edges, err := repo.AsEdgeRepo().Neighbors(context.Background(), accountURN, repository.DirOut, repository.EdgeFilter{})
	if err != nil || len(edges) != 1 {
		t.Fatalf("expected 1 outgoing edge from account, got %d (err=%v)", len(edges), err)
	}
	if edges[0].Type() != edge.TypeContains {
		t.Fatalf("edge type = %q", edges[0].Type())
	}
}

func TestEnsureScopeAncestors_DiscoverError(t *testing.T) {
	repo := memory.New()
	boom := errors.New("boom")
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{err: boom})

	_, err := svc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

// fakeCompute retorna um ComputeBatch pré-construído (ou erro).
type fakeCompute struct {
	batch collector.ComputeBatch
	err   error
}

func (f fakeCompute) DiscoverCompute(ctx context.Context, _ collector.Scope, _ collector.ScopeTopology) (collector.ComputeBatch, error) {
	return f.batch, f.err
}

// mkCompute monta um node.Compute pronto para testes.
func mkCompute(account, region, id, instType, state string, tags map[string]string, vf time.Time) node.Compute {
	accURN := node.NewURN(node.ProviderAWS, account, node.KindAccount, account)
	regURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	cURN := node.NewURN(node.ProviderAWS, account, node.KindCompute, id)
	return node.Compute{
		Base: node.Base{
			NodeURN:  cURN,
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vf, ObservedAt: vf, Confidence: 1, Source: node.Source{Collector: "aws", Method: node.MethodAPI}},
		},
		ProviderID:   node.ProviderAWS,
		Account:      accURN,
		Region:       regURN,
		ExtID:        id,
		Flavor:       node.ComputeVM,
		InstanceType: instType,
		State:        node.LifecycleState(state),
		Tags:         tags,
	}
}

func mkZoneEdge(account, region, zone, computeID string, vf time.Time) edge.Contains {
	zURN := node.NewURN(node.ProviderAWS, account, node.KindZone, zone)
	cURN := node.NewURN(node.ProviderAWS, account, node.KindCompute, computeID)
	_ = region
	return edge.Contains{Base: edge.Base{
		EdgeID:   edge.DeterministicID(zURN, edge.TypeContains, cURN, vf),
		EdgeType: edge.TypeContains,
		FromURN:  zURN,
		ToURN:    cURN,
		EdgeMeta: edge.Meta{ValidFrom: vf, ObservedAt: vf, Confidence: 1, Directional: true, Source: node.Source{Collector: "aws", Method: node.MethodAPI}},
	}}
}

func TestDiscoverCompute_NewInstancesCreated(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	// ScopeAncestors precisa existir antes.
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	c1 := mkCompute("123456789012", "us-east-1", "i-aaa", "m6i.large", "running", map[string]string{"env": "prod"}, vf)
	c2 := mkCompute("123456789012", "us-east-1", "i-bbb", "t3.micro", "running", nil, vf)
	batch := collector.ComputeBatch{
		Computes: []node.Compute{c1, c2},
		Edges: []edge.Contains{
			mkZoneEdge("123456789012", "us-east-1", "us-east-1a", "i-aaa", vf),
			mkZoneEdge("123456789012", "us-east-1", "us-east-1a", "i-bbb", vf),
		},
	}
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithComputeDiscoverer(fakeCompute{batch: batch})

	rep, err := svc.DiscoverCompute(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rep.New != 2 || rep.Updated != 0 || rep.Unchanged != 0 {
		t.Fatalf("report = %+v, want {New:2 Updated:0 Unchanged:0}", rep)
	}
	if rep.Edges != 2 {
		t.Fatalf("expected 2 edges, got %d", rep.Edges)
	}

	// Histórico de i-aaa: deve haver exatamente 1 versão (corrente).
	hist, err := repo.History(context.Background(), c1.URN())
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || !hist[0].Meta().IsCurrent() {
		t.Fatalf("expected 1 current version, got hist=%d", len(hist))
	}
}

func TestDiscoverCompute_UnchangedRerunOnlyTouches(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	c1 := mkCompute("123456789012", "us-east-1", "i-aaa", "m6i.large", "running", map[string]string{"env": "prod"}, vf)
	batch := collector.ComputeBatch{
		Computes: []node.Compute{c1},
		Edges:    []edge.Contains{mkZoneEdge("123456789012", "us-east-1", "us-east-1a", "i-aaa", vf)},
	}
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithComputeDiscoverer(fakeCompute{batch: batch})

	// 1ª execução: cria.
	if _, err := svc.DiscoverCompute(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t)); err != nil {
		t.Fatal(err)
	}
	// 2ª execução: mesmo conteúdo → Touch (não cria nova versão).
	rep, err := svc.DiscoverCompute(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != 0 || rep.Updated != 0 || rep.Unchanged != 1 {
		t.Fatalf("rerun report = %+v, want {Unchanged:1}", rep)
	}

	// Histórico continua com 1 versão (Touch não versiona).
	hist, err := repo.History(context.Background(), c1.URN())
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 version after unchanged rerun, got %d", len(hist))
	}
}

func TestDiscoverCompute_ChangedFieldBumpsVersion(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	c1 := mkCompute("123456789012", "us-east-1", "i-aaa", "m6i.large", "running", nil, vf)
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})

	// Run 1: m6i.large
	svc.WithComputeDiscoverer(fakeCompute{batch: collector.ComputeBatch{
		Computes: []node.Compute{c1},
		Edges:    []edge.Contains{mkZoneEdge("123456789012", "us-east-1", "us-east-1a", "i-aaa", vf)},
	}})
	if _, err := svc.DiscoverCompute(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t)); err != nil {
		t.Fatal(err)
	}

	// Run 2: mesma URN, instance_type mudou para m6i.xlarge → nova versão.
	c2 := mkCompute("123456789012", "us-east-1", "i-aaa", "m6i.xlarge", "running", nil, vf.Add(time.Hour))
	svc.WithComputeDiscoverer(fakeCompute{batch: collector.ComputeBatch{
		Computes: []node.Compute{c2},
		Edges:    []edge.Contains{mkZoneEdge("123456789012", "us-east-1", "us-east-1a", "i-aaa", vf.Add(time.Hour))},
	}})
	rep, err := svc.DiscoverCompute(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 1 || rep.New != 0 || rep.Unchanged != 0 {
		t.Fatalf("report = %+v, want {Updated:1}", rep)
	}

	cURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindCompute, "i-aaa")
	hist, err := repo.History(context.Background(), cURN)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 versions after change, got %d", len(hist))
	}
	// Apenas uma corrente.
	currentCount := 0
	for _, v := range hist {
		if v.Meta().IsCurrent() {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected 1 current version, got %d", currentCount)
	}
	// A corrente deve ter Version=2 e InstanceType=m6i.xlarge.
	cur, err := repo.GetByURN(context.Background(), cURN, repository.AsOf{})
	if err != nil {
		t.Fatal(err)
	}
	curComp, ok := cur.(node.Compute)
	if !ok {
		t.Fatalf("expected node.Compute, got %T", cur)
	}
	if curComp.Meta().Version != 2 {
		t.Fatalf("expected Version=2, got %d", curComp.Meta().Version)
	}
	if curComp.InstanceType != "m6i.xlarge" {
		t.Fatalf("expected m6i.xlarge, got %q", curComp.InstanceType)
	}
}

func TestDiscoverCompute_NoComputeDiscovererRegistered(t *testing.T) {
	repo := memory.New()
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := svc.DiscoverCompute(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, makeTopology(t)); err == nil {
		t.Fatal("expected error when no ComputeDiscoverer registered")
	}
}

// fakePersistence retorna um PersistenceBatch pré-construído (ou erro).
type fakePersistence struct {
	batch collector.PersistenceBatch
	err   error
}

func (f fakePersistence) DiscoverPersistence(ctx context.Context, _ collector.Scope, _ collector.ScopeTopology) (collector.PersistenceBatch, error) {
	return f.batch, f.err
}

func mkPersistence(account, region, id, engine string, size uint64, iops uint32, encrypted bool, tags map[string]string, vf time.Time) node.Persistence {
	accURN := node.NewURN(node.ProviderAWS, account, node.KindAccount, account)
	regURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	pURN := node.NewURN(node.ProviderAWS, account, node.KindPersistence, id)
	return node.Persistence{
		Base: node.Base{
			NodeURN:  pURN,
			NodeKind: node.KindPersistence,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vf, ObservedAt: vf, Confidence: 1, Source: node.Source{Collector: "aws", Method: node.MethodAPI}},
		},
		ProviderID: node.ProviderAWS,
		Account:    accURN,
		Region:     regURN,
		ExtID:      id,
		Flavor:     node.PersistenceBlock,
		Engine:     engine,
		SizeGiB:    size,
		IOPS:       iops,
		Encrypted:  encrypted,
		Tags:       tags,
	}
}

func mkPersistenceContainsEdge(account, zone, persID string, vf time.Time) edge.Contains {
	zURN := node.NewURN(node.ProviderAWS, account, node.KindZone, zone)
	pURN := node.NewURN(node.ProviderAWS, account, node.KindPersistence, persID)
	return edge.Contains{Base: edge.Base{
		EdgeID:   edge.DeterministicID(zURN, edge.TypeContains, pURN, vf),
		EdgeType: edge.TypeContains,
		FromURN:  zURN, ToURN: pURN,
		EdgeMeta: edge.Meta{ValidFrom: vf, ObservedAt: vf, Confidence: 1, Directional: true},
	}}
}

func TestDiscoverPersistence_NewVolumesCreated(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	p1 := mkPersistence("123456789012", "us-east-1", "vol-aaa", "gp3", 100, 3000, true, nil, vf)
	p2 := mkPersistence("123456789012", "us-east-1", "vol-bbb", "io2", 200, 10000, false, nil, vf)
	batch := collector.PersistenceBatch{
		Persistences: []node.Persistence{p1, p2},
		Edges: []edge.Edge{
			mkPersistenceContainsEdge("123456789012", "us-east-1a", "vol-aaa", vf),
			mkPersistenceContainsEdge("123456789012", "us-east-1a", "vol-bbb", vf),
		},
	}
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithPersistenceDiscoverer("ebs", fakePersistence{batch: batch})

	rep, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rep.New != 2 || rep.Updated != 0 || rep.Unchanged != 0 {
		t.Fatalf("report = %+v, want {New:2}", rep)
	}
	if rep.Edges != 2 {
		t.Fatalf("expected 2 edges, got %d", rep.Edges)
	}

	hist, err := repo.History(context.Background(), p1.URN())
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 version, got %d", len(hist))
	}
}

func TestDiscoverPersistence_UnchangedRerunOnlyTouches(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	p1 := mkPersistence("123456789012", "us-east-1", "vol-aaa", "gp3", 100, 3000, true, nil, vf)
	batch := collector.PersistenceBatch{
		Persistences: []node.Persistence{p1},
		Edges:        []edge.Edge{mkPersistenceContainsEdge("123456789012", "us-east-1a", "vol-aaa", vf)},
	}
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithPersistenceDiscoverer("ebs", fakePersistence{batch: batch})

	if _, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t)); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Unchanged != 1 || rep.New != 0 || rep.Updated != 0 {
		t.Fatalf("rerun report = %+v, want {Unchanged:1}", rep)
	}
	hist, _ := repo.History(context.Background(), p1.URN())
	if len(hist) != 1 {
		t.Fatalf("expected 1 version after unchanged rerun, got %d", len(hist))
	}
}

func TestDiscoverPersistence_SizeChangeBumpsVersion(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	p1 := mkPersistence("123456789012", "us-east-1", "vol-aaa", "gp3", 100, 3000, true, nil, vf)
	svc.WithPersistenceDiscoverer("ebs", fakePersistence{batch: collector.PersistenceBatch{
		Persistences: []node.Persistence{p1},
		Edges:        []edge.Edge{mkPersistenceContainsEdge("123456789012", "us-east-1a", "vol-aaa", vf)},
	}})
	if _, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t)); err != nil {
		t.Fatal(err)
	}

	// Tamanho cresce de 100→200 GiB.
	p2 := mkPersistence("123456789012", "us-east-1", "vol-aaa", "gp3", 200, 3000, true, nil, vf.Add(time.Hour))
	svc.persistences = nil // limpa para registrar nova versão do discoverer
	svc.WithPersistenceDiscoverer("ebs", fakePersistence{batch: collector.PersistenceBatch{
		Persistences: []node.Persistence{p2},
		Edges:        []edge.Edge{mkPersistenceContainsEdge("123456789012", "us-east-1a", "vol-aaa", vf.Add(time.Hour))},
	}})

	rep, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 1 || rep.New != 0 || rep.Unchanged != 0 {
		t.Fatalf("report = %+v, want {Updated:1}", rep)
	}

	pURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindPersistence, "vol-aaa")
	hist, _ := repo.History(context.Background(), pURN)
	if len(hist) != 2 {
		t.Fatalf("expected 2 versions after change, got %d", len(hist))
	}
	cur, _ := repo.GetByURN(context.Background(), pURN, repository.AsOf{})
	curP, ok := cur.(node.Persistence)
	if !ok {
		t.Fatalf("expected node.Persistence, got %T", cur)
	}
	if curP.Meta().Version != 2 || curP.SizeGiB != 200 {
		t.Fatalf("expected v=2 size=200, got v=%d size=%d", curP.Meta().Version, curP.SizeGiB)
	}
}

func TestDiscoverPersistence_NoDiscovererRegistered(t *testing.T) {
	repo := memory.New()
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, makeTopology(t)); err == nil {
		t.Fatal("expected error when no PersistenceDiscoverer registered")
	}
}

// S-008: erro parcial num discoverer não derruba os outros. EBS falha
// (AccessDenied simulado); S3 segue e contabiliza no relatório. O erro
// do EBS aparece em rep.Errors com o stage rotulado.
func TestDiscoverPersistence_PartialErrorContinues(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("AccessDenied")
	ok := mkPersistence("123456789012", "us-east-1", "bkt-x", "s3", 0, 0, false, nil, vf)
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithPersistenceDiscoverer("ebs", fakePersistence{err: boom}).
		WithPersistenceDiscoverer("s3", fakePersistence{batch: collector.PersistenceBatch{Persistences: []node.Persistence{ok}}})

	rep, err := svc.DiscoverPersistence(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatalf("unexpected fatal: %v", err)
	}
	if rep.New != 1 {
		t.Fatalf("expected S3 to contribute 1 new, got rep=%+v", rep)
	}
	if len(rep.Errors) != 1 {
		t.Fatalf("expected 1 captured error, got %d", len(rep.Errors))
	}
	if rep.Errors[0].Stage != "persistence/ebs" {
		t.Fatalf("expected stage persistence/ebs, got %q", rep.Errors[0].Stage)
	}
	if !errors.Is(rep.Errors[0].Err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", rep.Errors[0].Err)
	}
}

// fakeNetwork retorna um NetworkBatch pré-construído (ou erro).
type fakeNetwork struct {
	batch collector.NetworkBatch
	err   error
}

func (f fakeNetwork) DiscoverNetwork(ctx context.Context, _ collector.Scope, _ collector.ScopeTopology) (collector.NetworkBatch, error) {
	return f.batch, f.err
}

func mkVPC(account, region, id, cidr string, tags map[string]string, vf time.Time) node.Network {
	accURN := node.NewURN(node.ProviderAWS, account, node.KindAccount, account)
	regURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	nURN := node.NewURN(node.ProviderAWS, account, node.KindNetwork, id)
	return node.Network{
		Base: node.Base{
			NodeURN:  nURN,
			NodeKind: node.KindNetwork,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vf, ObservedAt: vf, Confidence: 1, Source: node.Source{Collector: "aws", Method: node.MethodAPI}},
		},
		ProviderID: node.ProviderAWS,
		Account:    accURN,
		Region:     regURN,
		ExtID:      id,
		Flavor:     node.NetworkVPC,
		CIDR:       cidr,
		Tags:       tags,
	}
}

func mkRegionContainsVPCEdge(account, region, vpcID string, vf time.Time) edge.Contains {
	regURN := node.NewURN(node.ProviderAWS, account, node.KindRegion, region)
	nURN := node.NewURN(node.ProviderAWS, account, node.KindNetwork, vpcID)
	return edge.Contains{Base: edge.Base{
		EdgeID:   edge.DeterministicID(regURN, edge.TypeContains, nURN, vf),
		EdgeType: edge.TypeContains,
		FromURN:  regURN, ToURN: nURN,
		EdgeMeta: edge.Meta{ValidFrom: vf, ObservedAt: vf, Confidence: 1, Directional: true},
	}}
}

func TestDiscoverNetwork_NewVPCsCreated(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	v1 := mkVPC("123456789012", "us-east-1", "vpc-aaa", "10.0.0.0/16", nil, vf)
	batch := collector.NetworkBatch{
		Networks: []node.Network{v1},
		Edges:    []edge.Edge{mkRegionContainsVPCEdge("123456789012", "us-east-1", "vpc-aaa", vf)},
	}
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithNetworkDiscoverer("vpc", fakeNetwork{batch: batch})

	rep, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rep.New != 1 || rep.Updated != 0 || rep.Unchanged != 0 {
		t.Fatalf("report = %+v, want {New:1}", rep)
	}
	if rep.Edges != 1 {
		t.Fatalf("expected 1 edge, got %d", rep.Edges)
	}
}

func TestDiscoverNetwork_UnchangedRerunOnlyTouches(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	v1 := mkVPC("123456789012", "us-east-1", "vpc-aaa", "10.0.0.0/16", nil, vf)
	batch := collector.NetworkBatch{
		Networks: []node.Network{v1},
		Edges:    []edge.Edge{mkRegionContainsVPCEdge("123456789012", "us-east-1", "vpc-aaa", vf)},
	}
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithNetworkDiscoverer("vpc", fakeNetwork{batch: batch})

	if _, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t)); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Unchanged != 1 || rep.New != 0 || rep.Updated != 0 {
		t.Fatalf("rerun report = %+v, want {Unchanged:1}", rep)
	}
	hist, _ := repo.History(context.Background(), v1.URN())
	if len(hist) != 1 {
		t.Fatalf("expected 1 version after unchanged rerun, got %d", len(hist))
	}
}

func TestDiscoverNetwork_CIDRChangeBumpsVersion(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	v1 := mkVPC("123456789012", "us-east-1", "vpc-aaa", "10.0.0.0/16", nil, vf)
	svc.WithNetworkDiscoverer("vpc", fakeNetwork{batch: collector.NetworkBatch{
		Networks: []node.Network{v1},
		Edges:    []edge.Edge{mkRegionContainsVPCEdge("123456789012", "us-east-1", "vpc-aaa", vf)},
	}})
	if _, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t)); err != nil {
		t.Fatal(err)
	}

	// CIDR muda → ContentHash diferente → bump.
	v2 := mkVPC("123456789012", "us-east-1", "vpc-aaa", "10.1.0.0/16", nil, vf.Add(time.Hour))
	svc.networks = nil
	svc.WithNetworkDiscoverer("vpc", fakeNetwork{batch: collector.NetworkBatch{
		Networks: []node.Network{v2},
		Edges:    []edge.Edge{mkRegionContainsVPCEdge("123456789012", "us-east-1", "vpc-aaa", vf.Add(time.Hour))},
	}})

	rep, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 1 || rep.New != 0 || rep.Unchanged != 0 {
		t.Fatalf("report = %+v, want {Updated:1}", rep)
	}
	nURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindNetwork, "vpc-aaa")
	hist, _ := repo.History(context.Background(), nURN)
	if len(hist) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(hist))
	}
	cur, _ := repo.GetByURN(context.Background(), nURN, repository.AsOf{})
	curN, ok := cur.(node.Network)
	if !ok {
		t.Fatalf("expected node.Network, got %T", cur)
	}
	if curN.Meta().Version != 2 || curN.CIDR != "10.1.0.0/16" {
		t.Fatalf("expected v=2 CIDR=10.1.0.0/16, got v=%d CIDR=%s", curN.Meta().Version, curN.CIDR)
	}
}

func TestDiscoverNetwork_NoDiscovererRegistered(t *testing.T) {
	repo := memory.New()
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "1", Region: "us-east-1"}, makeTopology(t)); err == nil {
		t.Fatal("expected error when no NetworkDiscoverer registered")
	}
}

// S-008: erro num NetworkDiscoverer não derruba os outros.
func TestDiscoverNetwork_PartialErrorContinues(t *testing.T) {
	repo := memory.New()
	vf := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	scopeSvc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})
	if _, err := scopeSvc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("Throttling")
	v1 := mkVPC("123456789012", "us-east-1", "vpc-ok", "10.0.0.0/16", nil, vf)
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)}).
		WithNetworkDiscoverer("lb", fakeNetwork{err: boom}).
		WithNetworkDiscoverer("vpc", fakeNetwork{batch: collector.NetworkBatch{Networks: []node.Network{v1}}})

	rep, err := svc.DiscoverNetwork(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}, makeTopology(t))
	if err != nil {
		t.Fatalf("unexpected fatal: %v", err)
	}
	if rep.New != 1 {
		t.Fatalf("expected VPC to contribute 1 new, got rep=%+v", rep)
	}
	if len(rep.Errors) != 1 || rep.Errors[0].Stage != "network/lb" {
		t.Fatalf("expected single error from network/lb, got %+v", rep.Errors)
	}
	if !errors.Is(rep.Errors[0].Err, boom) {
		t.Fatalf("expected wrapped underlying error, got %v", rep.Errors[0].Err)
	}
}

func TestEnsureScopeAncestors_Idempotent(t *testing.T) {
	repo := memory.New()
	svc := NewDiscoverService(repo, repo.AsEdgeRepo(), fakeDiscoverer{topo: makeTopology(t)})

	for i := range 3 {
		if _, err := svc.EnsureScopeAncestors(context.Background(), collector.Scope{Account: "123456789012", Region: "us-east-1"}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	// Após 3 execuções: histórico do Account deve ter 3 versões (cada
	// Upsert fecha a corrente e cria uma nova — semântica bitemporal).
	accountURN := node.NewURN(node.ProviderAWS, "123456789012", node.KindAccount, "123456789012")
	hist, err := repo.History(context.Background(), accountURN)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(hist))
	}
	// Apenas a última deve estar com IsCurrent.
	currentCount := 0
	for _, v := range hist {
		if v.Meta().IsCurrent() {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly 1 current version, got %d", currentCount)
	}
}
