package edge

import (
	"fmt"
	"slices"

	"costEngine/internal/entity/node"
)

// Adjacency descreve quais kinds podem participar de cada tipo de aresta.
type Adjacency struct {
	From []node.Kind
	To   []node.Kind
}

// allowed é a matriz de adjacência tipada. Mantém o grafo consistente:
// o repositório deve rejeitar arestas que não satisfaçam essas regras.
var allowed = map[Type]Adjacency{
	TypeContains: {
		From: []node.Kind{
			// Infra plane
			node.KindProvider, node.KindAccount, node.KindRegion, node.KindZone,
			node.KindNetwork, node.KindCompute,
			// Code plane (F-018)
			node.KindService, node.KindModule,
			// Governance + org plane (E-008, F-024/F-025/F-029)
			node.KindCompany, node.KindBusinessArea,
			node.KindDomain, node.KindCapability, node.KindFeature,
			node.KindEpic, node.KindTeam,
		},
		To: []node.Kind{
			// Infra plane
			node.KindAccount, node.KindRegion, node.KindZone,
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
			// Code plane (F-018)
			node.KindModule, node.KindEndpoint, node.KindFunction,
			node.KindType, node.KindVariable, node.KindCall,
			// Governance + org plane
			node.KindBusinessArea, node.KindDomain, node.KindCapability,
			node.KindFeature, node.KindUserStory,
			node.KindTeam, node.KindPerson,
		},
	},
	TypeDeployedOn: {
		From: []node.Kind{node.KindCompute},
		To:   []node.Kind{node.KindCompute},
	},
	TypeAttachedTo: {
		From: []node.Kind{node.KindPersistence, node.KindNetwork},
		To:   []node.Kind{node.KindCompute},
	},
	TypeRoutes: {
		From: []node.Kind{node.KindNetwork},
		To:   []node.Kind{node.KindNetwork},
	},
	TypePeers: {
		From: []node.Kind{node.KindNetwork},
		To:   []node.Kind{node.KindNetwork},
	},
	TypeDependsOn: {
		From: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
			// Code plane: Module/Service ──DEPENDS_ON──▶ Framework
			node.KindModule, node.KindService,
		},
		To: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
			node.KindFramework,
		},
	},
	TypeCommunicatesWith: {
		From: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
		},
		To: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
		},
	},
	TypeReplaces: {
		From: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
		},
		To: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
		},
	},
	TypeDefinedIn: {
		From: []node.Kind{node.KindEndpoint, node.KindFunction},
		To:   []node.Kind{node.KindService},
	},
	TypeServiceRunsOn: {
		From: []node.Kind{node.KindService},
		To:   []node.Kind{node.KindCompute},
	},

	// Code plane — Call family (ADR-007).
	TypeInvokes: {
		From: []node.Kind{node.KindFunction},
		To:   []node.Kind{node.KindCall},
	},
	TypeTargets: {
		From: []node.Kind{node.KindCall},
		To: []node.Kind{
			node.KindFunction, node.KindEndpoint,
			node.KindPersistence, node.KindMessaging,
			node.KindSchema, node.KindService, node.KindType,
		},
	},
	TypeUses: {
		From: []node.Kind{node.KindCall, node.KindModule, node.KindService},
		To:   []node.Kind{node.KindFramework},
	},

	// Code plane — Type system (F-021).
	TypeImplements: {
		From: []node.Kind{node.KindType},
		To:   []node.Kind{node.KindType},
	},
	TypeExtends: {
		From: []node.Kind{node.KindType},
		To:   []node.Kind{node.KindType},
	},
	TypeAliases: {
		From: []node.Kind{node.KindType},
		To:   []node.Kind{node.KindType},
	},

	// Code plane — Schema (F-022).
	TypeSerializesAs: {
		From: []node.Kind{node.KindType},
		To:   []node.Kind{node.KindSchema},
	},
	TypeImports: {
		From: []node.Kind{node.KindSchema},
		To:   []node.Kind{node.KindSchema},
	},

	// Code plane — Framework/License/SecurityAdvisory (F-023).
	TypeLicensedUnder: {
		From: []node.Kind{node.KindFramework},
		To:   []node.Kind{node.KindLicense},
	},
	TypeAffectedBy: {
		From: []node.Kind{node.KindFramework},
		To:   []node.Kind{node.KindSecurityAdvisory},
	},
	TypePatchedIn: {
		From: []node.Kind{node.KindSecurityAdvisory},
		To:   []node.Kind{node.KindFramework},
	},

	// Governance plane (E-008).
	TypeDelivers: {
		From: []node.Kind{node.KindFeature},
		To:   []node.Kind{node.KindUserStory},
	},
	TypeAssignedTo: {
		From: []node.Kind{node.KindPerson},
		To:   []node.Kind{node.KindUserStory},
	},
	TypeServes: {
		From: []node.Kind{node.KindUserStory},
		To:   []node.Kind{node.KindPersona},
	},

	// Org plane (F-010 legado + F-027).
	TypeMemberOf: {
		From: []node.Kind{node.KindPerson},
		To:   []node.Kind{node.KindSquad},
	},
	TypePartOf: {
		From: []node.Kind{node.KindSquad},
		To:   []node.Kind{node.KindTeam},
	},
	TypeReportsTo: {
		From: []node.Kind{node.KindPerson},
		To:   []node.Kind{node.KindPerson},
	},
	TypeHasRole: {
		From: []node.Kind{node.KindPerson},
		To:   []node.Kind{node.KindRole},
	},
	TypeLedBy: {
		From: []node.Kind{node.KindTeam},
		To:   []node.Kind{node.KindPerson},
	},

	// Bridge gov→code (F-013).
	TypeRealizes: {
		From: []node.Kind{node.KindFeature},
		To:   []node.Kind{node.KindService},
	},

	// Bridge org→code — bifurcação ADR-010 / F-029.
	// Team owna unidades de intenção/contrato; Person owna unidades de execução.
	TypeOwns: {
		From: []node.Kind{node.KindTeam, node.KindPerson},
		To: []node.Kind{
			// Team-owned (intenção)
			node.KindService, node.KindModule, node.KindEndpoint,
			node.KindFeature, node.KindEpic,
			// Person-owned (execução)
			node.KindFunction, node.KindCall, node.KindType, node.KindVariable,
		},
	},
}

