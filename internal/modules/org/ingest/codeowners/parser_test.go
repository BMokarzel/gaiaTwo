package codeowners

import (
	"strings"
	"testing"
)

func TestParse_GlobalRule(t *testing.T) {
	in := `# top comment
*  @alice @org/payments
`
	rules, errs, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no row errors, got %v", errs)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if !r.IsGlobal() {
		t.Fatalf("expected global, got pattern=%q", r.Pattern)
	}
	if r.LineNum != 2 {
		t.Fatalf("line num=%d want 2", r.LineNum)
	}
	if got, want := r.Owners, []string{"@alice", "@org/payments"}; !equalSlices(got, want) {
		t.Fatalf("owners=%v want %v", got, want)
	}
}

func TestParse_BlankAndComments(t *testing.T) {
	in := `
# only comments here

   # indented comment
`
	rules, errs, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(rules) != 0 || len(errs) != 0 {
		t.Fatalf("rules=%v errs=%v", rules, errs)
	}
}

func TestParse_InlineComment(t *testing.T) {
	in := `*.go @alice  # owner of go files
`
	rules, errs, err := Parse(strings.NewReader(in))
	if err != nil || len(errs) != 0 {
		t.Fatalf("errs=%v fatal=%v", errs, err)
	}
	if len(rules) != 1 || rules[0].Pattern != "*.go" {
		t.Fatalf("rules=%+v", rules)
	}
	if got, want := rules[0].Owners, []string{"@alice"}; !equalSlices(got, want) {
		t.Fatalf("owners=%v want %v", got, want)
	}
}

func TestParse_MissingOwners(t *testing.T) {
	in := `*
`
	rules, errs, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 valid rules, got %v", rules)
	}
	if len(errs) != 1 || errs[0].Field != "owners" {
		t.Fatalf("errs=%v", errs)
	}
}

func TestParse_OwnerWithoutAt(t *testing.T) {
	in := `* alice @bob
`
	rules, errs, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("fatal: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules, got %v", rules)
	}
	if len(errs) != 1 || errs[0].Field != "owner" {
		t.Fatalf("errs=%v", errs)
	}
}

func TestParse_MultipleRulesAndGlobalLastWins(t *testing.T) {
	in := `* @alice
/docs/ @docs-team
* @bob @org/platform
`
	rules, errs, err := Parse(strings.NewReader(in))
	if err != nil || len(errs) != 0 {
		t.Fatalf("err=%v errs=%v", err, errs)
	}
	if len(rules) != 3 {
		t.Fatalf("want 3 rules, got %d", len(rules))
	}
	gr, ok := GlobalRule(rules)
	if !ok {
		t.Fatal("expected global rule")
	}
	if got, want := gr.Owners, []string{"@bob", "@org/platform"}; !equalSlices(got, want) {
		t.Fatalf("global owners=%v want %v (last-wins)", got, want)
	}
}

func TestParse_NoGlobal(t *testing.T) {
	in := `/api/ @api-team
`
	rules, _, _ := Parse(strings.NewReader(in))
	if _, ok := GlobalRule(rules); ok {
		t.Fatal("expected no global rule")
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
