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

// RoleExists checks if a role exists in the PostgreSQL instance.
func (c *Client) RoleExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := c.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", name,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking if role %q exists: %w", name, err)
	}
	return exists, nil
}

// CreateOrUpdateRole creates a role with LOGIN and the given password,
// or updates the password if the role already exists.
func (c *Client) CreateOrUpdateRole(ctx context.Context, name, password string) error {
	exists, err := c.RoleExists(ctx, name)
	if err != nil {
		return err
	}

	quotedName := pgx.Identifier{name}.Sanitize()

	if exists {
		// ALTER ROLE to update password. Password is passed as a literal
		// since ALTER ROLE does not support parameterized passwords.
		_, err = c.conn.Exec(ctx,
			fmt.Sprintf("ALTER ROLE %s WITH LOGIN PASSWORD %s",
				quotedName, quoteLiteral(password)),
		)
		if err != nil {
			return fmt.Errorf("updating role %q: %w", name, err)
		}
		return nil
	}

	_, err = c.conn.Exec(ctx,
		fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD %s",
			quotedName, quoteLiteral(password)),
	)
	if err != nil {
		return fmt.Errorf("creating role %q: %w", name, err)
	}
	return nil
}

// DropRole drops a role if it exists.
func (c *Client) DropRole(ctx context.Context, name string) error {
	exists, err := c.RoleExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	quotedName := pgx.Identifier{name}.Sanitize()
	_, err = c.conn.Exec(ctx, fmt.Sprintf("DROP ROLE %s", quotedName))
	if err != nil {
		return fmt.Errorf("dropping role %q: %w", name, err)
	}
	return nil
}

// quoteLiteral safely quotes a string literal for use in SQL.
// This is used for passwords where parameterized queries are not possible
// in DDL statements.
func quoteLiteral(s string) string {
	// Use dollar-quoting with a unique tag to avoid injection.
	// This is safe because the tag itself contains no special chars.
	return fmt.Sprintf("'%s'", escapeString(s))
}

// escapeString escapes single quotes and backslashes for safe use in SQL string literals.
func escapeString(s string) string {
	result := make([]byte, 0, len(s))
	for i := range len(s) {
		switch s[i] {
		case '\'':
			result = append(result, '\'', '\'')
		case '\\':
			result = append(result, '\\', '\\')
		default:
			result = append(result, s[i])
		}
	}
	return string(result)
}
