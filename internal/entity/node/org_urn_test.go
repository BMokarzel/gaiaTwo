package node

import "testing"

func TestHashEmail_Stable(t *testing.T) {
	a := HashEmail("Alice@Example.COM")
	b := HashEmail("  alice@example.com  ")
	if a != b {
		t.Errorf("hash deve ignorar case/espaços: %q vs %q", a, b)
	}
	if len(a) != 32 {
		t.Errorf("hash len=%d want 32 (16 bytes hex)", len(a))
	}
}

func TestMaskEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"alice@example.com", "al***@example.com"},
		{"a@x.io", "***@x.io"},
		{"  Bob@Acme.IO  ", "bo***@acme.io"},
		{"no-at-symbol", "***"},
	}
	for _, c := range cases {
		if got := MaskEmail(c.in); got != c.want {
			t.Errorf("Mask(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestNewPersonURN(t *testing.T) {
	h := HashEmail("alice@example.com")
	got := NewPersonURN("acme", h)
	parts, err := ParseURN(got)
	if err != nil {
		t.Fatalf("ParseURN: %v", err)
	}
	if parts.Provider != ProviderOrg || parts.Account != "acme" ||
		parts.Kind != KindPerson || parts.ID != h {
		t.Errorf("parts=%+v", parts)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Platform Eng", "platform-eng"},
		{"Platform Eng (Core)", "platform-eng-core"},
		{"  --weird-- name  ", "weird-name"},
		{"Áéíóú", "áéíóú"},  // letters preservados, lowercase
		{"123 mix", "123-mix"},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestNewTeamSquadURN(t *testing.T) {
	tu := NewTeamURN("acme", "platform")
	if string(tu) != "urn:ce:org:acme:team/platform" {
		t.Errorf("team URN=%q", tu)
	}
	su := NewSquadURN("acme", "checkout")
	if string(su) != "urn:ce:org:acme:squad/checkout" {
		t.Errorf("squad URN=%q", su)
	}
}

func TestPersonContentHashStable(t *testing.T) {
	h := HashEmail("a@b.c")
	p1 := Person{EmailHash: h, Name: "A", Role: "Eng", SquadURN: "u", ManagerHash: "m"}
	p2 := Person{EmailHash: h, Name: "A", Role: "Eng", SquadURN: "u", ManagerHash: "m"}
	if p1.ContentHash() != p2.ContentHash() {
		t.Error("ContentHash deve ser estável")
	}
	p3 := Person{EmailHash: h, Name: "B"}
	if p1.ContentHash() == p3.ContentHash() {
		t.Error("ContentHash deve mudar com Name")
	}
	p4 := Person{EmailHash: h, Name: "A", Role: "Eng", SquadURN: "u", ManagerHash: "m", GithubHandle: "alice"}
	if p1.ContentHash() == p4.ContentHash() {
		t.Error("ContentHash deve mudar com GithubHandle (F-011)")
	}
}

func TestNormalizeGithubHandle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"@alice", "alice"},
		{"  @Alice  ", "alice"},
		{"alice", "alice"},
		{"@org/payments-team", ""}, // team handle não é Person — devolve vazio
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeGithubHandle(c.in); got != c.want {
			t.Errorf("NormalizeGithubHandle(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
