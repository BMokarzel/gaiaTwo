package cost

import (
	"time"

	"costEngine/internal/entity/node"
)

// Partition descreve um conjunto de arquivos Parquet entregue por uma
// fonte (manifest CUR no S3, diretório local em dev). É a unidade
// atômica de processamento: o ingestor lê uma partition por vez e o
// sink commita por partition.
//
// `RunID` identifica a execução do CUR no provedor — partitions com o
// mesmo `(ReportID, BillingMonth)` podem ter `RunID` diferentes (CUR
// reescreve o mês corrente). O sink usa isso para garantir que apenas
// o RunID mais recente fica visível.
type Partition struct {
	Provider     node.ProviderID `json:"provider"`
	ReportID     string          `json:"report_id"`
	BillingMonth time.Time       `json:"billing_month"` // sempre dia 1 UTC
	URI          string          `json:"uri"`           // s3://... ou file://...
	Manifest     string          `json:"manifest,omitempty"`
	RunID        string          `json:"run_id"`
	ObjectKeys   []string        `json:"object_keys"` // caminhos relativos a URI
}

// IsZero reporta se a partition é o zero value.
func (p Partition) IsZero() bool {
	return p.Provider == "" && p.ReportID == "" && p.URI == ""
}
