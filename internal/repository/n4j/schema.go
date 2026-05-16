package n4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Migrations contém os statements DDL para inicializar o schema Neo4j.
// Idempotentes (IF NOT EXISTS), seguros para rodar a cada boot.
var Migrations = []string{
	// Unicidade por (urn, version) — várias versões coexistem por URN.
	"CREATE CONSTRAINT node_urn_version_unique IF NOT EXISTS " +
		"FOR (n:CeNode) REQUIRE (n.urn, n.version) IS UNIQUE",

	// Index principal: lookup por URN.
	"CREATE INDEX node_urn_idx IF NOT EXISTS FOR (n:CeNode) ON (n.urn)",

	// Index secundário: bridge para o CUR. (provider, external_id) → URN.
	// O filtro por account ocorre no WHERE; o índice acelera o lookup
	// inicial (F-003/S-005 — cross-account fallback).
	"CREATE INDEX node_external_idx IF NOT EXISTS " +
		"FOR (n:CeNode) ON (n.provider, n.external_id)",

	// Alias de short ID quando o nó foi indexado por ARN — permite
	// resolver lookups com a forma curta (F-003/S-002).
	"CREATE INDEX node_short_id_idx IF NOT EXISTS " +
		"FOR (n:CeNode) ON (n.provider, n.short_id)",

	// Index para queries "current view".
	"CREATE INDEX node_current_idx IF NOT EXISTS FOR (n:CeNode) ON (n.valid_to)",

	// Edges: unicidade por (id, version).
	"CREATE CONSTRAINT edge_id_version_unique IF NOT EXISTS " +
		"FOR ()-[r:CE_EDGE]-() REQUIRE (r.id, r.version) IS UNIQUE",

	// Index para queries por tipo de edge.
	"CREATE INDEX edge_type_idx IF NOT EXISTS FOR ()-[r:CE_EDGE]-() ON (r.type)",

	// Current edges.
	"CREATE INDEX edge_current_idx IF NOT EXISTS FOR ()-[r:CE_EDGE]-() ON (r.valid_to)",
}

// Migrate aplica as migrações idempotentes. Deve ser chamado uma vez na
// inicialização da aplicação.
func (c *Client) Migrate(ctx context.Context) error {
	sess := c.session(ctx, neo4j.AccessModeWrite)
	defer sess.Close(ctx)

	for _, stmt := range Migrations {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, stmt, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("n4j: migration %q: %w", stmt, err)
		}
	}
	return nil
}
