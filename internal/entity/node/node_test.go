package node

import (
	"testing"
	"time"
)

func TestMeta_IsCurrent(t *testing.T) {
	now := time.Now()
	cur := Meta{ValidFrom: now}
	if !cur.IsCurrent() {
		t.Fatal("expected current (ValidTo nil)")
	}

	end := now.Add(time.Hour)
	closed := Meta{ValidFrom: now, ValidTo: &end}
	if closed.IsCurrent() {
		t.Fatal("expected not current (ValidTo set)")
	}
}

func TestBase_NodeImpl(t *testing.T) {
	u := NewURN(ProviderAWS, "1", KindCompute, "i-1")
	b := Base{NodeURN: u, NodeKind: KindCompute, NodeMeta: Meta{Version: 7}}
	if b.URN() != u {
		t.Fatalf("URN() = %q, want %q", b.URN(), u)
	}
	if b.Kind() != KindCompute {
		t.Fatalf("Kind() = %q, want %q", b.Kind(), KindCompute)
	}
	if b.Meta().Version != 7 {
		t.Fatalf("Meta().Version = %d, want 7", b.Meta().Version)
	}
}

// Verifica em tempo de compilação que cada Resource concreto satisfaz a
// interface Resource. Falha de implementação vira erro de build.
var (
	_ Resource = Compute{}
	_ Resource = Persistence{}
	_ Resource = Messaging{}
	_ Resource = Network{}
)

// Provider/Account/Region/Environment NÃO devem implementar Resource —
// apenas Node. (Verificado em compile-time via Node.)
var (
	_ Node = Provider{}
	_ Node = Account{}
	_ Node = Region{}
	_ Node = Zone{}
	_ Node = Environment{}
)

func TestProvider_NodeMethods(t *testing.T) {
	p := Provider{
		Base: Base{NodeURN: "urn:ce:aws::provider/aws", NodeKind: KindProvider},
		ID:   ProviderAWS,
		Name: "AWS",
	}
	if p.URN() != "urn:ce:aws::provider/aws" || p.Kind() != KindProvider {
		t.Fatal("provider node methods wrong")
	}
}

func TestPersistence_ResourceMethods(t *testing.T) {
	p := Persistence{
		Base:       Base{NodeURN: "urn:ce:aws:1:persistence/db", NodeKind: KindPersistence},
		ProviderID: ProviderAWS,
		Account:    "urn:ce:aws:1:account/1",
		Region:     "urn:ce:aws:1:region/us-east-1",
		ExtID:      "db",
		Tags:       map[string]string{"team": "data"},
		SpecRaw:    map[string]any{"engine": "postgres"},
	}
	if p.Provider() != ProviderAWS || p.AccountURN() == "" || p.RegionURN() == "" {
		t.Fatal("persistence resource methods wrong")
	}
	if p.ExternalID() != "db" || p.NativeTags()["team"] != "data" || p.Spec()["engine"] != "postgres" {
		t.Fatal("persistence detail methods wrong")
	}
}

func TestMessaging_ResourceMethods(t *testing.T) {
	m := Messaging{
		Base:       Base{NodeURN: "urn:ce:aws:1:messaging/q", NodeKind: KindMessaging},
		ProviderID: ProviderAWS,
		Account:    "urn:ce:aws:1:account/1",
		Region:     "urn:ce:aws:1:region/us-east-1",
		ExtID:      "q",
		Tags:       map[string]string{"env": "dev"},
		SpecRaw:    map[string]any{"protocol": "sqs"},
	}
	if m.Provider() != ProviderAWS || m.ExternalID() != "q" {
		t.Fatal("messaging resource methods wrong")
	}
	if m.NativeTags()["env"] != "dev" || m.Spec()["protocol"] != "sqs" {
		t.Fatal("messaging detail methods wrong")
	}
	if m.AccountURN() == "" || m.RegionURN() == "" {
		t.Fatal("messaging scope methods wrong")
	}
}

func TestNetwork_ResourceMethods(t *testing.T) {
	n := Network{
		Base:       Base{NodeURN: "urn:ce:aws:1:network/vpc", NodeKind: KindNetwork},
		ProviderID: ProviderAWS,
		Account:    "urn:ce:aws:1:account/1",
		Region:     "urn:ce:aws:1:region/us-east-1",
		ExtID:      "vpc-1",
		Tags:       map[string]string{"layer": "core"},
		SpecRaw:    map[string]any{"cidr": "10.0.0.0/16"},
	}
	if n.Provider() != ProviderAWS || n.ExternalID() != "vpc-1" {
		t.Fatal("network resource methods wrong")
	}
	if n.NativeTags()["layer"] != "core" || n.Spec()["cidr"] != "10.0.0.0/16" {
		t.Fatal("network detail methods wrong")
	}
	if n.AccountURN() == "" || n.RegionURN() == "" {
		t.Fatal("network scope methods wrong")
	}
}

func TestCompute_ResourceMethods(t *testing.T) {
	c := Compute{
		Base:       Base{NodeURN: "urn:ce:aws:1:compute/i-1", NodeKind: KindCompute},
		ProviderID: ProviderAWS,
		Account:    "urn:ce:aws:1:account/1",
		Region:     "urn:ce:aws:1:region/us-east-1",
		ExtID:      "i-1",
		Tags:       map[string]string{"env": "prod"},
		SpecRaw:    map[string]any{"raw": true},
	}

	if c.Provider() != ProviderAWS {
		t.Errorf("Provider mismatch")
	}
	if c.ExternalID() != "i-1" {
		t.Errorf("ExternalID = %q, want i-1", c.ExternalID())
	}
	if c.NativeTags()["env"] != "prod" {
		t.Errorf("Tags lookup failed")
	}
	if c.Spec()["raw"] != true {
		t.Errorf("Spec lookup failed")
	}
}
