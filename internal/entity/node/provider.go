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
	// nós Person/Team/Squad/Role/etc. O slot `<account>` da URN é
	// reusado como `<tenant>` (ex.: "acme") para suportar multi-org.
	ProviderOrg ProviderID = "org"

	// ProviderGov é o "provider lógico" do governance plane (E-008).
	// Hospeda Domain/Capability/Feature/Epic/UserStory/Persona. O slot
	// `<account>` da URN é o `<company>` (tenant). Distinto de
	// ProviderOrg para deixar claro o eixo: org = quem trabalha,
	// gov = o que é entregue / por quê.
	ProviderGov ProviderID = "gov"
)

// Provider é um nó-escopo: a "raiz" lógica de um provedor de cloud.
// Não é Resource (não tem custo direto, não tem região).
type Provider struct {
	Base
	ID   ProviderID `json:"id"`
	Name string     `json:"name"`
}
