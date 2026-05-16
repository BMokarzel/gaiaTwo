package service_compute

import (
	"testing"

	"costEngine/internal/entity/node"
)

func mkCompute(tags map[string]string) node.Compute {
	return node.Compute{
		Base: node.Base{NodeURN: "urn:ce:aws:111:compute/i-1"},
		Tags: tags,
	}
}

func TestResolveByTag_ServiceURNWins(t *testing.T) {
	byName := map[string]node.URN{"billing": "urn:ce:code:repo:svc/billing"}
	c := mkCompute(map[string]string{
		"service.urn": "urn:ce:code:repo:svc/payments",
		"Service":     "billing", // ignorado: service.urn tem precedência
	})
	r := resolveByTag(c, byName)
	if r.ServiceURN != "urn:ce:code:repo:svc/payments" {
		t.Errorf("urn=%q want payments", r.ServiceURN)
	}
	if r.Source != SourceTag || r.Confidence != ConfidenceTag {
		t.Errorf("source=%q conf=%v want tag/1.0", r.Source, r.Confidence)
	}
}

func TestResolveByTag_ServiceNameLookup(t *testing.T) {
	byName := map[string]node.URN{"payments": "urn:ce:code:repo:svc/payments"}
	r := resolveByTag(mkCompute(map[string]string{"Service": "payments"}), byName)
	if r.ServiceURN != "urn:ce:code:repo:svc/payments" {
		t.Errorf("urn=%q", r.ServiceURN)
	}
	if r.Source != SourceTag {
		t.Errorf("source=%q want tag", r.Source)
	}
}

func TestResolveByTag_UnknownServiceName(t *testing.T) {
	byName := map[string]node.URN{"payments": "urn:ce:code:repo:svc/payments"}
	r := resolveByTag(mkCompute(map[string]string{"Service": "ghost"}), byName)
	if r.HasMatch() {
		t.Errorf("expected no match, got %+v", r)
	}
}

func TestResolveByTag_NoTags(t *testing.T) {
	if r := resolveByTag(mkCompute(nil), map[string]node.URN{}); r.HasMatch() {
		t.Errorf("expected empty, got %+v", r)
	}
}

func TestResolveByNameConvention_Matches(t *testing.T) {
	byName := map[string]node.URN{"payments": "urn:ce:code:repo:svc/payments"}
	cases := []struct {
		name, expectSvc string
	}{
		{"svc-payments-prod-01", "payments"},
		{"svc-payments-staging-eu-1", "payments"},
		{"SVC-PAYMENTS-PROD-01", "payments"}, // case insensitive
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := resolveByNameConvention(mkCompute(map[string]string{"Name": tc.name}), byName)
			if !r.HasMatch() {
				t.Fatalf("no match for %q", tc.name)
			}
			if r.Confidence != ConfidenceNameConvention {
				t.Errorf("conf=%v want %v", r.Confidence, ConfidenceNameConvention)
			}
			if r.Source != SourceNameConvention {
				t.Errorf("source=%q", r.Source)
			}
		})
	}
}

func TestResolveByNameConvention_RejectsBareSvcName(t *testing.T) {
	byName := map[string]node.URN{"payments": "urn:ce:code:repo:svc/payments"}
	// Sem sufixo após o nome → não bate (protege contra `svc-foo` puro).
	r := resolveByNameConvention(mkCompute(map[string]string{"Name": "svc-payments"}), byName)
	if r.HasMatch() {
		t.Errorf("expected no match for bare svc-payments, got %+v", r)
	}
}

func TestResolveByNameConvention_NoPrefix(t *testing.T) {
	byName := map[string]node.URN{"payments": "urn:ce:code:repo:svc/payments"}
	if r := resolveByNameConvention(mkCompute(map[string]string{"Name": "payments-prod-01"}), byName); r.HasMatch() {
		t.Errorf("expected no match without svc- prefix")
	}
}

func TestResolveByNameConvention_DeletedService(t *testing.T) {
	// Convention match mas o Service não está no mapa (deletado/inválido).
	r := resolveByNameConvention(mkCompute(map[string]string{"Name": "svc-ghost-prod-01"}), map[string]node.URN{})
	if r.HasMatch() {
		t.Errorf("expected no match for deleted service, got %+v", r)
	}
}

func TestResolveOne_TagBeatsNameConvention(t *testing.T) {
	byName := map[string]node.URN{
		"payments": "urn:ce:code:repo:svc/payments",
		"billing":  "urn:ce:code:repo:svc/billing",
	}
	c := mkCompute(map[string]string{
		"Service": "billing",
		"Name":    "svc-payments-prod-01",
	})
	r := resolveOne(c, byName)
	if r.ServiceURN != "urn:ce:code:repo:svc/billing" {
		t.Errorf("urn=%q want billing (tag wins)", r.ServiceURN)
	}
	if r.Source != SourceTag {
		t.Errorf("source=%q want tag", r.Source)
	}
}

func TestResolveOne_NoSignal(t *testing.T) {
	byName := map[string]node.URN{}
	if r := resolveOne(mkCompute(nil), byName); r.HasMatch() {
		t.Errorf("expected no match, got %+v", r)
	}
}
