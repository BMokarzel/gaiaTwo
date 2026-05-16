package edge

import (
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

func TestDeterministicID_Stable(t *testing.T) {
	from := node.URN("urn:ce:aws:1:compute/i-1")
	to := node.URN("urn:ce:aws:1:persistence/db-1")
	vf := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	a := DeterministicID(from, TypeAttachedTo, to, vf)
	b := DeterministicID(from, TypeAttachedTo, to, vf)
	if a != b {
		t.Fatal("DeterministicID must be stable for same inputs")
	}
	if a == "" {
		t.Fatal("DeterministicID returned empty")
	}
}

func TestDeterministicID_DifferentTime(t *testing.T) {
	from := node.URN("urn:ce:aws:1:compute/i-1")
	to := node.URN("urn:ce:aws:1:persistence/db-1")

	a := DeterministicID(from, TypeAttachedTo, to, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := DeterministicID(from, TypeAttachedTo, to, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if a == b {
		t.Fatal("expected different IDs for different ValidFrom")
	}
}

func TestDeterministicID_DifferentType(t *testing.T) {
	from := node.URN("urn:ce:aws:1:compute/i-1")
	to := node.URN("urn:ce:aws:1:compute/i-2")
	vf := time.Now().UTC()

	a := DeterministicID(from, TypeCommunicatesWith, to, vf)
	b := DeterministicID(from, TypeDependsOn, to, vf)
	if a == b {
		t.Fatal("expected different IDs for different Type")
	}
}

func TestDeterministicID_Asymmetric(t *testing.T) {
	x := node.URN("urn:ce:aws:1:compute/i-1")
	y := node.URN("urn:ce:aws:1:compute/i-2")
	vf := time.Now().UTC()

	xy := DeterministicID(x, TypeCommunicatesWith, y, vf)
	yx := DeterministicID(y, TypeCommunicatesWith, x, vf)
	if xy == yx {
		t.Fatal("directional edges must produce different IDs for swapped endpoints")
	}
}

func TestMeta_IsCurrent(t *testing.T) {
	now := time.Now()
	if !(Meta{ValidFrom: now}).IsCurrent() {
		t.Fatal("expected current")
	}
	end := now.Add(time.Hour)
	if (Meta{ValidFrom: now, ValidTo: &end}).IsCurrent() {
		t.Fatal("expected closed")
	}
}

func makeEdge(t Type) Edge {
	return Base{
		EdgeID:   "id",
		EdgeType: t,
		FromURN:  "urn:ce:aws:1:x/1",
		ToURN:    "urn:ce:aws:1:x/2",
	}
}

func TestValidate_Allowed(t *testing.T) {
	cases := []struct {
		name             string
		typ              Type
		from, to         node.Kind
	}{
		{"contains: provider→account", TypeContains, node.KindProvider, node.KindAccount},
		{"contains: region→compute", TypeContains, node.KindRegion, node.KindCompute},
		{"contains: region→zone", TypeContains, node.KindRegion, node.KindZone},
		{"contains: zone→compute", TypeContains, node.KindZone, node.KindCompute},
		{"contains: account→region", TypeContains, node.KindAccount, node.KindRegion},
		{"deployed: compute→compute", TypeDeployedOn, node.KindCompute, node.KindCompute},
		{"attached: persistence→compute", TypeAttachedTo, node.KindPersistence, node.KindCompute},
		{"attached: network(eni)→compute", TypeAttachedTo, node.KindNetwork, node.KindCompute},
		{"routes: network→network", TypeRoutes, node.KindNetwork, node.KindNetwork},
		{"peers: network→network", TypePeers, node.KindNetwork, node.KindNetwork},
		{"depends: compute→persistence", TypeDependsOn, node.KindCompute, node.KindPersistence},
		{"comm: compute→messaging", TypeCommunicatesWith, node.KindCompute, node.KindMessaging},
		{"replaces: persistence→persistence", TypeReplaces, node.KindPersistence, node.KindPersistence},
		{"owns: person→service", TypeOwns, node.KindPerson, node.KindService},
		{"owns: team→service", TypeOwns, node.KindTeam, node.KindService},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(makeEdge(tc.typ), tc.from, tc.to); err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
		})
	}
}

func TestValidate_Rejects(t *testing.T) {
	cases := []struct {
		name             string
		typ              Type
		from, to         node.Kind
	}{
		{"deployed: persistence→compute (wrong from)", TypeDeployedOn, node.KindPersistence, node.KindCompute},
		{"attached: compute→persistence (wrong direction)", TypeAttachedTo, node.KindCompute, node.KindPersistence},
		{"routes: compute→network (wrong from)", TypeRoutes, node.KindCompute, node.KindNetwork},
		{"contains: compute→provider (wrong to)", TypeContains, node.KindCompute, node.KindProvider},
		{"owns: squad→service (wrong from)", TypeOwns, node.KindSquad, node.KindService},
		{"owns: person→compute (wrong to)", TypeOwns, node.KindPerson, node.KindCompute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(makeEdge(tc.typ), tc.from, tc.to); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestValidate_UnknownType(t *testing.T) {
	e := Base{EdgeType: Type("BOGUS")}
	if err := Validate(e, node.KindCompute, node.KindCompute); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestBase_Getters(t *testing.T) {
	vf := time.Now()
	b := Base{
		EdgeID:   "abc",
		EdgeType: TypeContains,
		FromURN:  "urn:ce:aws:1:account/1",
		ToURN:    "urn:ce:aws:1:region/us-east-1",
		EdgeMeta: Meta{ValidFrom: vf, Directional: true, Weight: 42},
	}
	if b.ID() != "abc" || b.Type() != TypeContains {
		t.Fatal("getters wrong")
	}
	if b.From() != "urn:ce:aws:1:account/1" || b.To() != "urn:ce:aws:1:region/us-east-1" {
		t.Fatal("endpoint getters wrong")
	}
	if b.Meta().Weight != 42 {
		t.Fatal("meta getter wrong")
	}
}

// Compile-time check: concrete edge structs must satisfy Edge.
var (
	_ Edge = Contains{}
	_ Edge = DeployedOn{}
	_ Edge = AttachedTo{}
	_ Edge = Routes{}
	_ Edge = Peers{}
	_ Edge = DependsOn{}
	_ Edge = CommunicatesWith{}
	_ Edge = Replaces{}
	_ Edge = Owns{}
)
