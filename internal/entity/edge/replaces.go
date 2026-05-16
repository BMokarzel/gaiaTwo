package edge

// Replaces modela versionamento estrutural entre versões de um recurso.
//
// resource@v2 ──▶ resource@v1
//
// Permite traversal histórico: "qual era o estado anterior deste cluster?".
type Replaces struct {
	Base
	Reason string `json:"reason,omitempty"` // "config-change", "scale-up", "recreate"
}
