package node

// Account representa uma conta/subscription/project no provedor.
// É um nó-escopo: agrupa recursos para fins de billing, IAM e isolamento.
type Account struct {
	Base
	ProviderID  ProviderID `json:"provider_id"`
	ExternalID  string     `json:"external_id"`  // ID nativo (12 dígitos AWS, GUID Azure, etc.)
	DisplayName string     `json:"display_name"`
	ParentURN   *URN       `json:"parent_urn,omitempty"` // OU, folder, management group
}
