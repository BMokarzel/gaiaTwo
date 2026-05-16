package n4j

import (
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

func TestNodeProps_Compute(t *testing.T) {
	c := node.Compute{
		Base: node.Base{
			NodeURN:  "urn:ce:aws:1:compute/i-1",
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{
				Version:    3,
				ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Confidence: 0.9,
				Labels:     []string{"env:prod"},
				Lineage:    node.Lineage{Rule: "discovered"},
			},
		},
		ProviderID: node.ProviderAWS,
		Account:    "urn:ce:aws:1:account/1",
		Region:     "urn:ce:aws:1:region/us-east-1",
		ExtID:      "i-1",
		Flavor:     node.ComputeVM,
		Tags:       map[string]string{"env": "prod"},
	}

	label, props, err := nodeProps(c)
	if err != nil {
		t.Fatalf("nodeProps error: %v", err)
	}
	if label != string(node.KindCompute) {
		t.Fatalf("label = %q, want %q", label, node.KindCompute)
	}
	if props["urn"] != string(c.URN()) {
		t.Fatal("urn missing/wrong")
	}
	if props["external_id"] != "i-1" {
		t.Fatal("external_id missing")
	}
	if props["provider"] != string(node.ProviderAWS) {
		t.Fatal("provider missing")
	}
	if _, ok := props["valid_to"]; !ok {
		t.Fatal("valid_to key should exist (may be nil)")
	}
	if props["valid_to"] != nil {
		t.Fatal("valid_to should be nil for current version")
	}
	if _, ok := props["tags_json"]; !ok {
		t.Fatal("tags_json missing")
	}
	if _, ok := props["type_specific_json"].(string); !ok {
		t.Fatal("type_specific_json missing or wrong type")
	}
}

func TestDecodeNode_Roundtrip(t *testing.T) {
	original := node.Compute{
		Base: node.Base{
			NodeURN:  "urn:ce:aws:1:compute/i-1",
			NodeKind: node.KindCompute,
			NodeMeta: node.Meta{
				Version:    7,
				ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Confidence: 1,
			},
		},
		ProviderID:   node.ProviderAWS,
		Account:      "urn:ce:aws:1:account/1",
		Region:       "urn:ce:aws:1:region/us-east-1",
		ExtID:        "i-1",
		Flavor:       node.ComputeVM,
		InstanceType: "m6i.large",
		VCPUs:        2,
		MemoryMiB:    8192,
	}

	_, props, err := nodeProps(original)
	if err != nil {
		t.Fatalf("nodeProps: %v", err)
	}
	decoded, err := decodeNode(props)
	if err != nil {
		t.Fatalf("decodeNode: %v", err)
	}
	got, ok := decoded.(node.Compute)
	if !ok {
		t.Fatalf("decoded type = %T, want Compute", decoded)
	}
	if got.URN() != original.URN() || got.InstanceType != original.InstanceType {
		t.Fatalf("roundtrip diff: got=%+v original=%+v", got, original)
	}
}

func TestDecodeNode_UnknownKind(t *testing.T) {
	_, err := decodeNode(map[string]any{
		"kind":               "bogus",
		"type_specific_json": "{}",
	})
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestEdgeProps_Roundtrip(t *testing.T) {
	original := edge.AttachedTo{
		Base: edge.Base{
			EdgeID:   "deadbeef",
			EdgeType: edge.TypeAttachedTo,
			FromURN:  "urn:ce:aws:1:persistence/db",
			ToURN:    "urn:ce:aws:1:compute/i-1",
			EdgeMeta: edge.Meta{
				ValidFrom:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Directional: true,
				Weight:      1,
			},
		},
		MountPoint: "/dev/sda1",
		ReadOnly:   false,
	}
	props := edgeProps(original)
	decoded, err := decodeEdge(props)
	if err != nil {
		t.Fatalf("decodeEdge: %v", err)
	}
	got, ok := decoded.(edge.AttachedTo)
	if !ok {
		t.Fatalf("decoded type = %T, want AttachedTo", decoded)
	}
	if got.ID() != original.ID() || got.MountPoint != original.MountPoint {
		t.Fatalf("roundtrip diff: got=%+v original=%+v", got, original)
	}
}

func TestBuildListQuery_Current(t *testing.T) {
	q, params := buildListQuery(repository.NodeFilter{Kind: node.KindCompute})
	if !contains(q, "n.valid_to IS NULL") {
		t.Fatalf("current query missing IS NULL: %s", q)
	}
	if !contains(q, "n.kind = $kind") {
		t.Fatalf("query missing kind filter: %s", q)
	}
	if params["kind"] != string(node.KindCompute) {
		t.Fatalf("params[kind] = %v, want %v", params["kind"], node.KindCompute)
	}
}

func TestBuildListQuery_AsOf(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q, params := buildListQuery(repository.NodeFilter{
		Kind: node.KindCompute,
		AsOf: repository.AsOf(at),
	})
	if !contains(q, "$at") {
		t.Fatalf("query missing $at: %s", q)
	}
	if params["at"] == nil {
		t.Fatalf("params[at] missing")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
