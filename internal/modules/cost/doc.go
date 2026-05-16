// Package cost agrupa os módulos da camada econômica (CUR parser, sink,
// allocation engine).
//
// Decomposição:
//
//   - `cost/parser` — converte arquivos CUR (Parquet) em `cost.Line`.
//   - `cost/source` — abstrai a origem (local fs, S3) das partitions.
//   - `cost/sink`   — escreve `cost.Line` no storage analítico (ClickHouse).
//
// Imports permitidos: `entity/cost`, `entity/node`. Este pacote NÃO importa
// `modules/core` nem `modules/infra` (regra de pirâmide modular).
package cost
