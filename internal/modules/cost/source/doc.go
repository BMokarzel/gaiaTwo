// Package source abstrai a origem das partitions CUR.
//
// Implementações:
//
//   - `source/local` — diretório no filesystem. Útil para dev e tests
//     (fixtures Parquet).
//   - `source/s3`    — S3 + manifest CUR. Produção.
//
// Ambas implementam `cost.CURImporter` (canal de `cost.Line` + canal de
// `cost.ParseError`). Source é stateless — descoberta de partitions é
// re-executada a cada run; idempotência fica no sink.
package source
