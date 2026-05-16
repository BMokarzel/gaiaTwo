package node

// ProviderID identifica o provedor de infraestrutura.
type ProviderID string

const (
	ProviderAWS    ProviderID = "aws"
	ProviderGCP    ProviderID = "gcp"
	ProviderAzure  ProviderID = "azure"
	ProviderK8s    ProviderID = "kubernetes"
	ProviderOnPrem ProviderID = "onprem"

	// ProviderCode é o "provider lógico" do code plane (F-007). Hospeda
	// nós Service/Endpoint/Function extraídos de repositórios. O slot
	// `<account>` da URN é reusado como `<repo>` para esses nós.
	ProviderCode ProviderID = "code"

	// ProviderOrg é o "provider lógico" do org plane (F-010). Hospeda
	// nós Person/Team/Squad/etc. O slot `<account>` da URN é reusado
	// como `<tenant>` (ex.: "acme") para suportar multi-org no futuro.
	ProviderOrg ProviderID = "org"
)

// Provider é um nó-escopo: a "raiz" lógica de um provedor de cloud.
// Não é Resource (não tem custo direto, não tem região).
type Provider struct {
	Base
	ID   ProviderID `json:"id"`
	Name string     `json:"name"`
}
