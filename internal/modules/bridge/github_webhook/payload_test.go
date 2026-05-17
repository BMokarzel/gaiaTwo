package github_webhook

import (
	"errors"
	"strings"
	"testing"
)

const samplePR = `{
	"action": "closed",
	"pull_request": {
		"html_url": "https://github.com/acme/api/pull/42",
		"merged": true,
		"merged_at": "2026-05-17T10:00:00Z",
		"labels": [
			{"name": "feature:urn:ce:gov:acme:feature/checkout-redesign"},
			{"name": "feature:urn:ce:gov:acme:feature/pricing-v2"},
			{"name": "type:bug"}
		],
		"head": {"repo": {"full_name": "acme/api"}}
	},
	"repository": {"full_name": "acme/api"},
	"files": [
		{"filename": "services/billing/charge.go"},
		{"filename": "services/billing/refund.go"},
		{"filename": "docs/README.md"}
	]
}`

func TestParse_Happy(t *testing.T) {
	p, err := Parse([]byte(samplePR))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(p.FeatureURNs), 2; got != want {
		t.Errorf("FeatureURNs len = %d, want %d", got, want)
	}
	if p.Repo != "acme/api" {
		t.Errorf("Repo = %q, want acme/api", p.Repo)
	}
	if len(p.Files) != 3 {
		t.Errorf("Files len = %d, want 3", len(p.Files))
	}
	if p.MergedAt.Year() != 2026 {
		t.Errorf("MergedAt year wrong: %v", p.MergedAt)
	}
}

func TestParse_Ignored(t *testing.T) {
	cases := map[string]string{
		"action != closed":   strings.Replace(samplePR, `"action": "closed"`, `"action": "opened"`, 1),
		"not merged":         strings.Replace(samplePR, `"merged": true`, `"merged": false`, 1),
		"no feature labels":  strings.Replace(samplePR, `"feature:urn:ce:gov:acme:feature/checkout-redesign"`, `"area:billing"`, 1),
	}
	// Case "no feature labels" still has second feature label — patch separately.
	cases["no feature labels at all"] = strings.NewReplacer(
		`"feature:urn:ce:gov:acme:feature/checkout-redesign"`, `"area:billing"`,
		`"feature:urn:ce:gov:acme:feature/pricing-v2"`, `"area:payments"`,
	).Replace(samplePR)
	delete(cases, "no feature labels")

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(body))
			if !errors.Is(err, ErrIgnored) {
				t.Fatalf("expected ErrIgnored, got %v", err)
			}
		})
	}
}

func TestParse_Malformed(t *testing.T) {
	cases := map[string]string{
		"invalid json":   `{not json`,
		"missing merged_at": `{"action":"closed","pull_request":{"merged":true,"labels":[{"name":"feature:urn:ce:gov:acme:feature/x"}]},"repository":{"full_name":"acme/api"}}`,
		"missing repo":   `{"action":"closed","pull_request":{"merged":true,"merged_at":"2026-05-17T10:00:00Z","labels":[{"name":"feature:urn:ce:gov:acme:feature/x"}]}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(body))
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("expected ErrMalformed, got %v", err)
			}
		})
	}
}
