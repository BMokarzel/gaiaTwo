package edge

import (
	"testing"

	"costEngine/internal/entity/node"
)

// Smoke test sobre a matriz de adjacência ampliada (E-007/E-008).
// Garante que toda combinação documentada em registry.go passa Validate.
func TestValidate_NewAdjacencies(t *testing.T) {
	cases := []struct {
		name string
		e    Edge
		from node.Kind
		to   node.Kind
	}{
		// F-018 — CONTAINS cross-plane
		{"Service→Module", Contains{Base: Base{EdgeType: TypeContains}}, node.KindService, node.KindModule},
		{"Module→Module", Contains{Base: Base{EdgeType: TypeContains}}, node.KindModule, node.KindModule},
		{"Module→Endpoint", Contains{Base: Base{EdgeType: TypeContains}}, node.KindModule, node.KindEndpoint},
		{"Module→Function", Contains{Base: Base{EdgeType: TypeContains}}, node.KindModule, node.KindFunction},
		{"Module→Type", Contains{Base: Base{EdgeType: TypeContains}}, node.KindModule, node.KindType},
		{"Module→Variable", Contains{Base: Base{EdgeType: TypeContains}}, node.KindModule, node.KindVariable},
		{"Module→Call", Contains{Base: Base{EdgeType: TypeContains}}, node.KindModule, node.KindCall},

		// F-019/F-020 — Call family
		{"Function→Call (INVOKES)", Invokes{Base: Base{EdgeType: TypeInvokes}}, node.KindFunction, node.KindCall},
		{"Call→Function (TARGETS)", Targets{Base: Base{EdgeType: TypeTargets}}, node.KindCall, node.KindFunction},
		{"Call→Endpoint (TARGETS)", Targets{Base: Base{EdgeType: TypeTargets}}, node.KindCall, node.KindEndpoint},
		{"Call→Persistence (TARGETS)", Targets{Base: Base{EdgeType: TypeTargets}}, node.KindCall, node.KindPersistence},
		{"Call→Messaging (TARGETS)", Targets{Base: Base{EdgeType: TypeTargets}}, node.KindCall, node.KindMessaging},
		{"Call→Schema (TARGETS)", Targets{Base: Base{EdgeType: TypeTargets}}, node.KindCall, node.KindSchema},
		{"Call→Framework (USES)", Uses{Base: Base{EdgeType: TypeUses}}, node.KindCall, node.KindFramework},
		{"Module→Framework (USES)", Uses{Base: Base{EdgeType: TypeUses}}, node.KindModule, node.KindFramework},
		{"Service→Framework (USES)", Uses{Base: Base{EdgeType: TypeUses}}, node.KindService, node.KindFramework},

		// F-021 — Type system
		{"Type→Type (IMPLEMENTS)", Implements{Base: Base{EdgeType: TypeImplements}}, node.KindType, node.KindType},
		{"Type→Type (EXTENDS)", Extends{Base: Base{EdgeType: TypeExtends}}, node.KindType, node.KindType},
		{"Type→Type (ALIASES)", Aliases{Base: Base{EdgeType: TypeAliases}}, node.KindType, node.KindType},

		// F-022 — Schema
		{"Type→Schema (SERIALIZES_AS)", SerializesAs{Base: Base{EdgeType: TypeSerializesAs}}, node.KindType, node.KindSchema},
		{"Schema→Schema (IMPORTS)", Imports{Base: Base{EdgeType: TypeImports}}, node.KindSchema, node.KindSchema},

		// F-023 — Framework/License/SecurityAdvisory
		{"Framework→License (LICENSED_UNDER)", LicensedUnder{Base: Base{EdgeType: TypeLicensedUnder}}, node.KindFramework, node.KindLicense},
		{"Framework→Advisory (AFFECTED_BY)", AffectedBy{Base: Base{EdgeType: TypeAffectedBy}}, node.KindFramework, node.KindSecurityAdvisory},
		{"Advisory→Framework (PATCHED_IN)", PatchedIn{Base: Base{EdgeType: TypePatchedIn}}, node.KindSecurityAdvisory, node.KindFramework},

		// F-024 — Governance (CONTAINS)
		{"Company→BusinessArea (CONTAINS)", Contains{Base: Base{EdgeType: TypeContains}}, node.KindCompany, node.KindBusinessArea},
		{"BusinessArea→Domain (CONTAINS)", Contains{Base: Base{EdgeType: TypeContains}}, node.KindBusinessArea, node.KindDomain},
		{"Domain→Capability (CONTAINS)", Contains{Base: Base{EdgeType: TypeContains}}, node.KindDomain, node.KindCapability},
		{"Capability→Feature (CONTAINS)", Contains{Base: Base{EdgeType: TypeContains}}, node.KindCapability, node.KindFeature},
		{"Feature→Feature (CONTAINS)", Contains{Base: Base{EdgeType: TypeContains}}, node.KindFeature, node.KindFeature},

		// F-025 — Epic/UserStory
		{"Epic→UserStory (CONTAINS)", Contains{Base: Base{EdgeType: TypeContains}}, node.KindEpic, node.KindUserStory},
		{"Feature→UserStory (DELIVERS)", Delivers{Base: Base{EdgeType: TypeDelivers}}, node.KindFeature, node.KindUserStory},
		{"Person→UserStory (ASSIGNED_TO)", AssignedTo{Base: Base{EdgeType: TypeAssignedTo}}, node.KindPerson, node.KindUserStory},

		// F-026 — Persona
		{"UserStory→Persona (SERVES)", Serves{Base: Base{EdgeType: TypeServes}}, node.KindUserStory, node.KindPersona},

		// F-027 — Role
		{"Person→Role (HAS_ROLE)", HasRole{Base: Base{EdgeType: TypeHasRole}}, node.KindPerson, node.KindRole},
		{"Team→Person (LED_BY)", LedBy{Base: Base{EdgeType: TypeLedBy}}, node.KindTeam, node.KindPerson},

		// F-013 — Bridge gov→code
		{"Feature→Service (REALIZES)", Realizes{Base: Base{EdgeType: TypeRealizes}}, node.KindFeature, node.KindService},

		// F-029 — Ownership bifurcation (hard rule via Validate)
		{"Team→Service (OWNS)", Owns{Base: Base{EdgeType: TypeOwns}}, node.KindTeam, node.KindService},
		{"Team→Feature (OWNS)", Owns{Base: Base{EdgeType: TypeOwns}}, node.KindTeam, node.KindFeature},
		{"Person→Function (OWNS)", Owns{Base: Base{EdgeType: TypeOwns}}, node.KindPerson, node.KindFunction},
		{"Person→Call (OWNS)", Owns{Base: Base{EdgeType: TypeOwns}}, node.KindPerson, node.KindCall},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.e, tc.from, tc.to); err != nil {
				t.Errorf("Validate(%s→%s): %v", tc.from, tc.to, err)
			}
		})
	}
}

