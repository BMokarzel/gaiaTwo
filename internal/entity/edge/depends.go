package edge

// DependsOn modela dependência lógica entre recursos.
//
// Pode ser:
//   - Declared = true  → obtida de IaC/manifest (forte garantia).
//   - Declared = false → inferida (DNS lookup, env var, service mesh).
//
// Hard indica se a ausência da dependência impede o funcionamento.
type DependsOn struct {
	Base
	Declared bool   `json:"declared"`
	Hard     bool   `json:"hard"`
	Protocol string `json:"protocol,omitempty"` // "tcp", "https", "grpc"
}
