// Package clickhouse encapsula o cliente ClickHouse usado pela camada
// de custo. ClickHouse foi escolhido como storage analítico (gold/silver
// unificado para o MVP — ver F-004 D1) porque dá sub-segundo em
// `GROUP BY` sobre rollups e oferece `ReplacingMergeTree` para
// idempotência por chave.
//
// Este pacote oferece:
//
//   - `Client` thin wrapper sobre `clickhouse.Conn` (DSN-driven).
//   - `Migrations` idempotentes para criar `fct_cur_lines` e `fct_cur_errors`.
//
// Imports permitidos: `entity/cost`. NÃO importa `modules/*`.
package clickhouse

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Config descreve a conexão ao ClickHouse.
type Config struct {
	Addrs    []string
	Database string
	Username string
	Password string
}

// Client é um wrapper fino sobre `driver.Conn`. Mantido pequeno para
// facilitar mock em testes (a `Conn` é interface).
type Client struct {
	Conn driver.Conn
	DB   string
}

// Open abre uma conexão e faz ping. Caller deve chamar Close().
func Open(ctx context.Context, cfg Config) (*Client, error) {
	if len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("clickhouse: Addrs empty")
	}
	if cfg.Database == "" {
		cfg.Database = "default"
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addrs,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("clickhouse: ping: %w", err)
	}
	return &Client{Conn: conn, DB: cfg.Database}, nil
}

// Close fecha a conexão. Idempotente.
func (c *Client) Close() error {
	if c.Conn == nil {
		return nil
	}
	err := c.Conn.Close()
	c.Conn = nil
	return err
}

// Migrate executa todas as Migrations sequencialmente.
func (c *Client) Migrate(ctx context.Context) error {
	for i, stmt := range Migrations {
		if err := c.Conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("clickhouse: migration %d: %w", i, err)
		}
	}
	return nil
}
