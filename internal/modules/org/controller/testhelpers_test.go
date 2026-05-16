package controller_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	orgctrl "costEngine/internal/modules/org/controller"
	orgservice "costEngine/internal/modules/org/service"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository/memory"
)

// fixture monta o handler completo (httpserver + org controller) sobre
// um memory.Repo. O Repo é exposto para que os testes adicionem nós/edges
// específicos do cenário sem dependências de v1.Server.
func fixture(t *testing.T) (http.Handler, *memory.Repo) {
	t.Helper()
	r := memory.New().WithClock(func() time.Time {
		return time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	})
	svc := orgservice.New(r, r.AsEdgeRepo())
	ctrl := orgctrl.New(svc)
	hs := httpserver.New(httpserver.Config{}, ctrl)
	return hs.Routes(), r
}

func doReq(t *testing.T, h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// decodeProblem decodifica um corpo application/problem+json (RFC 7807).
func decodeProblem(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

// orgFixture é o grafo determinístico usado pelos testes do controller:
// 2 teams; 1 deles tem 2 squads; 1 squad tem 2 pessoas.
type orgFixture struct {
	teamPayments node.URN
	teamData     node.URN
	squadCheck   node.URN
	squadGrowth  node.URN
	personAlice  node.URN
	personBob    node.URN
}

func newOrgFixture(t *testing.T, r *memory.Repo) orgFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	teamPayments := node.Team{
		Base: node.Base{
			NodeURN: node.NewTeamURN("acme", "payments"), NodeKind: node.KindTeam,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "payments", Name: "Payments",
	}
	teamData := node.Team{
		Base: node.Base{
			NodeURN: node.NewTeamURN("acme", "data"), NodeKind: node.KindTeam,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "data", Name: "Data",
	}
	squadCheck := node.Squad{
		Base: node.Base{
			NodeURN: node.NewSquadURN("acme", "checkout"), NodeKind: node.KindSquad,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "checkout", Name: "Checkout", TeamURN: teamPayments.URN(),
	}
	squadGrowth := node.Squad{
		Base: node.Base{
			NodeURN: node.NewSquadURN("acme", "growth"), NodeKind: node.KindSquad,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", Slug: "growth", Name: "Growth", TeamURN: teamPayments.URN(),
	}
	alice := node.Person{
		Base: node.Base{
			NodeURN: node.NewPersonURN("acme", node.HashEmail("alice@a.com")), NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("alice@a.com"),
		EmailHint: node.MaskEmail("alice@a.com"), Name: "Alice", SquadURN: squadCheck.URN(),
	}
	bob := node.Person{
		Base: node.Base{
			NodeURN: node.NewPersonURN("acme", node.HashEmail("bob@a.com")), NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: now, ObservedAt: now, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail("bob@a.com"),
		EmailHint: node.MaskEmail("bob@a.com"), Name: "Bob", SquadURN: squadCheck.URN(),
	}
	for _, n := range []node.Node{teamPayments, teamData, squadCheck, squadGrowth, alice, bob} {
		if err := r.Upsert(ctx, n); err != nil {
			t.Fatalf("upsert %s: %v", n.URN(), err)
		}
	}
	mkEdge := func(typ edge.Type, from, to node.URN, fromKind, toKind node.Kind) {
		t.Helper()
		var e edge.Edge
		b := edge.Base{
			EdgeID:   edge.DeterministicID(from, typ, to, now),
			EdgeType: typ,
			FromURN:  from, ToURN: to,
			EdgeMeta: edge.Meta{ValidFrom: now, ObservedAt: now, Directional: true, Confidence: 1},
		}
		switch typ {
		case edge.TypePartOf:
			e = edge.PartOf{Base: b}
		case edge.TypeMemberOf:
			e = edge.MemberOf{Base: b}
		}
		if err := r.AsEdgeRepo().Upsert(ctx, e, fromKind, toKind); err != nil {
			t.Fatalf("upsert edge: %v", err)
		}
	}
	mkEdge(edge.TypePartOf, squadCheck.URN(), teamPayments.URN(), node.KindSquad, node.KindTeam)
	mkEdge(edge.TypePartOf, squadGrowth.URN(), teamPayments.URN(), node.KindSquad, node.KindTeam)
	mkEdge(edge.TypeMemberOf, alice.URN(), squadCheck.URN(), node.KindPerson, node.KindSquad)
	mkEdge(edge.TypeMemberOf, bob.URN(), squadCheck.URN(), node.KindPerson, node.KindSquad)

	return orgFixture{
		teamPayments: teamPayments.URN(), teamData: teamData.URN(),
		squadCheck: squadCheck.URN(), squadGrowth: squadGrowth.URN(),
		personAlice: alice.URN(), personBob: bob.URN(),
	}
}

// addReportsTo adiciona edge ReportsTo (subordinate → manager).
func addReportsTo(t *testing.T, r *memory.Repo, subordinate, manager node.URN, vfrom time.Time) {
	t.Helper()
	e := edge.ReportsTo{Base: edge.Base{
		EdgeID:   edge.DeterministicID(subordinate, edge.TypeReportsTo, manager, vfrom),
		EdgeType: edge.TypeReportsTo,
		FromURN:  subordinate, ToURN: manager,
		EdgeMeta: edge.Meta{ValidFrom: vfrom, ObservedAt: vfrom, Directional: true, Confidence: 1},
	}}
	if err := r.AsEdgeRepo().Upsert(context.Background(), e, node.KindPerson, node.KindPerson); err != nil {
		t.Fatalf("upsert reports-to: %v", err)
	}
}

// mkPerson adiciona uma Person nova ao grafo e retorna a URN.
func mkPerson(t *testing.T, r *memory.Repo, email, name string, squad node.URN, vfrom time.Time) node.URN {
	t.Helper()
	p := node.Person{
		Base: node.Base{
			NodeURN:  node.NewPersonURN("acme", node.HashEmail(email)),
			NodeKind: node.KindPerson,
			NodeMeta: node.Meta{Version: 1, ValidFrom: vfrom, ObservedAt: vfrom, Confidence: 1},
		},
		Tenant: "acme", EmailHash: node.HashEmail(email),
		EmailHint: node.MaskEmail(email), Name: name, SquadURN: squad,
	}
	if err := r.Upsert(context.Background(), p); err != nil {
		t.Fatalf("upsert person: %v", err)
	}
	return p.URN()
}

// silence unused-imports when only a subset of helpers is touched.
var _ = decodeProblem
