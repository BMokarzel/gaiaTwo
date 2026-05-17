// Package openapi implementa F-008: ingestão de specs OpenAPI 3.0/3.1
// produzindo nós `Endpoint` ligados a um `Service` pré-existente.
//
// Pipeline:
//
//	parse(spec.yaml|spec.json) → Spec → Collect(cfg) → Result{Endpoints, Edges}
//
// Idempotência: URN do Endpoint é determinística em `(service, method,
// path)`. A versão muda somente quando algum campo significativo
// (handler operation_id, schemas de request/response, tags) muda — o
// repositório bitemporal cuida do versionamento via `ContentHash`.
//
// Limites do MVP:
//   - Apenas OpenAPI 3.0 e 3.1 (Swagger 2.0 fora do escopo).
//   - `$ref` é capturado literalmente; sem resolução transitiva.
//   - Nenhum schema é populado como nó (F-022 fará isso).
package openapi
