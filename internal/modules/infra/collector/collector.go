// Package collector define a interface dos descobridores de infraestrutura.
//
// Um Collector lê o estado de um provedor (AWS, GCP, Azure, K8s, ...) e
// emite nós que implementam node.Resource. A persistência fica fora — o
// pipeline da camada de service consome o canal e versiona via
// repository.NodeRepository.
//
// Esta interface é o contrato comum; cada provider tem sua própria
// implementação em subpacote (collector/aws, collector/gcp, ...).
package collector

import (
	"context"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// Scope delimita o que coletar numa execução. Mantido propositalmente
// pequeno: providers ricos (multi-conta, multi-projeto) modelam o
// supérfluo em flags específicas.
type Scope struct {
	// Account é o identificador nativo da conta no provedor.
	// Ex.: "123456789012" (AWS), "my-project" (GCP).
	Account string

	// Region é a região alvo. Vazio quando o coletor é global (S3, IAM)
	// ou faz fan-out interno.
	Region string
}

// Collector descobre recursos de um provider e emite-os via canal.
//
// Contrato de Discover:
//   - res é fechado quando a coleta termina sem erros pendentes.
//   - errs carrega erros recuperáveis (falha em 1 recurso isolado); o
//     canal é fechado junto com res.
//   - cancelamento via ctx interrompe a coleta; canais ainda são fechados
//     pelo collector.
type Collector interface {
	Provider() node.ProviderID
	Discover(ctx context.Context, scope Scope) (<-chan node.Resource, <-chan error)
}

// Validator é uma sub-capacidade opcional: o collector pode reportar
// prontidão (credenciais, conectividade, identidade) antes de iniciar a
// descoberta pesada. CLI roda Validate antes de Discover para falhar
// rápido com mensagem clara.
type Validator interface {
	Validate(ctx context.Context, scope Scope) error
}

// ScopeTopology é o resultado mínimo da descoberta da hierarquia de
// escopo (Account → Region → Zone) para um provider/scope. Estes nós
// não são Resources cobráveis — são "estrutura" do grafo, ancestrais
// que toda Resource posterior referencia.
type ScopeTopology struct {
	Account node.Account
	Region  node.Region
	Zones   []node.Zone
	// Edges traz `Contains` Account→Region e Region→Zone (um por Zone).
	Edges []edge.Contains
}

// ScopeDiscoverer descobre a hierarquia de escopo para um Scope.
//
// É um contrato separado de Collector porque scopes (Account/Region/Zone)
// não são Resource (não têm custo direto, não emitem ExternalID), e a
// descoberta deles é pré-requisito barato e síncrono — não faz sentido
// passar por canais.
type ScopeDiscoverer interface {
	DiscoverScope(ctx context.Context, scope Scope) (ScopeTopology, error)
}

// ComputeBatch é o resultado bruto de uma descoberta de Compute. Os
// edges Zone→Compute (Contains) são pré-computados pelo discoverer,
// que conhece a AZ de cada instância.
//
// Mantemos um snapshot estático (sem canais) porque o passo seguinte
// é uma decisão bitemporal por nó (Upsert vs Touch), e isso é mais
// claro com a lista materializada.
type ComputeBatch struct {
	Computes []node.Compute
	// Edges Zone→Compute, um por Compute (o discoverer já sabe a AZ).
	Edges []edge.Contains
}

// ComputeDiscoverer descobre workloads compute (VMs, containers, etc.)
// para um Scope. Separado de Collector pelo mesmo motivo de
// ScopeDiscoverer: o consumidor precisa raciocinar bitemporalmente
// sobre cada item, e canais escondem a granularidade.
type ComputeDiscoverer interface {
	DiscoverCompute(ctx context.Context, scope Scope, topo ScopeTopology) (ComputeBatch, error)
}

// PersistenceBatch é o resultado bruto de uma descoberta de Persistence
// (EBS, S3, RDS, ...). Edges Zone→Persistence (Contains) e, quando
// aplicável, Persistence→Compute (AttachedTo) são pré-computados pelo
// discoverer, que conhece a AZ e o(s) attachment(s) de cada volume.
//
// Mantemos snapshot estático pelo mesmo motivo de ComputeBatch.
type PersistenceBatch struct {
	Persistences []node.Persistence
	// Edges contém TODAS as edges relacionadas a essa coleta —
	// Zone→Persistence (Contains) e Persistence→Compute (AttachedTo).
	// O tipo concreto varia; a service layer despacha via endpointKinds.
	Edges []edge.Edge
}

// PersistenceDiscoverer descobre recursos de armazenamento (block,
// object, rdbms, ...). Múltiplas implementações coexistem (uma por
// flavor); a service layer permite registrar várias e agrega os
// resultados num único ciclo de upsert bitemporal.
type PersistenceDiscoverer interface {
	DiscoverPersistence(ctx context.Context, scope Scope, topo ScopeTopology) (PersistenceBatch, error)
}

// NetworkBatch é o resultado bruto de uma descoberta de Network
// (VPC, Subnet, SG, LB, ...). Edges Contains entre os componentes
// (Region→VPC, Zone→Subnet, VPC→Subnet, etc.) são pré-computados
// pelo discoverer, que conhece a topologia do provedor.
//
// Mantemos snapshot estático pelos mesmos motivos de ComputeBatch /
// PersistenceBatch.
type NetworkBatch struct {
	Networks []node.Network
	// Edges contém TODAS as edges relacionadas a essa coleta —
	// Region→VPC, Zone→Subnet, VPC→Subnet (Contains), e quando aplicável
	// AttachedTo, Routes, etc. O tipo concreto varia; a service layer
	// despacha via endpointKinds.
	Edges []edge.Edge
}

// NetworkDiscoverer descobre recursos de rede (VPC, Subnet, SG, LB,
// gateway, ENI, ...). Múltiplas implementações coexistem (uma por
// família/flavor); a service layer registra várias e agrega num único
// ciclo de upsert bitemporal, mesmo padrão de PersistenceDiscoverer.
type NetworkDiscoverer interface {
	DiscoverNetwork(ctx context.Context, scope Scope, topo ScopeTopology) (NetworkBatch, error)
}
