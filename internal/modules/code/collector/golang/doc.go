// Package golang é o coletor Go (F-007 D6).
//
// Entrada: caminho local de um repositório Go.
// Saída: `Result` com slices de `node.Service`, `node.Endpoint`,
// `node.Function` + edges `DEFINED_IN`.
//
// Pipeline:
//
//	walk(repo) → discover(go.mod) → parsePackages → emit{Function,Endpoint}
//
// Decisões (F-007 D5/D6):
//   - Handlers HTTP detectados: `net/http` (`http.HandleFunc`,
//     `*ServeMux.HandleFunc`, `*ServeMux.Handle`) e `go-chi/chi`
//     (`r.Get/Post/Put/Delete/Patch/Head/Options/Method/MethodFunc`).
//   - Funções exportadas relevantes: top-level (não-método) em pacotes
//     cujo nome curto contém "handler", "service" ou "usecase". Métodos
//     em receivers são incluídos com nome `(T).Method`.
//   - Filtros default: ignorar `vendor/`, `*_test.go`, paths com `gen/`
//     ou `mocks/`. Override por configuração.
//   - Idempotência: URN é determinística (`node.NewServiceURN` etc.) —
//     reprocessar emite mesmo conjunto de nós.
package golang
