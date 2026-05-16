package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "support.yaml", `
id: aws-support-by-team
version: 1
type: proportional_to_allocated
description: AWS Support rateado por team
match:
  service: AWSSupportBusinessSupport
  resource_id_empty: true
target:
  dimension: team
`)
	writeFile(t, dir, "egress.yaml", `
id: egress-us-east-1
version: 2
type: by_destination
match:
  service: AWSDataTransfer
target:
  region: us-east-1
`)
	writeFile(t, dir, "static.yaml", `
id: annual-contract-2026
version: 1
type: static_override
match:
  reasons: [no_resource_id]
target:
  shares:
    "urn:ce:org:acme:team/platform": 0.6
    "urn:ce:org:acme:team/product": 0.4
`)
	rs, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rs) != 3 {
		t.Fatalf("len=%d want 3", len(rs))
	}
	// Ordenação determinística por id.
	wantOrder := []string{"annual-contract-2026", "aws-support-by-team", "egress-us-east-1"}
	for i, w := range wantOrder {
		if rs[i].ID != w {
			t.Errorf("rs[%d].ID=%q want %q", i, rs[i].ID, w)
		}
	}
}

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	rs, err := Load(dir)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(rs) != 0 {
		t.Errorf("len=%d want 0", len(rs))
	}
}

func TestLoad_DirNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestLoad_DuplicateID(t *testing.T) {
	dir := t.TempDir()
	body := `
id: x
version: 1
type: by_destination
match: {}
target:
  region: us-east-1
`
	writeFile(t, dir, "a.yaml", body)
	writeFile(t, dir, "b.yaml", body)
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("want duplicate err, got %v", err)
	}
}

func TestLoad_UnknownField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.yaml", `
id: x
version: 1
type: by_destination
match: {}
target:
  region: us-east-1
extra_field: 42
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("want error for unknown field")
	}
}

func TestValidate_RuleErrors(t *testing.T) {
	cases := []struct {
		name string
		r    Rule
		want string
	}{
		{"bad-id-upper", Rule{ID: "BadID", Version: 1, Type: TypeByDestination, Target: Target{Region: "us"}}, "id"},
		{"bad-id-underscore", Rule{ID: "bad_id", Version: 1, Type: TypeByDestination, Target: Target{Region: "us"}}, "id"},
		{"zero-version", Rule{ID: "x", Version: 0, Type: TypeByDestination, Target: Target{Region: "us"}}, "version"},
		{"unknown-type", Rule{ID: "x", Version: 1, Type: "???"}, "unknown type"},
		{"prop-no-dim", Rule{ID: "x", Version: 1, Type: TypeProportional}, "dimension required"},
		{"prop-bad-dim", Rule{ID: "x", Version: 1, Type: TypeProportional, Target: Target{Dimension: "nope"}}, "not supported"},
		{"dest-no-region", Rule{ID: "x", Version: 1, Type: TypeByDestination}, "region required"},
		{"static-no-shares", Rule{ID: "x", Version: 1, Type: TypeStaticOverride}, "shares required"},
		{"static-frac-bad", Rule{ID: "x", Version: 1, Type: TypeStaticOverride, Target: Target{Shares: map[string]float64{"a": 1.5}}}, "out of"},
		{"static-sum-bad", Rule{ID: "x", Version: 1, Type: TypeStaticOverride, Target: Target{Shares: map[string]float64{"a": 0.3, "b": 0.3}}}, "sum to"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.r.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("want err containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestCoversPeriod(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	r := Rule{ID: "x", Version: 1, Type: TypeByDestination, Target: Target{Region: "us"}, ValidFrom: &t1, ValidTo: &t3}

	if !r.CoversPeriod(t2) {
		t.Errorf("t2 should be covered")
	}
	if r.CoversPeriod(t3) {
		t.Errorf("t3 (==valid_to) should be excluded")
	}
	tBefore := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	if r.CoversPeriod(tBefore) {
		t.Errorf("tBefore should be excluded")
	}
	// validFrom < validTo invariant
	rBad := Rule{ID: "x", Version: 1, Type: TypeByDestination, Target: Target{Region: "us"}, ValidFrom: &t3, ValidTo: &t1}
	if err := rBad.Validate(); err == nil {
		t.Error("want valid_from<valid_to error")
	}
}