// F-029 — bifurcation: ValidateOwnership é o lint (soft) que separa
// granularidade. Person não deve owner Service; Team não deve owner Function.
func TestValidateOwnership_Bifurcation(t *testing.T) {
	ok := []struct {
		owner, owned node.Kind
	}{
		{node.KindTeam, node.KindService},
		{node.KindTeam, node.KindModule},
		{node.KindTeam, node.KindEndpoint},
		{node.KindTeam, node.KindFeature},
		{node.KindTeam, node.KindEpic},
		{node.KindPerson, node.KindFunction},
		{node.KindPerson, node.KindCall},
		{node.KindPerson, node.KindType},
		{node.KindPerson, node.KindVariable},
	}
	for _, c := range ok {
		if err := ValidateOwnership(c.owner, c.owned); err != nil {
			t.Errorf("OK %s→%s falhou: %v", c.owner, c.owned, err)
		}
	}
	bad := []struct {
		owner, owned node.Kind
	}{
		{node.KindTeam, node.KindFunction},  // grão fino → Team é inadequado
		{node.KindTeam, node.KindCall},
		{node.KindPerson, node.KindService}, // contrato grosso → Person é inadequado
		{node.KindPerson, node.KindModule},
	}
	for _, c := range bad {
		if err := ValidateOwnership(c.owner, c.owned); err == nil {
			t.Errorf("esperado erro em %s→%s", c.owner, c.owned)
		}
	}
}
