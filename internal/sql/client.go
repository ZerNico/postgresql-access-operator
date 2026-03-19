/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Client wraps a pgx connection for administrative PostgreSQL operations.
type Client struct {
	conn *pgx.Conn
}

// ConnectConfig holds the parameters needed to connect to PostgreSQL.
type ConnectConfig struct {
	Host     string
	Port     int32
	Database string
	Username string
	Password string
}

// Connect establishes a direct connection to PostgreSQL.
// Uses a direct connection (not a pool) because CREATE DATABASE / CREATE ROLE
// cannot run inside a transaction.
func Connect(ctx context.Context, cfg ConnectConfig) (*Client, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database, cfg.Username, cfg.Password,
	)

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connecting to PostgreSQL: %w", err)
	}

	return &Client{conn: conn}, nil
}

// Close closes the underlying connection.
func (c *Client) Close(ctx context.Context) error {
	return c.conn.Close(ctx)
}

// Ping checks if the connection is alive.
func (c *Client) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}