// Validate verifica se a aresta é estruturalmente válida segundo a matriz
// de adjacência. Não inspeciona dados do mundo real — só topologia de tipos.
func Validate(e Edge, fromKind, toKind node.Kind) error {
	rule, ok := allowed[e.Type()]
	if !ok {
		return fmt.Errorf("edge: unknown type %q", e.Type())
	}
	if !slices.Contains(rule.From, fromKind) {
		return fmt.Errorf("edge %s: invalid From kind %q", e.Type(), fromKind)
	}
	if !slices.Contains(rule.To, toKind) {
		return fmt.Errorf("edge %s: invalid To kind %q", e.Type(), toKind)
	}
	return nil
}

// ValidateOwnership aplica a regra de bifurcação ADR-010 / F-029.
// Retorna erro se Team owna unidade de execução fina OU Person owna
// unidade de intenção/contrato grossa. Usado como **lint** (warning
// no coletor) e não bloqueia ingestão — só Validate é hard constraint.
func ValidateOwnership(ownerKind, ownedKind node.Kind) error {
	teamOwnable := map[node.Kind]struct{}{
		node.KindService: {}, node.KindModule: {}, node.KindEndpoint: {},
		node.KindFeature: {}, node.KindEpic: {},
	}
	personOwnable := map[node.Kind]struct{}{
		node.KindFunction: {}, node.KindCall: {},
		node.KindType: {}, node.KindVariable: {},
	}
	switch ownerKind {
	case node.KindTeam:
		if _, ok := teamOwnable[ownedKind]; !ok {
			return fmt.Errorf("ownership lint: Team should not own %q (ADR-010 — Team é eixo de intenção)", ownedKind)
		}
	case node.KindPerson:
		if _, ok := personOwnable[ownedKind]; !ok {
			return fmt.Errorf("ownership lint: Person should not own %q (ADR-010 — Person é eixo de execução)", ownedKind)
		}
	default:
		return fmt.Errorf("ownership lint: owner kind %q is not eligible", ownerKind)
	}
	return nil
}
