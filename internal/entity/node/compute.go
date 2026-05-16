package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// ComputeFlavor classifica o tipo de workload compute.
type ComputeFlavor string

const (
	ComputeVM        ComputeFlavor = "vm"        // EC2, GCE, Azure VM
	ComputeContainer ComputeFlavor = "container" // ECS task, k8s pod
	ComputeFunction  ComputeFlavor = "function"  // Lambda, Cloud Functions
	ComputeCluster   ComputeFlavor = "cluster"   // EKS, GKE, AKS
	ComputeBatch     ComputeFlavor = "batch"     // Batch, Dataproc job
)

// LifecycleState representa o estado operacional do compute.
type LifecycleState string

const (
	StatePending    LifecycleState = "pending"
	StateRunning    LifecycleState = "running"
	StateStopped    LifecycleState = "stopped"
	StateTerminated LifecycleState = "terminated"
)

// Compute representa qualquer workload de execução: VM, container,
// função serverless ou cluster orquestrador.
type Compute struct {
	Base
	ProviderID   ProviderID        `json:"provider_id"`
	Account      URN               `json:"account_urn"`
	Region       URN               `json:"region_urn"`
	ExtID        string            `json:"external_id"`             // ID nativo no provedor (ARN, instance-id)
	Flavor       ComputeFlavor     `json:"flavor"`
	InstanceType string            `json:"instance_type,omitempty"` // "m6i.large"
	VCPUs        float32           `json:"vcpus,omitempty"`
	MemoryMiB    uint64            `json:"memory_mib,omitempty"`
	State        LifecycleState    `json:"state,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	SpecRaw      map[string]any    `json:"spec_raw,omitempty"`
}

// Resource interface implementation.
func (c Compute) Provider() ProviderID          { return c.ProviderID }
func (c Compute) AccountURN() URN               { return c.Account }
func (c Compute) RegionURN() URN                { return c.Region }
func (c Compute) ExternalID() string            { return c.ExtID }
func (c Compute) NativeTags() map[string]string { return c.Tags }
func (c Compute) Spec() map[string]any          { return c.SpecRaw }

// ContentHash retorna um hash determinístico dos *campos significativos*
// de um Compute — propositalmente exclui Meta (versão, timestamps,
// observed_at) e SpecRaw (ruído do provedor). Usado por coletores para
// detectar mudança e decidir entre Upsert (nova versão) e Touch (só
// atualiza observed_at).
//
// Inclui: Provider, Flavor, InstanceType, VCPUs, MemoryMiB, State, Tags
// (ordenadas por chave). NÃO inclui ExtID/Account/Region (parte da URN,
// não podem variar para o mesmo nó).
func (c Compute) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(c.ProviderID))
	sb.WriteByte('|')
	sb.WriteString(string(c.Flavor))
	sb.WriteByte('|')
	sb.WriteString(c.InstanceType)
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatFloat(float64(c.VCPUs), 'f', -1, 32))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatUint(c.MemoryMiB, 10))
	sb.WriteByte('|')
	sb.WriteString(string(c.State))
	sb.WriteByte('|')

	keys := make([]string, 0, len(c.Tags))
	for k := range c.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(c.Tags[k])
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
