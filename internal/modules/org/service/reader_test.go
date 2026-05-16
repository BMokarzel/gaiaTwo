package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org"
	"costEngine/internal/modules/org/ingest/hris"
	"costEngine/internal/modules/org/service"
	"costEngine/internal/repository/memory"
)

// buildFixture parsa um CSV mínimo e aplica via Writer. Devolve um
// org.Service apontando para o mesmo repo memory, pronto para reads.
func buildFixture(t *testing.T, csv string) org.Service {
	t.Helper()
	rows, errs, err := hris.Parse(strings.NewReader(csv))
	if err != nil || len(errs) != 0 {
		t.Fatalf("Parse: %v / row errs %v", err, errs)
	}
	res, err := hris.Build(rows, hris.EmitOptions{
		Tenant:     "acme",
		ObservedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	repo := memory.New()
	w := service.Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	if _, err := w.Apply(context.Background(), res); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return service.New(repo, repo.AsEdgeRepo())
}

const csvSmall = `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,bob@x.com,2024-01-15,
bob@x.com,Bob,Manager,Platform,Checkout,,2020-03-01,
carol@x.com,Carol,Eng,Product,Growth,bob@x.com,2023-06-01,
`

func TestReader_ListTeams(t *testing.T) {
	svc := buildFixture(t, csvSmall)
	p, err := svc.ListTeams(context.Background(), org.ListTeamsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(p.Items) != 2 {
		t.Fatalf("len = %d, want 2 teams", len(p.Items))
	}
	if p.NextOffset != 0 {
		t.Errorf("NextOffset = %d, want 0 (single page)", p.NextOffset)
	}
	// Counts derivados pelo service.
	for _, ts := range p.Items {
		if ts.SquadCount == 0 {
			t.Errorf("team %s has 0 squads (counts not derived)", ts.Team.URN())
		}
	}
}

func TestReader_GetTeam_NotFound_TypedError(t *testing.T) {
	svc := buildFixture(t, csvSmall)
	_, err := svc.GetTeam(context.Background(), node.URN("urn:ce:org:team:acme:nope"), org.AsOfOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	var typed *org.ErrTeamNotFound
	if !errors.As(err, &typed) {
		t.Fatalf("expected *org.ErrTeamNotFound, got %T", err)
	}
	if typed.HTTPStatus() != 404 || typed.Code() != "org.team.not_found" {
		t.Errorf("status/code: %d / %q", typed.HTTPStatus(), typed.Code())
	}
}

func TestReader_GetTeam_WithSquads(t *testing.T) {
	svc := buildFixture(t, csvSmall)
	// Listar primeiro para descobrir URN canônica do team.
	teams, _ := svc.ListTeams(context.Background(), org.ListTeamsQuery{Limit: 10})
	var platform node.URN
	for _, ts := range teams.Items {
		if strings.Contains(string(ts.Team.URN()), "platform") {
			platform = ts.Team.URN()
			break
		}
	}
	if platform == "" {
		t.Fatal("could not locate Platform team URN")
	}
	d, err := svc.GetTeam(context.Background(), platform, org.AsOfOptions{})
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if len(d.Squads) != 1 || d.SquadCount != 1 {
		t.Errorf("Squads=%v SquadCount=%d", d.Squads, d.SquadCount)
	}
	if d.PersonCount != 2 { // Alice + Bob
		t.Errorf("PersonCount = %d, want 2", d.PersonCount)
	}
}

func TestReader_GetPerson_DerivesTeamURN(t *testing.T) {
	svc := buildFixture(t, csvSmall)
	// URN da Alice: hashed email — listar via squad members do
	// Checkout (mais simples).
	teams, _ := svc.ListTeams(context.Background(), org.ListTeamsQuery{Limit: 10})
	var platform node.URN
	for _, ts := range teams.Items {
		if strings.Contains(string(ts.Team.URN()), "platform") {
			platform = ts.Team.URN()
		}
	}
	det, _ := svc.GetTeam(context.Background(), platform, org.AsOfOptions{})
	if len(det.Squads) == 0 {
		t.Fatal("Platform team has no squads")
	}
	mp, err := svc.ListSquadMembers(context.Background(), det.Squads[0], org.ListMembersQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListSquadMembers: %v", err)
	}
	if len(mp.Items) == 0 {
		t.Fatal("Checkout squad has no members")
	}
	personURN := mp.Items[0].Person.URN()
	prof, err := svc.GetPerson(context.Background(), personURN, org.AsOfOptions{})
	if err != nil {
		t.Fatalf("GetPerson: %v", err)
	}
	if prof.TeamURN == "" {
		t.Errorf("TeamURN not derived for %s", personURN)
	}
}

func TestReader_ListReports_BobHasSubordinates(t *testing.T) {
	svc := buildFixture(t, csvSmall)
	// Bob é manager de Alice e Carol. Descobrir URN de Bob via List.
	// Mais robusto: iterar membros do squad Checkout (Bob, Alice).
	teams, _ := svc.ListTeams(context.Background(), org.ListTeamsQuery{Limit: 10})
	var platform node.URN
	for _, ts := range teams.Items {
		if strings.Contains(string(ts.Team.URN()), "platform") {
			platform = ts.Team.URN()
		}
	}
	det, _ := svc.GetTeam(context.Background(), platform, org.AsOfOptions{})
	mp, _ := svc.ListSquadMembers(context.Background(), det.Squads[0], org.ListMembersQuery{Limit: 10})
	var bob node.URN
	for _, m := range mp.Items {
		if m.Person.Role == "Manager" {
			bob = m.Person.URN()
		}
	}
	if bob == "" {
		t.Fatal("Bob URN not found")
	}
	tree, err := svc.ListReports(context.Background(), bob, org.ListReportsQuery{Depth: 2})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(tree.Reports) != 2 { // Alice + Carol
		t.Errorf("reports = %d, want 2", len(tree.Reports))
	}
	if tree.Truncated {
		t.Error("unexpected truncated=true")
	}
}

func TestReader_Search_EmptyQ_TypedError(t *testing.T) {
	svc := buildFixture(t, csvSmall)
	_, err := svc.Search(context.Background(), org.SearchQuery{Q: "  ", Limit: 10})
	if err == nil {
		t.Fatal("expected error")
	}
	var inv *org.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *org.ErrInvalidURN for empty q, got %T", err)
	}
}
