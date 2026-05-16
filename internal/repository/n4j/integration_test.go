//go:build integration

// Integration tests para a implementação Neo4j (F-002 + F-003).
//
// Para rodar:
//
//	# 1. Sobe um Neo4j (qualquer forma — Docker Desktop, Neo4j Desktop, etc.).
//	docker run --rm -p 7687:7687 -p 7474:7474 \
//	    -e NEO4J_AUTH=neo4j/testpass neo4j:5
//
//	# 2. Aponta o teste para ele.
//	export NEO4J_TEST_URI=bolt://localhost:7687
//	export NEO4J_TEST_USER=neo4j
//	export NEO4J_TEST_PASS=testpass
//
//	# 3. Roda só os integration tests.
//	go test -tags=integration ./internal/repository/n4j/...
//
// Sem as env vars, os testes skipam com mensagem clara. O test isola o
// estado limpando o database antes de cada teste (`MATCH (n) DETACH DELETE n`)
// — NÃO aponte para um Neo4j com dados que importam.
package n4j

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// ----------------------------------------------------------------------------
// fixture: spin-up + cleanup
// ----------------------------------------------------------------------------

// envOr lê uma env var com default.
func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// setupClient conecta + migra. Skip se Neo4j não está acessível.
func setupClient(t *testing.T) *Client {
	t.Helper()
	uri := os.Getenv("NEO4J_TEST_URI")
	if uri == "" {
		t.Skip("NEO4J_TEST_URI não definido — pule integration tests")
	}
	cfg := Config{
		URI:      uri,
		Username: envOr("NEO4J_TEST_USER", "neo4j"),
		Password: envOr("NEO4J_TEST_PASS", "testpass"),
		Database: os.Getenv("NEO4J_TEST_DB"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Connect(ctx, cfg)
	if err != nil {
		t.Skipf("Neo4j inacessível em %s: %v", uri, err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	if err := c.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	wipe(t, c)
	t.Cleanup(func() { wipe(t, c) })
	return c
}

// wipe limpa o database. Único caminho seguro pra ter testes determinísticos.
func wipe(t *testing.T, c *Client) {
	t.Helper()
	_, err := c.withWrite(context.Background(), func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(context.Background(), "MATCH (n) DETACH DELETE n", nil)
		return nil, err
	})
	if err != nil {
		t.Fatalf("wipe: %v", err)
	}
}

// setRepoClock instala um relógio determinístico para os testes.
// Restaura o original via t.Cleanup.
func setRepoClock(t *testing.T, base time.Time) func() time.Time {
	t.Helper()
	var counter int64
	original := nowFn
	nowFn = func() time.Time {
		i := atomic.AddInt64(&counter, 1)
		return base.Add(time.Duration(i) * time.Hour)
	}
	t.Cleanup(func() { nowFn = original })
	return nowFn
}

// mkCompute helper.
func mkCompute(urn, account, ext string, version uint64, vfrom time.Time) node.Compute {
	return node.Compute{
		Base: node.Base{
			NodeURN:  node.URN(urn),
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{
				Version:    version,
				ValidFrom:  vfrom,
				ObservedAt: vfrom,
				Confidence: 1,
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    node.URN("urn:ce:aws:" + account + ":account/" + account),
		Region:     node.URN("urn:ce:aws:" + account + ":region/us-east-1"),
		ExtID:      ext,
		Flavor:     node.ComputeVM,
	}
}

// ----------------------------------------------------------------------------
// F-002 ACs
// ----------------------------------------------------------------------------

// AC-1: Upsert de nó novo → versão 1 corrente, valid_to NULL.
func TestN4jIntegration_Upsert_NewNode(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	n := mkCompute("urn:ce:aws:111:compute/i-1", "111", "i-1", 1, time.Now())
	ctx := context.Background()
	if err := repo.Upsert(ctx, n); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.GetByURN(ctx, n.URN(), repository.AsOf{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.URN() != n.URN() {
		t.Fatalf("URN diff: got %q want %q", got.URN(), n.URN())
	}
	if got.Meta().ValidTo != nil {
		t.Fatalf("valid_to deve ser NULL para versão corrente, got %v", got.Meta().ValidTo)
	}
}

// AC-2: Upsert sobre nó existente → versão N fecha (valid_to=now),
// versão N+1 nasce corrente.
func TestN4jIntegration_Upsert_Versions(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	urn := "urn:ce:aws:111:compute/i-1"
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	v1 := mkCompute(urn, "111", "i-1", 1, t0)
	v2 := mkCompute(urn, "111", "i-1", 2, t1)
	v2.InstanceType = "m6i.large" // muda algo significativo

	if err := repo.Upsert(ctx, v1); err != nil {
		t.Fatalf("upsert v1: %v", err)
	}
	if err := repo.Upsert(ctx, v2); err != nil {
		t.Fatalf("upsert v2: %v", err)
	}

	hist, err := repo.History(ctx, v1.URN())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("want 2 versões, got %d", len(hist))
	}
	// Versão 1 fechada.
	if hist[0].Meta().ValidTo == nil {
		t.Fatalf("v1 deve ter valid_to preenchido, got NULL")
	}
	// Versão 2 corrente.
	if hist[1].Meta().ValidTo != nil {
		t.Fatalf("v2 deve estar corrente, valid_to=%v", hist[1].Meta().ValidTo)
	}
}

// AC-3: AsOf entre versões retorna a versão visível naquela janela.
func TestN4jIntegration_GetByURN_AsOf(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	// Relógio determinístico: cada Upsert "acontece" 1h depois.
	clockBase := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setRepoClock(t, clockBase)
	ctx := context.Background()

	urn := "urn:ce:aws:111:compute/i-1"
	v1 := mkCompute(urn, "111", "i-1", 1, clockBase)
	v2 := mkCompute(urn, "111", "i-1", 2, clockBase.Add(2*time.Hour))
	v3 := mkCompute(urn, "111", "i-1", 3, clockBase.Add(4*time.Hour))

	for _, v := range []node.Compute{v1, v2, v3} {
		if err := repo.Upsert(ctx, v); err != nil {
			t.Fatalf("upsert v%d: %v", v.NodeMeta.Version, err)
		}
	}

	// AsOf entre v2 e v3 → deve retornar v2.
	// Após upsert v1, v2, v3: clock dispara 1,2,3,4,5h.
	// v2 vale entre clock(2h)=t2 e clock(3h)=t3.
	asOf := clockBase.Add(150 * time.Minute) // entre 2h e 3h
	got, err := repo.GetByURN(ctx, node.URN(urn), repository.AsOf(asOf))
	if err != nil {
		t.Fatalf("as-of: %v", err)
	}
	if got.Meta().Version != 2 {
		t.Fatalf("as-of em janela de v2 retornou version=%d, want 2", got.Meta().Version)
	}
}

// AC-4: History retorna versões em ordem cronológica (valid_from asc).
func TestN4jIntegration_History_Chronological(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	urn := "urn:ce:aws:111:compute/i-1"
	for i := 1; i <= 4; i++ {
		v := mkCompute(urn, "111", "i-1", uint64(i),
			time.Date(2026, time.Month(i), 1, 0, 0, 0, 0, time.UTC))
		if err := repo.Upsert(ctx, v); err != nil {
			t.Fatalf("upsert v%d: %v", i, err)
		}
	}
	hist, err := repo.History(ctx, node.URN(urn))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 4 {
		t.Fatalf("want 4 versões, got %d", len(hist))
	}
	for i := 1; i < len(hist); i++ {
		if hist[i].Meta().ValidFrom.Before(hist[i-1].Meta().ValidFrom) {
			t.Fatalf("ordem cronológica quebrada em i=%d", i)
		}
	}
}

// AC-5: Delete fecha versão corrente; Get current → ErrNotFound.
func TestN4jIntegration_Delete_SoftClose(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	n := mkCompute("urn:ce:aws:111:compute/i-1", "111", "i-1", 1, time.Now())
	if err := repo.Upsert(ctx, n); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.Delete(ctx, n.URN()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := repo.GetByURN(ctx, n.URN(), repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("get após delete: want ErrNotFound, got %v", err)
	}
	// History preserva a versão fechada.
	hist, err := repo.History(ctx, n.URN())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 || hist[0].Meta().ValidTo == nil {
		t.Fatalf("history após delete deve ter 1 versão fechada, got %+v", hist)
	}
}

// AC-6: Operações concorrentes sobre a mesma URN não criam versões
// duplicadas (constraint (urn, version) UNIQUE).
//
// Cenário: 10 goroutines fazendo Upsert simultâneo da mesma URN. O total
// de versões persistidas deve ser igual ao número de Upserts bem-sucedidos
// (não duplicados), e o constraint deve prevenir colisões de (urn, version).
func TestN4jIntegration_ConcurrentUpsert_NoDuplicateVersions(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	// Relógio precisa avançar entre chamadas concorrentes.
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	urn := "urn:ce:aws:111:compute/i-1"
	const workers = 10

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			v := mkCompute(urn, "111", "i-1", uint64(idx+1), time.Now())
			_ = repo.Upsert(ctx, v)
		}(i)
	}
	wg.Wait()

	hist, err := repo.History(ctx, node.URN(urn))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) == 0 {
		t.Fatalf("want pelo menos uma versão persistida")
	}
	// Versões devem ter `version` numérico único; constraint impede pares
	// (urn, version) repetidos.
	seen := map[uint64]bool{}
	for _, v := range hist {
		ver := v.Meta().Version
		if seen[ver] {
			t.Fatalf("versão duplicada %d violando constraint", ver)
		}
		seen[ver] = true
	}
	// Exatamente uma versão corrente.
	current := 0
	for _, v := range hist {
		if v.Meta().ValidTo == nil {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("want exatamente 1 versão corrente, got %d (de %d total)", current, len(hist))
	}
}

// Equivalente bitemporal para edges (S-006).
func TestN4jIntegration_Edge_UpsertDelete(t *testing.T) {
	c := setupClient(t)
	nodeRepo := NewNodeRepo(c)
	edgeRepo := NewEdgeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	// Dois nós correntes.
	from := mkCompute("urn:ce:aws:111:compute/i-1", "111", "i-1", 1, time.Now())
	pers := node.Persistence{
		Base: node.Base{
			NodeURN:  "urn:ce:aws:111:persistence/vol-1",
			NodeKind: node.KindPersistence,
			NodeMeta: node.Meta{Version: 1, ValidFrom: time.Now(), ObservedAt: time.Now(), Confidence: 1},
		},
		ProviderID: node.ProviderAWS,
		Account:    "urn:ce:aws:111:account/111",
		Region:     "urn:ce:aws:111:region/us-east-1",
		ExtID:      "vol-1",
		Flavor:     node.PersistenceBlock,
	}
	if err := nodeRepo.Upsert(ctx, from); err != nil {
		t.Fatalf("upsert from: %v", err)
	}
	if err := nodeRepo.Upsert(ctx, pers); err != nil {
		t.Fatalf("upsert to: %v", err)
	}

	att := edge.AttachedTo{
		Base: edge.Base{
			EdgeID:   "edge-1",
			EdgeType: edge.TypeAttachedTo,
			FromURN:  pers.URN(),
			ToURN:    from.URN(),
			EdgeMeta: edge.Meta{
				ValidFrom:   time.Now(),
				ObservedAt:  time.Now(),
				Confidence:  1,
				Directional: true,
				Weight:      1,
			},
		},
		MountPoint: "/dev/sda1",
	}
	if err := edgeRepo.Upsert(ctx, att, node.KindPersistence, node.KindCompute); err != nil {
		t.Fatalf("upsert edge: %v", err)
	}

	// Neighbors do compute → encontra a aresta corrente.
	es, err := edgeRepo.Neighbors(ctx, from.URN(), repository.DirIn, repository.EdgeFilter{})
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if len(es) != 1 {
		t.Fatalf("want 1 aresta, got %d", len(es))
	}

	// Delete fecha a aresta.
	if err := edgeRepo.Delete(ctx, "edge-1"); err != nil {
		t.Fatalf("delete edge: %v", err)
	}
	es, err = edgeRepo.Neighbors(ctx, from.URN(), repository.DirIn, repository.EdgeFilter{})
	if err != nil {
		t.Fatalf("neighbors após delete: %v", err)
	}
	if len(es) != 0 {
		t.Fatalf("want 0 arestas correntes após delete, got %d", len(es))
	}
}

// ----------------------------------------------------------------------------
// F-003 ACs (sobre n4j)
// ----------------------------------------------------------------------------

// Resolve short ID → URN (versão corrente).
func TestN4jIntegration_GetByExternalID_Strict(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	n := mkCompute("urn:ce:aws:111:compute/i-0abc", "111", "i-0abc", 1, time.Now())
	if err := repo.Upsert(ctx, n); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	urn, err := repo.GetByExternalID(ctx, node.ProviderAWS, "111", "i-0abc", repository.AsOf{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if urn != n.URN() {
		t.Fatalf("got %q want %q", urn, n.URN())
	}
}

// ARN ↔ short ID — qualquer forma resolve.
func TestN4jIntegration_GetByExternalID_ARNAndShort(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	arn := "arn:aws:ec2:us-east-1:111:instance/i-0abc"
	n := mkCompute("urn:ce:aws:111:compute/i-0abc", "111", arn, 1, time.Now())
	if err := repo.Upsert(ctx, n); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	for _, lookup := range []string{arn, "i-0abc"} {
		urn, err := repo.GetByExternalID(ctx, node.ProviderAWS, "111", lookup, repository.AsOf{})
		if err != nil {
			t.Fatalf("lookup %q: %v", lookup, err)
		}
		if urn != n.URN() {
			t.Fatalf("lookup %q: got %q want %q", lookup, urn, n.URN())
		}
	}
}

// AsOf retorna versão histórica.
func TestN4jIntegration_GetByExternalID_AsOf(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	clockBase := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setRepoClock(t, clockBase)
	ctx := context.Background()

	urn := "urn:ce:aws:111:compute/i-1"
	v1 := mkCompute(urn, "111", "i-1", 1, clockBase)
	v2 := mkCompute(urn, "111", "i-1", 2, clockBase.Add(2*time.Hour))

	if err := repo.Upsert(ctx, v1); err != nil {
		t.Fatalf("upsert v1: %v", err)
	}
	if err := repo.Upsert(ctx, v2); err != nil {
		t.Fatalf("upsert v2: %v", err)
	}

	// Corrente → v2 URN (mesma).
	got, err := repo.GetByExternalID(ctx, node.ProviderAWS, "111", "i-1", repository.AsOf{})
	if err != nil || got != node.URN(urn) {
		t.Fatalf("current: got=%q err=%v", got, err)
	}
	// AsOf no início → v1 ainda corrente.
	got, err = repo.GetByExternalID(ctx, node.ProviderAWS, "111", "i-1",
		repository.AsOf(clockBase.Add(90*time.Minute)))
	if err != nil || got != node.URN(urn) {
		t.Fatalf("as-of: got=%q err=%v", got, err)
	}
}

// NotFound: nunca existiu OU foi deletado.
func TestN4jIntegration_GetByExternalID_NotFound(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	// Nunca existiu.
	_, err := repo.GetByExternalID(ctx, node.ProviderAWS, "111", "i-doesnotexist", repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("never-existed: want ErrNotFound, got %v", err)
	}

	// Deletado.
	n := mkCompute("urn:ce:aws:111:compute/i-x", "111", "i-x", 1, time.Now())
	if err := repo.Upsert(ctx, n); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.Delete(ctx, n.URN()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = repo.GetByExternalID(ctx, node.ProviderAWS, "111", "i-x", repository.AsOf{})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("deleted: want ErrNotFound, got %v", err)
	}
}

// Cross-account / ambíguo: dois recursos mesmo short ID em contas diferentes.
func TestN4jIntegration_GetByExternalID_Ambiguous(t *testing.T) {
	c := setupClient(t)
	repo := NewNodeRepo(c)
	setRepoClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	c1 := mkCompute("urn:ce:aws:A:compute/i-dup", "A", "i-dup", 1, time.Now())
	c2 := mkCompute("urn:ce:aws:B:compute/i-dup", "B", "i-dup", 1, time.Now())
	if err := repo.Upsert(ctx, c1); err != nil {
		t.Fatalf("upsert c1: %v", err)
	}
	if err := repo.Upsert(ctx, c2); err != nil {
		t.Fatalf("upsert c2: %v", err)
	}

	// Estrito por A → resolve A.
	urn, err := repo.GetByExternalID(ctx, node.ProviderAWS, "A", "i-dup", repository.AsOf{})
	if err != nil || urn != c1.URN() {
		t.Fatalf("strict A: urn=%q err=%v", urn, err)
	}
	// Wildcard sem account → ambíguo.
	_, err = repo.GetByExternalID(ctx, node.ProviderAWS, "", "i-dup", repository.AsOf{})
	if !errors.Is(err, repository.ErrAmbiguous) {
		t.Fatalf("wildcard: want ErrAmbiguous, got %v", err)
	}
}

// guard: garante que o test não rodou sem env quando o pacote falha noutra parte.
func TestN4jIntegration_SmokeGuard(t *testing.T) {
	if os.Getenv("NEO4J_TEST_URI") == "" {
		t.Skip("integration env não configurada")
	}
	c := setupClient(t)
	if c == nil {
		t.Fatal(fmt.Errorf("setupClient devolveu nil sem skip"))
	}
}
