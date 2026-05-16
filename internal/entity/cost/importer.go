package cost

import (
	"context"
	"errors"
	"time"
)

// Sentinelas de erro do pacote.
var (
	// ErrPartitionNotFound é retornado quando a partition solicitada não
	// existe na fonte (ex.: mês sem CUR ainda entregue).
	ErrPartitionNotFound = errors.New("cost: partition not found")
	// ErrInvalidLine é retornado quando uma linha do CUR não pode ser
	// mapeada (coluna obrigatória ausente, tipo inválido). Linhas
	// inválidas seguem para o sink de erros, não param o stream.
	ErrInvalidLine = errors.New("cost: invalid line")
)

// CURImporter abstrai a origem (AWS CUR hoje, GCP/Azure futuro).
//
// Implementações esperadas:
//
//   - `internal/modules/cost/source/local` — diretório com Parquet (dev/tests).
//   - `internal/modules/cost/source/s3`    — S3 com manifest CUR v1/v2.
//
// O `Read` retorna um canal que streama `Line`s já normalizadas; o canal
// fecha quando a partition acaba ou quando o contexto é cancelado.
// Erros não-fatais (linha malformada) são reportados via canal de erros
// separado (ver `ReadResult`).
type CURImporter interface {
	// Discover detecta partitions disponíveis desde "since".
	Discover(ctx context.Context, since time.Time) ([]Partition, error)
	// Read entrega o stream de linhas + erros de uma partition.
	Read(ctx context.Context, p Partition) (ReadResult, error)
}

// ReadResult agrega os canais de saída de uma leitura de partition.
//
//   - Lines fecha quando a partition termina ou ctx é cancelado.
//   - Errors carrega `ParseError` para linhas que não puderam ser
//     mapeadas. Não é fatal — o consumidor decide se persiste/aborta.
type ReadResult struct {
	Lines  <-chan Line
	Errors <-chan ParseError
}

// ParseError descreve uma linha que não pôde ser convertida para `Line`.
// Persistida pelo sink em `fct_cur_errors` para auditoria.
type ParseError struct {
	SourceFile string    `json:"source_file"`
	RowIndex   int64     `json:"row_index"`
	Reason     string    `json:"reason"`
	Raw        string    `json:"raw,omitempty"` // amostra truncada
	At         time.Time `json:"at"`
}
