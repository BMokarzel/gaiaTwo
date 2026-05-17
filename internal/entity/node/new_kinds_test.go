package node

import "testing"

// =============================================================================
// F-019 / F-020 — Call
// =============================================================================

func TestNewCallURN(t *testing.T) {
	caller := NewFunctionURN("ex", ".", "ex/pkg", "DoIt")
	urn := NewCallURN("ex", CallHttpCall, caller, 3)
	want := URN("urn:ce:code:ex:call/http!urn:ce:code:ex:function/.!ex/pkg!DoIt#3")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestCallContentHashStable(t *testing.T) {
	caller := NewFunctionURN("ex", ".", "ex/pkg", "Caller")
	c1 := Call{Kind_: CallDataAccess, CallerURN: caller, Ordinal: 1, TargetURL: "db://x", OperationKind: "select"}
	c2 := Call{Kind_: CallDataAccess, CallerURN: caller, Ordinal: 1, TargetURL: "db://x", OperationKind: "select"}
	if c1.ContentHash() != c2.ContentHash() {
		t.Error("ContentHash diverge para Calls idênticos")
	}
	c3 := c1
	c3.OperationKind = "insert"
	if c1.ContentHash() == c3.ContentHash() {
		t.Error("ContentHash não diverge para OperationKind distinto")
	}
}

// =============================================================================
// F-021 — Type, Variable
// =============================================================================

func TestNewTypeURN(t *testing.T) {
	urn := NewTypeURN("ex", ".", "ex/pkg", "User")
	want := URN("urn:ce:code:ex:type/.!ex/pkg!User")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestNewVariableURN(t *testing.T) {
	urn := NewVariableURN("ex", ".", "ex/pkg", "DefaultTimeout")
	want := URN("urn:ce:code:ex:variable/.!ex/pkg!DefaultTimeout")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestTypeContentHash_StableAcrossFieldOrder(t *testing.T) {
	t1 := Type{Symbol: "User", Kind_: TypeKindStruct,
		Fields: []FieldSlot{{Name: "id", TypeRef: "string"}, {Name: "name", TypeRef: "string"}}}
	t2 := Type{Symbol: "User", Kind_: TypeKindStruct,
		Fields: []FieldSlot{{Name: "id", TypeRef: "string"}, {Name: "name", TypeRef: "string"}}}
	if t1.ContentHash() != t2.ContentHash() {
		t.Error("ContentHash deve ser estável para Types idênticos")
	}
}

// =============================================================================
// F-022 — Schema
// =============================================================================

func TestNewSchemaURN(t *testing.T) {
	urn := NewSchemaURN("ex", ".", "user.v1.User")
	want := URN("urn:ce:code:ex:schema/.!user.v1.User")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
	// Global schema
	g := NewSchemaURN("", "", "common.v1.Money")
	wantG := URN("urn:ce:code:_global:schema/.!common.v1.Money")
	if g != wantG {
		t.Errorf("global got %q want %q", g, wantG)
	}
}

// =============================================================================
// F-023 — Framework, License, SecurityAdvisory
// =============================================================================

func TestNewFrameworkURN(t *testing.T) {
	urn := NewFrameworkURN("go", "github.com/go-chi/chi/v5")
	want := URN("urn:ce:code:_global:framework/go!github.com/go-chi/chi/v5")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestNewLicenseURN(t *testing.T) {
	urn := NewLicenseURN("MIT")
	want := URN("urn:ce:code:_global:license/MIT")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestNewSecurityAdvisoryURN(t *testing.T) {
	urn := NewSecurityAdvisoryURN("GHSA-xxxx-yyyy-zzzz")
	want := URN("urn:ce:code:_global:security_advisory/GHSA-xxxx-yyyy-zzzz")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

// =============================================================================
// F-024..F-026 — Governance (Company, BusinessArea, Domain, Capability,
//                              Feature, Epic, UserStory, Persona)
// =============================================================================

func TestGovernanceURNs(t *testing.T) {
	cases := []struct {
		got, want URN
	}{
		{NewCompanyURN("acme", "acme"), URN("urn:ce:gov:acme:company/acme")},
		{NewBusinessAreaURN("acme", "marketplace"), URN("urn:ce:gov:acme:business_area/marketplace")},
		{NewDomainURN("acme", "checkout"), URN("urn:ce:gov:acme:domain/checkout")},
		{NewCapabilityURN("acme", "payments"), URN("urn:ce:gov:acme:capability/payments")},
		{NewFeatureURN("acme", "pix"), URN("urn:ce:gov:acme:feature/pix")},
		{NewEpicURN("acme", "q3-2026-checkout"), URN("urn:ce:gov:acme:epic/q3-2026-checkout")},
		{NewUserStoryURN("acme", "US-123"), URN("urn:ce:gov:acme:user_story/US-123")},
		{NewPersonaURN("acme", "seller"), URN("urn:ce:gov:acme:persona/seller")},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q want %q", c.got, c.want)
		}
	}
}

func TestFeatureContentHash_StableAcrossEqualInputs(t *testing.T) {
	f1 := Feature{ShortID: "pix", Name: "Pagamento via Pix", Status: "active"}
	f2 := Feature{ShortID: "pix", Name: "Pagamento via Pix", Status: "active"}
	if f1.ContentHash() != f2.ContentHash() {
		t.Error("ContentHash deve ser estável para Features idênticas")
	}
}

// =============================================================================
// F-027 — Role
// =============================================================================

func TestNewRoleURN(t *testing.T) {
	urn := NewRoleURN("acme", TrackBackend, LevelSenior)
	want := URN("urn:ce:org:acme:role/backend-senior")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestRoleContentHashStable(t *testing.T) {
	r1 := Role{Track: TrackBackend, Level: LevelSenior, Name: "Backend Senior"}
	r2 := Role{Track: TrackBackend, Level: LevelSenior, Name: "Backend Senior"}
	if r1.ContentHash() != r2.ContentHash() {
		t.Error("ContentHash diverge")
	}
}
