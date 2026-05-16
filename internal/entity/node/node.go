// Package node define o contrato raiz e os tipos transversais para todo
// nó do grafo de inteligência arquitetural.
//
// Princípios:
//   - Kind é fechado (core estável); extensões vão em Labels/Properties.
//   - Toda entidade carrega metadados bitemporais (ValidFrom/ValidTo +
//     ObservedAt) e lineage.
//   - Resource é a sub-interface dos nós cobráveis (infra real). Scopes
//     como Provider/Account/Region implementam apenas Node.
package node

import "time"

// URN é o identificador canônico estável de um nó.
// Formato: urn:ce:<provider>:<account>:<kind>/<id>
type URN string

// Kind classifica o nó dentro do core ontológico.
type Kind string

const (
	KindProvider    Kind = "provider"
	KindAccount     Kind = "account"
	KindRegion      Kind = "region"
	KindZone        Kind = "zone"
	KindEnvironment Kind = "environment"
	KindCompute     Kind = "compute"
	KindPersistence Kind = "persistence"
	KindMessaging   Kind = "messaging"
	KindNetwork     Kind = "network"

	// Code plane (F-007) — kinds para nós extraídos de repositórios.
	// Não são Resource (sem account/region/spec do provedor); são Node puros.
	KindService  Kind = "service"  // 1 módulo Go (raiz com go.mod)
	KindEndpoint Kind = "endpoint" // handler HTTP (método+rota)
	KindFunction Kind = "function" // função exportada relevante

	// Org plane (F-010) — kinds para hierarquia organizacional.
	// Person.URN usa hash do email para minimizar PII (ver F-010 D2).
	KindPerson Kind = "person"
	KindTeam   Kind = "team"
	KindSquad  Kind = "squad"
)

// Method identifica como um fato foi obtido.
type Method string

const (
	MethodAPI      Method = "api"
	MethodInferred Method = "inferred"
	MethodDeclared Method = "declared"
	MethodImported Method = "imported"
)

// Source descreve a origem da observação.
type Source struct {
	Collector string `json:"collector"`
	RunID     string `json:"run_id"`
	Method    Method `json:"method"`
}

// Lineage descreve a cadeia causal de um fato inferido/derivado.
type Lineage struct {
	DerivedFrom []URN  `json:"derived_from,omitempty"`
	Rule        string `json:"rule,omitempty"`
}

// Meta carrega os atributos transversais presentes em todo nó.
//
// Bitemporal:
//   - ValidFrom/ValidTo  → quando o fato vale no mundo real.
//   - ObservedAt         → quando o sistema soube (transaction time).
type Meta struct {
	Version    uint64         `json:"version"`
	ValidFrom  time.Time      `json:"valid_from"`
	ValidTo    *time.Time     `json:"valid_to,omitempty"`
	ObservedAt time.Time      `json:"observed_at"`
	Source     Source         `json:"source"`
	Confidence float32        `json:"confidence"`
	Labels     []string       `json:"labels,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	Lineage    Lineage        `json:"lineage,omitempty"`
}

// IsCurrent retorna true se o fato ainda é o corrente (sem ValidTo).
func (m Meta) IsCurrent() bool { return m.ValidTo == nil }

// Node é o contrato mínimo que todo nó do grafo implementa.
type Node interface {
	URN() URN
	Kind() Kind
	Meta() Meta
}

// Resource é a sub-interface dos nós que representam infra real, cobrável
// e geograficamente localizada. Scopes (Provider/Account/Region) NÃO
// implementam Resource.
type Resource interface {
	Node
	Provider() ProviderID
	AccountURN() URN
	RegionURN() URN
	// ExternalID retorna o identificador nativo no provedor (ARN, instance-id,
	// bucket name, etc). É o bridge para fontes externas — em especial o CUR
	// da AWS, que usa esse valor em lineItem/ResourceId.
	ExternalID() string
	NativeTags() map[string]string
	Spec() map[string]any
}

// Base é o embed comum a todos os structs concretos.
type Base struct {
	NodeURN  URN  `json:"urn"`
	NodeKind Kind `json:"kind"`
	NodeMeta Meta `json:"meta"`
}

func (b Base) URN() URN   { return b.NodeURN }
func (b Base) Kind() Kind { return b.NodeKind }
func (b Base) Meta() Meta { return b.NodeMeta }
