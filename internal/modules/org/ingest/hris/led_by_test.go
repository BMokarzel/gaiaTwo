package hris

import (
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// TestBuild_F027LedBy cobre os sinais de liderança reconciliados em
// LED_BY:
//   - manager-de-fato (alguém reporta a ele) → líder do team dele
//   - título com keyword ("Manager", "Lead", "Head") → líder
//   - Role parsed com IsLeadership (Director/VP/C-level) → líder
//   - Pessoas terminadas (end_date preenchido) não viram líderes
func TestBuild_F027LedBy(t *testing.T) {
	rows := parseFixture(t, `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Backend Senior,Platform,Checkout,bob@x.com,2024-01-15,
bob@x.com,Bob,Engineering Manager,Platform,Checkout,,2024-01-15,
carol@x.com,Carol,Backend Senior,Payments,Cards,dave@x.com,2024-01-15,
dave@x.com,Dave,Tech Lead,Payments,Cards,,2024-01-15,
eve@x.com,Eve,VP Engineering,Platform,Infra,,2024-01-15,
frank@x.com,Frank,Backend Mid,Platform,Checkout,,2024-01-15,
mallory@x.com,Mallory,Director Platform,Platform,Infra,,2024-01-15,2024-06-01
`)
	res, err := Build(rows, EmitOptions{Tenant: "acme", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	platformURN := node.NewTeamURN("acme", "platform")
	paymentsURN := node.NewTeamURN("acme", "payments")
	bobURN := node.NewPersonURN("acme", node.HashEmail("bob@x.com"))
	daveURN := node.NewPersonURN("acme", node.HashEmail("dave@x.com"))
	eveURN := node.NewPersonURN("acme", node.HashEmail("eve@x.com"))
	frankURN := node.NewPersonURN("acme", node.HashEmail("frank@x.com"))
	malloryURN := node.NewPersonURN("acme", node.HashEmail("mallory@x.com"))

	// Esperado:
	//   Platform → Bob (manager + keyword)
	//   Payments → Dave (manager + keyword)
	//   Platform → Eve (Role IsLeadership=true via VP)
	// NÃO esperado:
	//   Platform → Frank (sem signal)
	//   Platform → Mallory (Director, mas terminada)
	want := map[string]bool{
		string(platformURN) + "|" + string(bobURN): true,
		string(paymentsURN) + "|" + string(daveURN): true,
		string(platformURN) + "|" + string(eveURN): true,
	}

	got := map[string]bool{}
	for _, ledBy := range res.LedBy {
		key := string(ledBy.From()) + "|" + string(ledBy.To())
		got[key] = true
	}

	for w := range want {
		if !got[w] {
			t.Errorf("missing LED_BY %q in %v", w, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("LED_BY count=%d want %d; got=%v", len(got), len(want), got)
	}

	// Confere que Frank (sem signal) e Mallory (terminada) ficaram de fora.
	for _, ledBy := range res.LedBy {
		if ledBy.To() == frankURN {
			t.Errorf("Frank não deveria virar líder: %+v", ledBy)
		}
		if ledBy.To() == malloryURN {
			t.Errorf("Mallory (terminada) não deveria virar líder: %+v", ledBy)
		}
	}

	// Validate estruturalmente.
	for _, ledBy := range res.LedBy {
		if err := edge.Validate(ledBy, node.KindTeam, node.KindPerson); err != nil {
			t.Errorf("Validate LedBy: %v", err)
		}
	}
}

// TestHasLeaderKeyword cobre o heurístico.
func TestHasLeaderKeyword(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Engineering Manager", true},
		{"Tech Lead", true},
		{"Head of Platform", true},
		{"VP Engineering", true},
		{"CTO", true},
		{"Backend Senior", false},
		{"Mid Frontend", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := hasLeaderKeyword(tc.in); got != tc.want {
				t.Errorf("hasLeaderKeyword(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}
