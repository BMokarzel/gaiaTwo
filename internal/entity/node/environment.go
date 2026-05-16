package node

// EnvironmentTier classifica o ambiente.
type EnvironmentTier string

const (
	TierProd    EnvironmentTier = "prod"
	TierStaging EnvironmentTier = "staging"
	TierDev     EnvironmentTier = "dev"
	TierTest    EnvironmentTier = "test"
)

// Environment é um Concept (não infra real cobrável) — modela a noção
// lógica de ambiente para queries semânticas.
type Environment struct {
	Base
	Tier EnvironmentTier `json:"tier"`
	Name string          `json:"name"`
}
