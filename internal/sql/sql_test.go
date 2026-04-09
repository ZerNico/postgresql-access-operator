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
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testPassword = "testpassword"
	testUser     = "postgres"
	testDB       = "postgres"
)

// setupPostgres starts a real PostgreSQL container using testcontainers.
// This mirrors the mariadb-operator approach of testing against a real database.
func setupPostgres(t *testing.T) (*Client, func()) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(testDB),
		postgres.WithUsername(testUser),
		postgres.WithPassword(testPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	client, err := Connect(ctx, ConnectConfig{
		Host:     host,
		Port:     int32(port.Int()),
		Database: testDB,
		Username: testUser,
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	cleanup := func() {
		_ = client.Close(ctx)
		_ = pgContainer.Terminate(ctx)
	}

	return client, cleanup
}

func TestPing(t *testing.T) {
	client, cleanup := setupPostgres(t)
	defer cleanup()

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestCreateAndDropDatabase(t *testing.T) {
	client, cleanup := setupPostgres(t)
	defer cleanup()

	ctx := context.Background()
	dbName := "test_create_db"

	// Create database
	if err := client.CreateDatabase(ctx, dbName); err != nil {
		t.Fatalf("CreateDatabase failed: %v", err)
	}

	// Verify it exists
	exists, err := client.DatabaseExists(ctx, dbName)
	if err != nil {
		t.Fatalf("DatabaseExists failed: %v", err)
	}
	if !exists {
		t.Fatal("Database should exist after creation")
	}

	// Create again (idempotent)
	if err := client.CreateDatabase(ctx, dbName); err != nil {
		t.Fatalf("CreateDatabase (idempotent) failed: %v", err)
	}

	// Drop database
	if err := client.DropDatabase(ctx, dbName); err != nil {
		t.Fatalf("DropDatabase failed: %v", err)
	}

	// Verify it's gone
	exists, err = client.DatabaseExists(ctx, dbName)
	if err != nil {
		t.Fatalf("DatabaseExists after drop failed: %v", err)
	}
	if exists {
		t.Fatal("Database should not exist after drop")
	}

	// Drop again (idempotent)
	if err := client.DropDatabase(ctx, dbName); err != nil {
		t.Fatalf("DropDatabase (idempotent) failed: %v", err)
	}
}

func TestCreateAndDropRole(t *testing.T) {
	client, cleanup := setupPostgres(t)
	defer cleanup()

	ctx := context.Background()
	roleName := "test_role"
	password := "rolepassword123"

	// Create role
	if err := client.CreateOrUpdateRole(ctx, roleName, password); err != nil {
		t.Fatalf("CreateOrUpdateRole failed: %v", err)
	}

	// Verify it exists
	exists, err := client.RoleExists(ctx, roleName)
	if err != nil {
		t.Fatalf("RoleExists failed: %v", err)
	}
	if !exists {
		t.Fatal("Role should exist after creation")
	}

	// Update password (idempotent)
	if err := client.CreateOrUpdateRole(ctx, roleName, "newpassword456"); err != nil {
		t.Fatalf("CreateOrUpdateRole (update) failed: %v", err)
	}

	// Drop role
	if err := client.DropRole(ctx, roleName); err != nil {
		t.Fatalf("DropRole failed: %v", err)
	}

	// Verify it's gone
	exists, err = client.RoleExists(ctx, roleName)
	if err != nil {
		t.Fatalf("RoleExists after drop failed: %v", err)
	}
	if exists {
		t.Fatal("Role should not exist after drop")
	}

	// Drop again (idempotent)
	if err := client.DropRole(ctx, roleName); err != nil {
		t.Fatalf("DropRole (idempotent) failed: %v", err)
	}
}

func TestGrantAndRevokePrivileges(t *testing.T) {
	client, cleanup := setupPostgres(t)
	defer cleanup()

	ctx := context.Background()
	dbName := "test_grant_db"
	roleName := "test_grant_role"

	// Setup: create database and role
	if err := client.CreateDatabase(ctx, dbName); err != nil {
		t.Fatalf("CreateDatabase failed: %v", err)
	}
	if err := client.CreateOrUpdateRole(ctx, roleName, "grantpass"); err != nil {
		t.Fatalf("CreateOrUpdateRole failed: %v", err)
	}

	// Connect to the target database for schema-level grants
	host := client.conn.Config().Host
	port := int32(client.conn.Config().Port)

	dbClient, err := Connect(ctx, ConnectConfig{
		Host:     host,
		Port:     port,
		Database: dbName,
		Username: testUser,
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("Failed to connect to target database: %v", err)
	}
	defer func() { _ = dbClient.Close(ctx) }()

	// Grant privileges
	opts := GrantOpts{
		Privileges: []string{"ALL PRIVILEGES"},
		Database:   dbName,
		Schema:     "public",
		Role:       roleName,
	}
	if err := dbClient.GrantPrivileges(ctx, opts); err != nil {
		t.Fatalf("GrantPrivileges failed: %v", err)
	}

	// Grant again (idempotent)
	if err := dbClient.GrantPrivileges(ctx, opts); err != nil {
		t.Fatalf("GrantPrivileges (idempotent) failed: %v", err)
	}

	// Verify role can CREATE TABLE (PG 15+ requires explicit CREATE on schema)
	roleClient, err := Connect(ctx, ConnectConfig{
		Host:     host,
		Port:     port,
		Database: dbName,
		Username: roleName,
		Password: "grantpass",
	})
	if err != nil {
		t.Fatalf("Failed to connect as granted role: %v", err)
	}
	defer func() { _ = roleClient.Close(ctx) }()

	if _, err := roleClient.conn.Exec(ctx, "CREATE TABLE test_table (id serial PRIMARY KEY, name text)"); err != nil {
		t.Fatalf("Role should be able to CREATE TABLE after grant: %v", err)
	}
	if _, err := roleClient.conn.Exec(ctx, "INSERT INTO test_table (name) VALUES ('hello')"); err != nil {
		t.Fatalf("Role should be able to INSERT after grant: %v", err)
	}
	if _, err := roleClient.conn.Exec(ctx, "DROP TABLE test_table"); err != nil {
		t.Fatalf("Role should be able to DROP TABLE after grant: %v", err)
	}

	// Revoke privileges
	if err := dbClient.RevokePrivileges(ctx, opts); err != nil {
		t.Fatalf("RevokePrivileges failed: %v", err)
	}
}

func TestSpecialCharactersInPassword(t *testing.T) {
	client, cleanup := setupPostgres(t)
	defer cleanup()

	ctx := context.Background()
	roleName := "special_char_role"

	// Test with special characters that could cause SQL injection
	passwords := []string{
		"simple",
		"with'quote",
		"with\\backslash",
		"with'both\\chars",
		"p@ss!w0rd#$%^&*()",
		"multi'ple''quotes",
	}

	for _, pw := range passwords {
		t.Run(pw, func(t *testing.T) {
			if err := client.CreateOrUpdateRole(ctx, roleName, pw); err != nil {
				t.Fatalf("CreateOrUpdateRole with password %q failed: %v", pw, err)
			}
			exists, err := client.RoleExists(ctx, roleName)
			if err != nil {
				t.Fatalf("RoleExists failed: %v", err)
			}
			if !exists {
				t.Fatal("Role should exist")
			}
			// Drop for next iteration
			if err := client.DropRole(ctx, roleName); err != nil {
				t.Fatalf("DropRole failed: %v", err)
			}
		})
	}
}
