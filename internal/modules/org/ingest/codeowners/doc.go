// Package codeowners é o parser e ingestor de arquivos CODEOWNERS
// (F-011). Suporta o subset necessário para o MVP:
//
//   - Comentários iniciando com `#`.
//   - Comentário inline (texto após `#` é descartado).
//   - Linhas em branco ignoradas.
//   - Regras `<pattern> @owner1 [@owner2 ...]`.
//   - Owners em duas formas: `@user` (Person via github_handle) e
//     `@org/team` (Team via nome).
//
// NÃO inclui (follow-up): paths múltiplos, negação, escape, e a regra
// "última match vence" do GitHub. O MVP foca na linha global (`*`); o
// resolver/emit pode escolher qual regra aplicar.
//
// Erros de linha individual (pattern vazio, owners ausentes) viram
// `RowError` não-fatal: o ingest continua para que um typo isolado não
// derrube o pipeline inteiro.
package codeowners
