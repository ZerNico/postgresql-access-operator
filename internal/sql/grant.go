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
	"strings"

	"github.com/jackc/pgx/v5"
)

// GrantOpts holds the parameters for granting privileges.
type GrantOpts struct {
	Privileges []string
	Database   string
	Schema     string
	Role       string
}

// GrantPrivileges grants privileges on a database and schema to a role.
// It runs three statements to cover PostgreSQL's permission model:
//  1. GRANT ... ON DATABASE ... TO ... (database-level connect/create)
//  2. GRANT ... ON ALL TABLES IN SCHEMA ... TO ... (existing tables)
//  3. ALTER DEFAULT PRIVILEGES ... GRANT ... ON TABLES TO ... (future tables)
func (c *Client) GrantPrivileges(ctx context.Context, opts GrantOpts) error {
	privs := strings.Join(opts.Privileges, ", ")
	quotedDB := pgx.Identifier{opts.Database}.Sanitize()
	quotedSchema := pgx.Identifier{opts.Schema}.Sanitize()
	quotedRole := pgx.Identifier{opts.Role}.Sanitize()

	// 1. Database-level grant
	stmt := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", privs, quotedDB, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("granting database privileges: %w", err)
	}

	// 2. Schema-level grant on all existing tables
	stmt = fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s",
		privs, quotedSchema, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("granting schema privileges: %w", err)
	}

	// 3. Default privileges for future tables
	stmt = fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT %s ON TABLES TO %s",
		quotedSchema, privs, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("setting default privileges: %w", err)
	}

	// 4. Grant USAGE on schema so the role can access objects in it
	stmt = fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", quotedSchema, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("granting schema usage: %w", err)
	}

	return nil
}

// RevokePrivileges revokes privileges previously granted.
func (c *Client) RevokePrivileges(ctx context.Context, opts GrantOpts) error {
	privs := strings.Join(opts.Privileges, ", ")
	quotedDB := pgx.Identifier{opts.Database}.Sanitize()
	quotedSchema := pgx.Identifier{opts.Schema}.Sanitize()
	quotedRole := pgx.Identifier{opts.Role}.Sanitize()

	// Revoke default privileges first
	stmt := fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s REVOKE %s ON TABLES FROM %s",
		quotedSchema, privs, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("revoking default privileges: %w", err)
	}

	// Revoke schema-level grants
	stmt = fmt.Sprintf("REVOKE %s ON ALL TABLES IN SCHEMA %s FROM %s",
		privs, quotedSchema, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("revoking schema privileges: %w", err)
	}

	// Revoke database-level grants
	stmt = fmt.Sprintf("REVOKE %s ON DATABASE %s FROM %s", privs, quotedDB, quotedRole)
	if _, err := c.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("revoking database privileges: %w", err)
	}

	return nil
}
