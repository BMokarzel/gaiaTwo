package node

import (
	"errors"
	"testing"
)

func TestNewURN(t *testing.T) {
	got := NewURN(ProviderAWS, "123456789012", KindCompute, "i-0a1b2c3d")
	want := URN("urn:ce:aws:123456789012:compute/i-0a1b2c3d")
	if got != want {
		t.Fatalf("NewURN = %q, want %q", got, want)
	}
}

func TestParseURN_Happy(t *testing.T) {
	cases := []struct {
		name string
		in   URN
		want URNParts
	}{
		{
			name: "simple",
			in:   "urn:ce:aws:123456789012:compute/i-0a1b2c3d",
			want: URNParts{Provider: ProviderAWS, Account: "123456789012", Kind: KindCompute, ID: "i-0a1b2c3d"},
		},
		{
			name: "id with slash (ARN-like)",
			in:   "urn:ce:aws:123:persistence/arn:aws:rds:us-east-1:123:db:payments",
			want: URNParts{Provider: ProviderAWS, Account: "123", Kind: KindPersistence, ID: "arn:aws:rds:us-east-1:123:db:payments"},
		},
		{
			name: "gcp project",
			in:   "urn:ce:gcp:my-project:network/vpc-prod",
			want: URNParts{Provider: ProviderGCP, Account: "my-project", Kind: KindNetwork, ID: "vpc-prod"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseURN(tc.in)
			if err != nil {
				t.Fatalf("ParseURN(%q) error = %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseURN(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseURN_Invalid(t *testing.T) {
	cases := []struct {
		name string
		in   URN
	}{
		{"empty", ""},
		{"wrong scheme", "uri:ce:aws:123:compute/i-1"},
		{"wrong namespace", "urn:other:aws:123:compute/i-1"},
		{"missing account", "urn:ce:aws::compute/i-1"},
		{"missing provider", "urn:ce::123:compute/i-1"},
		{"no kind/id slash", "urn:ce:aws:123:compute"},
		{"empty kind", "urn:ce:aws:123:/i-1"},
		{"empty id", "urn:ce:aws:123:compute/"},
		{"too few segments", "urn:ce:aws"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseURN(tc.in)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.in)
			}
			if !errors.Is(err, ErrInvalidURN) {
				t.Fatalf("expected ErrInvalidURN for %q, got %v", tc.in, err)
			}
		})
	}
}

func TestURN_RoundTrip(t *testing.T) {
	original := NewURN(ProviderAWS, "123", KindCompute, "i-abc")
	parts, err := ParseURN(original)
	if err != nil {
		t.Fatalf("ParseURN error: %v", err)
	}
	rebuilt := NewURN(parts.Provider, parts.Account, parts.Kind, parts.ID)
	if rebuilt != original {
		t.Fatalf("roundtrip mismatch: %q vs %q", rebuilt, original)
	}
}

func TestURN_IsValid(t *testing.T) {
	if !URN("urn:ce:aws:123:compute/i-1").IsValid() {
		t.Fatal("expected valid")
	}
	if URN("bogus").IsValid() {
		t.Fatal("expected invalid")
	}
}
