package hris

import (
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func TestParseRoleString(t *testing.T) {
	cases := []struct {
		in        string
		wantTrack node.RoleTrack
		wantLevel node.RoleLevel
		wantOK    bool
	}{
		{"Backend Senior", node.TrackBackend, node.LevelSenior, true},
		{"Senior Backend Engineer", node.TrackBackend, node.LevelSenior, true},
		{"Staff SRE", node.TrackSRE, node.LevelStaff, true},
		{"Frontend Mid", node.TrackFrontend, node.LevelMid, true},
		{"Sr Mobile", node.TrackMobile, node.LevelSenior, true},
		{"QA Junior", node.TrackQA, node.LevelJunior, true},
		{"Director Platform", node.TrackSRE, node.LevelDirector, true},
		{"Full Stack Senior", node.TrackFullstack, node.LevelSenior, true},
		// Sem level identificável → false.
		{"Manager", "", "", false},
		{"Eng", "", "", false},
		{"VP Engineering", "", node.LevelVP, false}, // sem track
		{"", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			tr, lv, ok := parseRoleString(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok=%v want %v (tr=%q lv=%q)", ok, tc.wantOK, tr, lv)
			}
			if ok && (tr != tc.wantTrack || lv != tc.wantLevel) {
				t.Errorf("(%q,%q) want (%q,%q)", tr, lv, tc.wantTrack, tc.wantLevel)
			}
		})
	}
}

// TestBuild_EmitsRoleAndHasRole confirma F-027: Build extrai Role
// canônico + emite HAS_ROLE quando o free-text é parseável.
func TestBuild_EmitsRoleAndHasRole(t *testing.T) {
	rows := parseFixture(t, `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Backend Senior,Platform,Checkout,,2024-01-15,
bob@x.com,Bob,Backend Senior,Platform,Checkout,,2024-01-15,
carol@x.com,Carol,Staff SRE,Platform,Infra,,2024-01-15,
dave@x.com,Dave,Manager,Platform,Checkout,,2024-01-15,
`)
	res, err := Build(rows, EmitOptions{Tenant: "acme", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// alice+bob compartilham `Backend Senior` → 1 Role; carol agrega `Staff SRE` → 2 Roles total.
	if len(res.Roles) != 2 {
		t.Fatalf("Roles=%d want 2 (Backend Senior + Staff SRE)", len(res.Roles))
	}
	// HAS_ROLE: alice, bob, carol = 3. Dave (Manager) sem parse → sem edge.
	if len(res.HasRole) != 3 {
		t.Fatalf("HasRole=%d want 3 (alice/bob/carol)", len(res.HasRole))
	}

	// Confere que dave NÃO tem HasRole.
	daveURN := node.NewPersonURN("acme", node.HashEmail("dave@x.com"))
	for _, hr := range res.HasRole {
		if hr.From() == daveURN {
			t.Errorf("dave (Manager) não deveria ter HAS_ROLE: %+v", hr)
		}
	}

	// Confere que alice+bob apontam para o MESMO Role URN.
	wantRole := node.NewRoleURN("acme", node.TrackBackend, node.LevelSenior)
	aliceURN := node.NewPersonURN("acme", node.HashEmail("alice@x.com"))
	bobURN := node.NewPersonURN("acme", node.HashEmail("bob@x.com"))
	var aliceRoleURN, bobRoleURN node.URN
	for _, hr := range res.HasRole {
		if hr.From() == aliceURN {
			aliceRoleURN = hr.To()
		}
		if hr.From() == bobURN {
			bobRoleURN = hr.To()
		}
	}
	if aliceRoleURN != wantRole || bobRoleURN != wantRole {
		t.Errorf("alice=%q bob=%q want both=%q", aliceRoleURN, bobRoleURN, wantRole)
	}

	// HAS_ROLE valida estruturalmente.
	for _, hr := range res.HasRole {
		if err := edge.Validate(hr, node.KindPerson, node.KindRole); err != nil {
			t.Errorf("Validate HasRole: %v", err)
		}
	}
}
