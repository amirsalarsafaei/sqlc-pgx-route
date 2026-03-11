# sqlc-pgx-route

An sqlc code generator (fork of [sqlc-gen-go](https://github.com/sqlc-dev/sqlc-gen-go)) that adds read/write query routing for PGX Pools.

## Overview

This plugin **replaces** the standard `sqlc-gen-go` code generator. It generates all the standard sqlc Go output (db.go, models.go, query.sql.go, etc.) **plus** a `poolroute.go` file with a `PoolRouteQueries` wrapper that automatically routes:
- **Read queries** (SELECT) → to a read replica pool
- **Write queries** (INSERT, UPDATE, DELETE, SELECT FOR UPDATE) → to the primary pool

This is similar to Django's `DATABASE_ROUTERS` with `db_for_read`/`db_for_write`, but determined at **code generation time** using the PostgreSQL parser, *not at runtime*.

## How it works

- Drop-in replacement for `sqlc-gen-go` with all the same options
- Uses the real PostgreSQL parser (`pg_query_go`) to classify each query
- When `emit_poolroute: true` is set, generates an additional `poolroute.go` with a thin `PoolRouteQueries` wrapper
- Each method delegates to the correct pool — no runtime SQL parsing needed

### Query Classification

| SQL Statement | Classification | Reasoning |
|---|---|---|
| `SELECT ...` | Read | Pure read |
| `SELECT ... FOR UPDATE` | Write | Acquires row locks |
| `INSERT ...` | Write | Mutation |
| `INSERT ... RETURNING *` | Write | Mutation (even with RETURNING) |
| `UPDATE ...` | Write | Mutation |
| `DELETE ...` | Write | Mutation |
| `WITH cte AS (INSERT ...) SELECT ...` | Write | CTE contains mutation |
| `WITH cte AS (SELECT ...) SELECT ...` | Read | No mutations |

### Override Annotations

Force a query to use a specific pool with comments:

```sql
-- name: GetAuthorFromPrimary :one
-- rw_mode:write
SELECT * FROM authors WHERE id = $1;
```

Both `rw_mode:write` and the shorthand `rw:write` are accepted.

## Installation

```bash
go install github.com/amirsalarsafaei/sqlc-pgx-routepool/cmd/sqlc-gen-poolroute@latest
```

## Configuration

This plugin replaces `sqlc-gen-go` — use it as the sole code generator:

```yaml
version: '2'
plugins:
  - name: poolroute
    process:
      cmd: sqlc-gen-poolroute
sql:
  - engine: postgresql
    queries: query.sql
    schema: schema.sql
    codegen:
      - plugin: poolroute
        out: db
        options:
          package: db
          sql_package: pgx/v5
          emit_poolroute: true
```

All standard `sqlc-gen-go` options are supported (`emit_json_tags`, `emit_interface`, `emit_db_tags`, `overrides`, etc.). The only addition is `emit_poolroute: true` which enables the `poolroute.go` generation.

## Usage

```go
package main

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
    "yourapp/db"
)

func main() {
    ctx := context.Background()
    
    primary, _ := pgxpool.New(ctx, "postgres://primary:5432/mydb")
    replica, _ := pgxpool.New(ctx, "postgres://replica:5432/mydb")
    
    // PoolRouteQueries routes reads to replica, writes to primary
    queries := db.NewPoolRouteQueries(primary, replica)
    
    // This goes to the replica
    authors, _ := queries.ListAuthors(ctx)
    
    // This goes to the primary  
    author, _ := queries.CreateAuthor(ctx, db.CreateAuthorParams{
        Name: "Haruki",
        Bio:  pgtype.Text{String: "Author", Valid: true},
    })
    
    // Transactions always use the primary
    tx, _ := primary.Begin(ctx)
    txQueries := queries.WithTx(tx)
    txQueries.GetAuthor(ctx, author.ID)
    txQueries.DeleteAuthor(ctx, author.ID)
    tx.Commit(ctx)
}
```

## How it differs from other approaches

| Approach | When routing decided | Handles edge cases | Maintenance |
|---|---|---|---|
| **This plugin** | Code generation time | ✅ Uses PG parser | Auto-generated |
| Runtime SQL inspection | Every query execution | ❌ String matching | Manual |
| Manual routing | Developer decides | ✅ Full control | Manual |
| GORM dbresolver | Runtime (ORM callbacks) | ⚠️ ORM-specific | GORM-only |

## License

MIT
