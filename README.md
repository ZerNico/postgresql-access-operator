# postgresql-access-operator

A Kubernetes operator that manages **databases, roles, and grants** on existing PostgreSQL instances. Apps self-service their database access by creating CRDs in their own namespace -- the operator connects to a shared PostgreSQL server and executes the SQL.

This operator does **not** deploy or manage PostgreSQL servers. It is the PostgreSQL equivalent of the [mariadb-operator](https://github.com/mariadb-operator/mariadb-operator)'s Database/User/Grant CRDs.

## How it works

```
postgresql namespace (central)          app-staging namespace
+-------------------------------------+  +---------------------------+
| postgresql-access-operator           |  | PostgreSQLDatabase CR     |
| PostgreSQL CR: postgresql-staging ---+--+-- postgresRef             |
| PostgreSQL CR: postgresql-prod       |  | PostgreSQLUser CR         |
+-------------------------------------+  | PostgreSQLGrant CR        |
                                          | Secret (password)         |
                                          +---------------------------+
```

1. The **platform team** deploys PostgreSQL servers (e.g. via Bitnami Helm chart) and registers them as `PostgreSQL` CRs
2. **App teams** create `PostgreSQLDatabase`, `PostgreSQLUser`, and `PostgreSQLGrant` CRs in their own namespace
3. The **operator** resolves the cross-namespace reference, connects as superuser, and runs the SQL

## CRDs

| CRD | Purpose | SQL |
|-----|---------|-----|
| `PostgreSQL` | Reference to an existing PostgreSQL server (connection details) | `SELECT 1` (connectivity check) |
| `PostgreSQLDatabase` | Creates a database | `CREATE DATABASE` |
| `PostgreSQLUser` | Creates a login role, reads password from a Secret | `CREATE ROLE ... LOGIN PASSWORD` |
| `PostgreSQLGrant` | Grants privileges on a database + schema to a role | `GRANT ... ON DATABASE`, `GRANT ... ON ALL TABLES IN SCHEMA`, `ALTER DEFAULT PRIVILEGES` |

## Quick start

### 1. Register a PostgreSQL instance

```yaml
apiVersion: db.zernico.de/v1alpha1
kind: PostgreSQL
metadata:
  name: postgresql-staging
  namespace: postgresql
spec:
  host: "postgresql-staging.postgresql.svc.cluster.local"
  port: 5432
  superuserSecretKeyRef:
    name: postgresql-staging-root
    key: password
```

### 2. Create a database, user, and grant in your app namespace

```yaml
# Secret with the app's database password
apiVersion: v1
kind: Secret
metadata:
  name: my-app-db
  namespace: myapp-staging
type: Opaque
data:
  password: <base64-encoded-password>
---
# Database
apiVersion: db.zernico.de/v1alpha1
kind: PostgreSQLDatabase
metadata:
  name: my-app
  namespace: myapp-staging
spec:
  postgresRef:
    name: postgresql-staging
    namespace: postgresql
  name: "myapp_staging"
  cleanupPolicy: Skip
---
# User (role with login)
apiVersion: db.zernico.de/v1alpha1
kind: PostgreSQLUser
metadata:
  name: my-app
  namespace: myapp-staging
spec:
  postgresRef:
    name: postgresql-staging
    namespace: postgresql
  name: "myapp_staging"
  passwordSecretKeyRef:
    name: my-app-db
    key: password
  cleanupPolicy: Skip
---
# Grant
apiVersion: db.zernico.de/v1alpha1
kind: PostgreSQLGrant
metadata:
  name: my-app
  namespace: myapp-staging
spec:
  postgresRef:
    name: postgresql-staging
    namespace: postgresql
  privileges:
    - "ALL PRIVILEGES"
  database: "myapp_staging"
  schema: "public"
  role: "myapp_staging"
  cleanupPolicy: Skip
```

### 3. Connect from your app

```yaml
env:
  - name: DB_HOST
    value: "postgresql-staging.postgresql.svc.cluster.local"
  - name: DB_PORT
    value: "5432"
  - name: DB_NAME
    value: "myapp_staging"
  - name: DB_USER
    value: "myapp_staging"
  - name: DB_PASSWORD
    valueFrom:
      secretKeyRef:
        name: my-app-db
        key: password
```

## Key features

- **Cross-namespace references** -- CRDs in app namespaces reference PostgreSQL instances in a central namespace
- **cleanupPolicy: Skip** (default) -- deleting a CR does NOT drop the database/role/grant. Set to `Delete` to enable cleanup
- **Continuous reconciliation** -- drift detection with configurable `requeueInterval` (default 30s) and jitter
- **Per-resource retry intervals** -- configure `retryInterval` on each CR for error backoff
- **Finalizer safety** -- finalizers are only added after the first successful SQL operation. If SQL never succeeds, deletion is not blocked. If the PostgreSQL CR is deleted, finalizers are removed gracefully
- **Identifier safety** -- all SQL identifiers are quoted via `pgx.Identifier{}.Sanitize()` to prevent injection

## Installation

The Helm charts are published to **two channels** on every release. Pick whichever your tooling prefers.

### Via Helm (OCI, recommended)

Pulls directly from GHCR -- no `helm repo add` needed.

```bash
# CRDs
helm install postgresql-access-operator-crds \
  oci://ghcr.io/zernico/charts/postgresql-access-operator-crds \
  --version 0.1.1

# Operator
helm install postgresql-access-operator \
  oci://ghcr.io/zernico/charts/postgresql-access-operator \
  --version 0.1.1 \
  --namespace postgresql \
  --create-namespace \
  --set clusterWide=true
```

Requires Helm 3.8+.

### Via Helm (classic chart repository)

```bash
helm repo add postgresql-access-operator https://zernico.github.io/postgresql-access-operator
helm repo update
helm install postgresql-access-operator-crds postgresql-access-operator/postgresql-access-operator-crds
helm install postgresql-access-operator postgresql-access-operator/postgresql-access-operator \
  --namespace postgresql \
  --create-namespace \
  --set clusterWide=true
```

### Via kustomize

```bash
make install      # Install CRDs
make deploy IMG=ghcr.io/zernico/postgresql-access-operator:0.1.1
```

## Development

### Prerequisites

- Go 1.24+
- Docker (for testcontainers)
- kubectl + access to a Kubernetes cluster (for e2e only)

### Build

```bash
make generate    # Regenerate deepcopy & manifests
make manifests   # Regenerate CRD & RBAC YAML
go build -o bin/manager cmd/main.go
```

### Test

```bash
# API type unit tests (no Docker needed)
go test ./api/v1alpha1/...

# SQL client tests against real PostgreSQL (needs Docker)
go test ./internal/sql/...

# Controller integration tests with envtest + testcontainers (needs Docker)
make setup-envtest
go test ./internal/controller/...

# All tests
make test
```

### Docker

```bash
make docker-build IMG=ghcr.io/zernico/postgresql-access-operator:0.1.0
make docker-push IMG=ghcr.io/zernico/postgresql-access-operator:0.1.0
```

## Project structure

```
api/v1alpha1/              CRD type definitions
internal/controller/       Reconciliation logic for all 4 controllers
internal/sql/              pgx-based SQL client (CREATE/DROP/GRANT/REVOKE)
config/crd/bases/          Generated CRD manifests
config/rbac/               Generated RBAC (ClusterRole with Secrets + CRD access)
config/samples/            Example CRs
charts/crds/               Helm chart: CRDs only
charts/operator/           Helm chart: operator deployment
```

## API reference

### PostgreSQL

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.host` | string | (required) | Hostname or IP of the PostgreSQL server |
| `spec.port` | int32 | `5432` | TCP port |
| `spec.database` | string | `"postgres"` | Admin database to connect to |
| `spec.superuserUsername` | string | `"postgres"` | Superuser username |
| `spec.superuserSecretKeyRef` | SecretKeyRef | (required) | Reference to the superuser password Secret |

### PostgreSQLDatabase

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.postgresRef` | PostgresRef | (required) | Cross-namespace reference to a PostgreSQL CR |
| `spec.name` | string | `metadata.name` | Database name in PostgreSQL |
| `spec.cleanupPolicy` | Skip/Delete | `Skip` | Whether to DROP DATABASE on CR deletion |
| `spec.requeueInterval` | duration | `30s` | Drift detection interval |
| `spec.retryInterval` | duration | `5s` | Error retry interval |

### PostgreSQLUser

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.postgresRef` | PostgresRef | (required) | Cross-namespace reference to a PostgreSQL CR |
| `spec.name` | string | `metadata.name` | Role name in PostgreSQL |
| `spec.passwordSecretKeyRef` | SecretKeyRef | (required) | Reference to the password Secret (same namespace) |
| `spec.cleanupPolicy` | Skip/Delete | `Skip` | Whether to DROP ROLE on CR deletion |

### PostgreSQLGrant

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.postgresRef` | PostgresRef | (required) | Cross-namespace reference to a PostgreSQL CR |
| `spec.privileges` | []string | (required) | Privileges to grant (e.g. `["ALL PRIVILEGES"]`) |
| `spec.database` | string | (required) | Target database |
| `spec.schema` | string | `"public"` | Target schema |
| `spec.role` | string | (required) | Role to grant to |
| `spec.cleanupPolicy` | Skip/Delete | `Skip` | Whether to REVOKE on CR deletion |

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
