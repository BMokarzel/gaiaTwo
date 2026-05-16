package node

import "testing"

func TestNewServiceURN(t *testing.T) {
	tests := []struct {
		name           string
		repo, module   string
		want           URN
		wantParseKind  Kind
		wantParseID    string
	}{
		{"raiz", "costEngine", "", "urn:ce:code:costEngine:service/.", KindService, "."},
		{"sub", "costEngine", "cmd/cli", "urn:ce:code:costEngine:service/cmd/cli", KindService, "cmd/cli"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewServiceURN(tt.repo, tt.module)
			if got != tt.want {
				t.Errorf("URN=%q want %q", got, tt.want)
			}
			parts, err := ParseURN(got)
			if err != nil {
				t.Fatalf("ParseURN: %v", err)
			}
			if parts.Provider != ProviderCode || parts.Account != tt.repo || parts.Kind != tt.wantParseKind || parts.ID != tt.wantParseID {
				t.Errorf("parts=%+v", parts)
			}
		})
	}
}

func TestNewEndpointURN(t *testing.T) {
	urn := NewEndpointURN("costEngine", "internal/api", "get", "/v1/users/{id}")
	want := URN("urn:ce:code:costEngine:endpoint/internal/api!GET:/v1/users/{id}")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
	parts, err := ParseURN(urn)
	if err != nil {
		t.Fatalf("ParseURN: %v", err)
	}
	if parts.Kind != KindEndpoint || parts.ID != "internal/api!GET:/v1/users/{id}" {
		t.Errorf("parts=%+v", parts)
	}
}

func TestNewFunctionURN(t *testing.T) {
	urn := NewFunctionURN("costEngine", ".", "usecase", "CreateUser")
	want := URN("urn:ce:code:costEngine:function/.!usecase.CreateUser")
	if urn != want {
		t.Errorf("got %q want %q", urn, want)
	}
}

func TestServiceContentHashStable(t *testing.T) {
	s1 := Service{Repo: "r", ModulePath: ".", Language: "go", GoModule: "x", Tags: map[string]string{"a": "1", "b": "2"}}
	s2 := Service{Repo: "r", ModulePath: ".", Language: "go", GoModule: "x", Tags: map[string]string{"b": "2", "a": "1"}}
	if s1.ContentHash() != s2.ContentHash() {
		t.Error("hash deve ser estável independente da ordem de Tags")
	}
}
