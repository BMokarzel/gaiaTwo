package edge

// AttachedTo modela acoplamento físico/lógico observável.
//
// Persistence(block) ──▶ Compute
// Network(eni)       ──▶ Compute
//
// Distingue-se de DependsOn por ser estrutural, não inferido a partir
// de comportamento de runtime.
type AttachedTo struct {
	Base
	MountPoint string `json:"mount_point,omitempty"`
	ReadOnly   bool   `json:"read_only,omitempty"`
}
