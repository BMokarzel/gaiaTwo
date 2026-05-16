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
			node.KindProvider, node.KindAccount, node.KindRegion, node.KindZone,
			node.KindNetwork, node.KindCompute,
		},
		To: []node.Kind{
			node.KindAccount, node.KindRegion, node.KindZone,
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
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
		},
		To: []node.Kind{
			node.KindCompute, node.KindPersistence,
			node.KindMessaging, node.KindNetwork,
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
	TypeOwns: {
		From: []node.Kind{node.KindPerson, node.KindTeam},
		To:   []node.Kind{node.KindService},
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
