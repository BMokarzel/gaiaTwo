package edge

// CommunicatesWith é um FATO observado de tráfego entre recursos
// (flow logs, service mesh, eBPF).
//
// Pode existir sem DependsOn (tráfego oportunista) e vice-versa
// (dependência declarada que ainda não comunicou).
//
// Convenção: Meta.Weight = BytesIn + BytesOut, para habilitar queries
// "top talkers" sem inspecionar Properties.
type CommunicatesWith struct {
	Base
	Protocol    string  `json:"protocol,omitempty"`
	PortDst     uint16  `json:"port_dst,omitempty"`
	BytesIn     uint64  `json:"bytes_in,omitempty"`
	BytesOut    uint64  `json:"bytes_out,omitempty"`
	Packets     uint64  `json:"packets,omitempty"`
	LatencyP50  float32 `json:"latency_p50_ms,omitempty"`
	LatencyP99  float32 `json:"latency_p99_ms,omitempty"`
	CrossZone   bool    `json:"cross_zone,omitempty"`   // egress inter-AZ
	CrossRegion bool    `json:"cross_region,omitempty"` // egress inter-region
}
