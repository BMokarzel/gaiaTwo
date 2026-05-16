package edge

// DeployedOn modela onde um workload (container/function) executa.
//
// Separado de Contains porque a relação é mais fraca: o workload pode
// migrar entre hosts sem deixar de existir.
type DeployedOn struct {
	Base
	Scheduled bool   `json:"scheduled"`         // true = orquestrado (k8s scheduler)
	Replicas  uint32 `json:"replicas,omitempty"`
}
