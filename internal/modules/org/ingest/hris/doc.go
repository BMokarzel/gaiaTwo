// Package hris é o ingestor de CSV de HRIS (F-010).
//
// Entrada: arquivo CSV no schema:
//
//	email, name, role, team, squad, manager_email, start_date, end_date_or_blank
//
// Saída: `Result` com slices de `node.Person`, `node.Team`, `node.Squad`
// + edges `MemberOf`, `PartOf`, `ReportsTo`.
//
// Decisões (F-010 D1..D8):
//   - URN da Person usa hash do email (SHA-256 lower+trim, 16 bytes
//     hex) — email plain nunca é armazenado.
//   - Team/Squad indexados por slug determinístico (lowercase, non-alpha
//     → "-").
//   - `end_date_or_blank` preenchido → Person fechada (Delete) + edges
//     MEMBER_OF saindo dela fechadas.
//   - Linha mal-formada → relatório, não derruba execução.
//   - Ciclo em `manager_email` → erro fatal (aborta o ingest).
package hris
