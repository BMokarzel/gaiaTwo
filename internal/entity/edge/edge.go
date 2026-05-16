// Package edge define o contrato raiz e os tipos transversais para toda
// aresta do grafo de inteligência arquitetural.
//
// Arestas são tipadas, direcionadas (com exceção explícita), versionadas
// (bitemporal) e ponderadas. O ID é determinístico para garantir
// idempotência de coletores.
package edge

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"costEngine/internal/entity/node"
)

// Type classifica a aresta dentro do core ontológico.
type Type string

const (
	TypeContains         Type = "CONTAINS"          // hierarquia / contenção
	TypeDeployedOn       Type = "DEPLOYED_ON"       // compute → host/cluster
	TypeAttachedTo       Type = "ATTACHED_TO"       // disco/ENI → compute
	TypeRoutes           Type = "ROUTES"            // network → network
	TypePeers            Type = "PEERS"             // simétrica (VPC peering)
	TypeDependsOn        Type = "DEPENDS_ON"        // dependência declarada/inferida
	TypeCommunicatesWith Type = "COMMUNICATES_WITH" // tráfego observado
	TypeReplaces         Type = "REPLACES"          // versionamento estrutural

	// Code plane (F-007).
	TypeDefinedIn Type = "DEFINED_IN" // Endpoint/Function → Service

	// Bridge code→infra (F-009, ADR-004).
	TypeServiceRunsOn Type = "RUNS_ON" // Service → Compute

	// Org plane (F-010).
	TypeMemberOf  Type = "MEMBER_OF"  // Person → Squad
	TypePartOf    Type = "PART_OF"    // Squad → Team
	TypeReportsTo Type = "REPORTS_TO" // Person → Person

	// Bridge org→code (F-011): code ownership a partir do CODEOWNERS.
	TypeOwns Type = "OWNS" // Person|Team → Service
)

// Meta carrega os atributos transversais presentes em toda aresta.
type Meta struct {
	ValidFrom   time.Time      `json:"valid_from"`
	ValidTo     *time.Time     `json:"valid_to,omitempty"`
	ObservedAt  time.Time      `json:"observed_at"`
	Source      node.Source    `json:"source"`
	Confidence  float32        `json:"confidence"`
	Directional bool           `json:"directional"`
	Weight      float64        `json:"weight,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Lineage     node.Lineage   `json:"lineage,omitempty"`
}

// IsCurrent retorna true se a aresta ainda é a corrente.
func (m Meta) IsCurrent() bool { return m.ValidTo == nil }

// Edge é o contrato mínimo que toda aresta implementa.
type Edge interface {
	ID() string
	Type() Type
	From() node.URN
	To() node.URN
	Meta() Meta
}

// Base é o embed comum a todos os structs concretos.
type Base struct {
	EdgeID   string    `json:"id"`
	EdgeType Type      `json:"type"`
	FromURN  node.URN  `json:"from"`
	ToURN    node.URN  `json:"to"`
	EdgeMeta Meta      `json:"meta"`
}

func (b Base) ID() string     { return b.EdgeID }
func (b Base) Type() Type     { return b.EdgeType }
func (b Base) From() node.URN { return b.FromURN }
func (b Base) To() node.URN   { return b.ToURN }
func (b Base) Meta() Meta     { return b.EdgeMeta }

// DeterministicID gera um ID estável a partir de (from|type|to|validFrom).
// Coletores podem reprocessar o mesmo evento sem duplicar arestas.
func DeterministicID(from node.URN, t Type, to node.URN, validFrom time.Time) string {
	h := sha256.New()
	h.Write([]byte(from))
	h.Write([]byte{'|'})
	h.Write([]byte(t))
	h.Write([]byte{'|'})
	h.Write([]byte(to))
	h.Write([]byte{'|'})
	h.Write([]byte(validFrom.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}
