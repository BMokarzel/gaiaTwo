package codeowners

import (
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

// TestBuild_F029Lint confirma que `Build` aplica o lint ADR-010
// (F-029) antes de emitir OWNS: Team→Service é aceito; Person→Service
// vai para Rejected com a razão preenchida.
func TestBuild_F029Lint(t *testing.T) {
	svc := node.NewServiceURN("payments", ".")
	team := node.NewTeamURN("acme", "payments")
	alice := node.NewPersonURN("acme", node.HashEmail("alice@a.com"))
	now := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)

	res := Build(svc, []ResolvedOwner{
		{Handle: "@org/payments", URN: team, Kind: node.KindTeam},
		{Handle: "@alice", URN: alice, Kind: node.KindPerson},
	}, nil, EmitOptions{Tenant: "acme", RunID: "r1", ObservedAt: now})

	if len(res.Owns) != 1 {
		t.Fatalf("Owns=%d want 1 (só team)", len(res.Owns))
	}
	if res.Owns[0].From() != team {
		t.Errorf("Owns[0].From=%s want team=%s", res.Owns[0].From(), team)
	}
	if len(res.Rejected) != 1 {
		t.Fatalf("Rejected=%d want 1 (alice)", len(res.Rejected))
	}
	rj := res.Rejected[0]
	if rj.URN != alice || rj.Kind != node.KindPerson {
		t.Errorf("Rejected[0]=%+v want alice/Person", rj)
	}
	if !strings.Contains(rj.Reason, "Person") {
		t.Errorf("Rejected.Reason=%q (esperado mencionar Person)", rj.Reason)
	}
}
