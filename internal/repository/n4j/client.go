// Package n4j é a implementação Neo4j das interfaces NodeRepository e
// EdgeRepository.
//
// Convenções de modelagem no grafo Neo4j:
//
//   - Label primária do nó = node.Kind (Provider, Account, ..., Compute).
//   - Toda versão de um nó é um vértice Neo4j separado, ligado por
//     :REPLACES à versão anterior. A versão corrente tem `valid_to = null`.
//   - Propriedades comuns ficam no top-level (urn, kind, version,
//     valid_from, valid_to, observed_at, source_*, confidence).
//   - Labels/Properties/Tags/Spec do domínio são serializados como JSON
//     em props únicas (`labels_json`, `properties_json`, `tags_json`, `spec_json`)
//     para evitar acoplar o schema do Cypher ao schema da app.
//
// As migrações de schema (constraints + indexes) ficam em schema.go.
package n4j

import (
	"context"
	"errors"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Config descreve a conexão Neo4j.
type Config struct {
	URI      string // bolt://host:7687
	Username string
	Password string
	Database string // "" → default
}

// Client embrulha o driver Neo4j com helpers de sessão.
type Client struct {
	driver neo4j.DriverWithContext
	db     string
}

// Connect abre o driver e valida conectividade.
func Connect(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URI == "" {
		return nil, errors.New("n4j: empty URI")
	}
	auth := neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	drv, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return nil, fmt.Errorf("n4j: driver: %w", err)
	}
	if err := drv.VerifyConnectivity(ctx); err != nil {
		_ = drv.Close(ctx)
		return nil, fmt.Errorf("n4j: verify: %w", err)
	}
	return &Client{driver: drv, db: cfg.Database}, nil
}

// Close libera o driver.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.driver == nil {
		return nil
	}
	return c.driver.Close(ctx)
}

// session abre uma sessão configurada para o database do client.
func (c *Client) session(ctx context.Context, mode neo4j.AccessMode) neo4j.SessionWithContext {
	return c.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode:   mode,
		DatabaseName: c.db,
	})
}

// withWrite executa fn numa transação de escrita.
func (c *Client) withWrite(ctx context.Context, fn func(tx neo4j.ManagedTransaction) (any, error)) (any, error) {
	sess := c.session(ctx, neo4j.AccessModeWrite)
	defer sess.Close(ctx)
	return sess.ExecuteWrite(ctx, fn)
}

// withRead executa fn numa transação de leitura.
func (c *Client) withRead(ctx context.Context, fn func(tx neo4j.ManagedTransaction) (any, error)) (any, error) {
	sess := c.session(ctx, neo4j.AccessModeRead)
	defer sess.Close(ctx)
	return sess.ExecuteRead(ctx, fn)
}
