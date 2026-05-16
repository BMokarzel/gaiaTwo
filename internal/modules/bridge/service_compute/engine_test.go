package service_compute

import (
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

func mkC(extID string, tags map[string]string) node.Compute {
	return node.Compute{
		Base: node.Base{NodeURN: node.URN("urn:ce:aws:111:compute/" + extID)},
		Tags: tags,
	}
}

func TestApply_TagOpensEdge(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	in := Input{
		Account:        "urn:ce:aws:111:account/111",
		Computes:       []node.Compute{mkC("i-1", map[string]string{"Service": "payments"})},
		ServicesByName: map[string]node.URN{"payments": "urn:ce:code:repo:service/payments"},
		Now:            now,
	}
	out := Apply(in)
	if len(out.Opens) != 1 {
		t.Fatalf("opens=%d want 1", len(out.Opens))
	}
	o := out.Opens[0]
	if o.ServiceURN != "urn:ce:code:repo:service/payments" || o.ComputeURN != "urn:ce:aws:111:compute/i-1" {
		t.Errorf("bad open: %+v", o)
	}
	if o.Source != SourceTag || o.Confidence != ConfidenceTag {
		t.Errorf("source/conf wrong: %+v", o)
	}
	if !o.ValidFrom.Equal(now) {
		t.Errorf("valid_from=%v want %v", o.ValidFrom, now)
	}
	if len(out.Closes) != 0 || len(out.Orphans) != 0 {
		t.Errorf("unexpected closes/orphans: %+v", out)
	}
	if out.Stats.Opens != 1 || out.Stats.ResolvedTag != 1 {
		t.Errorf("stats: %+v", out.Stats)
	}
}

func TestApply_NameConventionFallback(t *testing.T) {
	in := Input{
		Computes:       []node.Compute{mkC("i-1", map[string]string{"Name": "svc-payments-prod-01"})},
		ServicesByName: map[string]node.URN{"payments": "urn:ce:code:repo:service/payments"},
		Now:            time.Unix(1700000000, 0),
	}
	out := Apply(in)
	if len(out.Opens) != 1 {
		t.Fatalf("opens=%d", len(out.Opens))
	}
	if out.Opens[0].Source != SourceNameConvention {
		t.Errorf("source=%q want name_convention", out.Opens[0].Source)
	}
	if out.Opens[0].Confidence != ConfidenceNameConvention {
		t.Errorf("conf=%v want %v", out.Opens[0].Confidence, ConfidenceNameConvention)
	}
}

func TestApply_NoSignalNoEdge(t *testing.T) {
	in := Input{
		Computes: []node.Compute{mkC("i-1", nil)},
		Now:      time.Now(),
	}
	out := Apply(in)
	if len(out.Opens) != 0 || len(out.Closes) != 0 {
		t.Errorf("unexpected open/close: %+v", out)
	}
	if len(out.Orphans) != 1 || out.Orphans[0] != "urn:ce:aws:111:compute/i-1" {
		t.Errorf("orphans: %+v", out.Orphans)
	}
}

func TestApply_TagChangeClosesOldOpensNew(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	c := mkC("i-1", map[string]string{"Service": "billing"})
	in := Input{
		Computes: []node.Compute{c},
		ServicesByName: map[string]node.URN{
			"payments": "urn:ce:code:repo:service/payments",
			"billing":  "urn:ce:code:repo:service/billing",
		},
		ExistingEdges: []ExistingEdge{{
			ComputeURN: c.URN(),
			ServiceURN: "urn:ce:code:repo:service/payments",
			Source:     SourceTag,
			Confidence: ConfidenceTag,
			ValidFrom:  now.Add(-24 * time.Hour),
		}},
		Now: now,
	}
	out := Apply(in)
	if len(out.Closes) != 1 || out.Closes[0].ServiceURN != "urn:ce:code:repo:service/payments" {
		t.Errorf("closes wrong: %+v", out.Closes)
	}
	if out.Closes[0].Reason != "owner_changed" {
		t.Errorf("reason=%q want owner_changed", out.Closes[0].Reason)
	}
	if !out.Closes[0].ValidTo.Equal(now) {
		t.Errorf("valid_to=%v", out.Closes[0].ValidTo)
	}
	if len(out.Opens) != 1 || out.Opens[0].ServiceURN != "urn:ce:code:repo:service/billing" {
		t.Errorf("opens wrong: %+v", out.Opens)
	}
}

func TestApply_OrphanedComputeClosesEdge(t *testing.T) {
	// Compute perdeu a tag mas o edge corrente ainda existe.
	now := time.Now()
	c := mkC("i-1", map[string]string{"Name": "untagged-box"})
	in := Input{
		Computes:       []node.Compute{c},
		ServicesByName: map[string]node.URN{},
		ExistingEdges: []ExistingEdge{{
			ComputeURN: c.URN(),
			ServiceURN: "urn:ce:code:repo:service/payments",
		}},
		Now: now,
	}
	out := Apply(in)
	if len(out.Closes) != 1 || out.Closes[0].Reason != "orphaned" {
		t.Errorf("closes wrong: %+v", out.Closes)
	}
	if len(out.Orphans) != 1 {
		t.Errorf("orphans=%d want 1", len(out.Orphans))
	}
	if len(out.Opens) != 0 {
		t.Errorf("unexpected open: %+v", out.Opens)
	}
}

func TestApply_MatchIsNoop(t *testing.T) {
	c := mkC("i-1", map[string]string{"Service": "payments"})
	in := Input{
		Computes:       []node.Compute{c},
		ServicesByName: map[string]node.URN{"payments": "urn:ce:code:repo:service/payments"},
		ExistingEdges: []ExistingEdge{{
			ComputeURN: c.URN(),
			ServiceURN: "urn:ce:code:repo:service/payments",
			Source:     SourceTag,
			Confidence: ConfidenceTag,
		}},
		Now: time.Now(),
	}
	out := Apply(in)
	if len(out.Opens) != 0 || len(out.Closes) != 0 {
		t.Errorf("expected noop, got %+v", out)
	}
}

func TestApply_AmbiguousServiceSkipsEverything(t *testing.T) {
	// Name aponta para um service que tem 2 candidatos: NÃO emite edge,
	// NÃO fecha edge corrente, REGISTRA em Ambiguous.
	c := mkC("i-1", map[string]string{"Service": "payments"})
	in := Input{
		Computes:       []node.Compute{c},
		ServicesByName: map[string]node.URN{}, // não entra no byName
		AmbiguousNames: map[string][]node.URN{
			"payments": {
				"urn:ce:code:repo-a:service/payments",
				"urn:ce:code:repo-b:service/payments",
			},
		},
		ExistingEdges: []ExistingEdge{{
			ComputeURN: c.URN(),
			ServiceURN: "urn:ce:code:repo-a:service/payments",
		}},
		Now: time.Now(),
	}
	out := Apply(in)
	if len(out.Ambiguous) != 1 {
		t.Fatalf("ambiguous=%d want 1", len(out.Ambiguous))
	}
	if out.Ambiguous[0].Name != "payments" || len(out.Ambiguous[0].Candidates) != 2 {
		t.Errorf("ambiguity: %+v", out.Ambiguous[0])
	}
	if len(out.Opens) != 0 || len(out.Closes) != 0 {
		t.Errorf("ambiguous should not move state: %+v", out)
	}
}

func TestApply_ServiceURNTagEscapesAmbiguity(t *testing.T) {
	c := mkC("i-1", map[string]string{
		"service.urn": "urn:ce:code:repo-a:service/payments",
		"Service":     "payments",
	})
	in := Input{
		Computes:       []node.Compute{c},
		ServicesByName: map[string]node.URN{}, // não importa
		AmbiguousNames: map[string][]node.URN{
			"payments": {
				"urn:ce:code:repo-a:service/payments",
				"urn:ce:code:repo-b:service/payments",
			},
		},
		Now: time.Now(),
	}
	out := Apply(in)
	if len(out.Ambiguous) != 0 {
		t.Errorf("URN tag must escape ambiguity: %+v", out.Ambiguous)
	}
	if len(out.Opens) != 1 || out.Opens[0].ServiceURN != "urn:ce:code:repo-a:service/payments" {
		t.Errorf("opens: %+v", out.Opens)
	}
}

func TestApply_Idempotent(t *testing.T) {
	now := time.Now()
	c := mkC("i-1", map[string]string{"Service": "payments"})
	in := Input{
		Computes:       []node.Compute{c},
		ServicesByName: map[string]node.URN{"payments": "urn:ce:code:repo:service/payments"},
		Now:            now,
	}
	o1 := Apply(in)
	// Caller aplicou o1 → segunda passada com ExistingEdges atualizados.
	in.ExistingEdges = []ExistingEdge{{
		ComputeURN: c.URN(),
		ServiceURN: o1.Opens[0].ServiceURN,
		Source:     SourceTag,
		Confidence: ConfidenceTag,
		ValidFrom:  now,
	}}
	o2 := Apply(in)
	if len(o2.Opens) != 0 || len(o2.Closes) != 0 {
		t.Errorf("second pass not idempotent: %+v", o2)
	}
}

func TestApply_DeterministicOrder(t *testing.T) {
	// Mesmo input em ordens diferentes ⇒ mesmo output.
	mk := func(order []string) Input {
		cs := make([]node.Compute, len(order))
		for i, id := range order {
			cs[i] = mkC(id, map[string]string{"Service": "payments"})
		}
		return Input{
			Computes:       cs,
			ServicesByName: map[string]node.URN{"payments": "urn:ce:code:repo:service/payments"},
			Now:            time.Unix(1, 0),
		}
	}
	a := Apply(mk([]string{"i-c", "i-a", "i-b"}))
	b := Apply(mk([]string{"i-a", "i-b", "i-c"}))
	if len(a.Opens) != len(b.Opens) {
		t.Fatalf("len mismatch %d vs %d", len(a.Opens), len(b.Opens))
	}
	for i := range a.Opens {
		if a.Opens[i].ComputeURN != b.Opens[i].ComputeURN {
			t.Errorf("order differs at %d: %q vs %q", i, a.Opens[i].ComputeURN, b.Opens[i].ComputeURN)
		}
	}
}
