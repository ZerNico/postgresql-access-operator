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

// DatabaseExists checks if a database exists in the PostgreSQL instance.
func (c *Client) DatabaseExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := c.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", name,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking if database %q exists: %w", name, err)
	}
	return exists, nil
}

// CreateDatabase creates a database if it does not already exist.
// CREATE DATABASE cannot run inside a transaction, so we check first and
// use a direct statement.
func (c *Client) CreateDatabase(ctx context.Context, name string) error {
	exists, err := c.DatabaseExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Use pgx.Identifier for safe quoting of the database name.
	quotedName := pgx.Identifier{name}.Sanitize()
	_, err = c.conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quotedName))
	if err != nil {
		return fmt.Errorf("creating database %q: %w", name, err)
	}
	return nil
}

// DropDatabase drops a database if it exists.
// DROP DATABASE cannot run inside a transaction.
func (c *Client) DropDatabase(ctx context.Context, name string) error {
	exists, err := c.DatabaseExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	quotedName := pgx.Identifier{name}.Sanitize()
	_, err = c.conn.Exec(ctx, fmt.Sprintf("DROP DATABASE %s", quotedName))
	if err != nil {
		return fmt.Errorf("dropping database %q: %w", name, err)
	}
	return nil
}
