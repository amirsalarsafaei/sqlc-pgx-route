# sqlc-pgx-route

Adds read/write query routing to [sqlc](https://sqlc.dev)-generated code for PGX
pools — **without forking sqlc**.

`sqlc-pgx-route` runs *after* sqlc's own (unmodified) Go generator. It reads the
generated package, classifies every query as a read or a write at code-generation
time using the real PostgreSQL parser, and emits one extra file, `poolroute.go`,
containing a `PoolRouteQueries` wrapper that routes:

- **Reads** (`SELECT`) → a read replica pool
- **Writes** (`INSERT`/`UPDATE`/`DELETE`, `SELECT … FOR UPDATE`, write-CTEs, `:copyfrom`, `:batch`) → the primary pool

The decision is baked in at build time, so there is **no runtime SQL parsing**.

> Earlier versions of this project were a full fork of `sqlc-gen-go`. It is now a
> small post-processor instead: you always use the latest sqlc, and this tool
> only adds the routing layer on top.

## How it works

```
sqlc generate            # built-in Go generator → db.go, models.go, query.sql.go
sqlc-pgx-route ./db      # reads that package    → db/poolroute.go
```

`sqlc-pgx-route` parses the already-generated Go with `go/ast`, so it reuses the
exact method signatures and types sqlc produced — nothing to keep in sync, no
fork to maintain. The generated wrapper **embeds the writer `*Queries`** (so it
satisfies `Querier` and sends writes / `:copyfrom` / `:batch` to the primary for
free) and overrides only the read methods to use the replica.

### Query classification

| SQL statement | Routed to | Reasoning |
|---|---|---|
| `SELECT …` | replica | pure read |
| `SELECT … FOR UPDATE` | primary | acquires row locks |
| `INSERT` / `UPDATE` / `DELETE` (incl. `RETURNING`) | primary | mutation |
| `WITH cte AS (INSERT …) SELECT …` | primary | CTE contains a mutation |
| `WITH cte AS (SELECT …) SELECT …` | replica | no mutations |
| `:copyfrom`, `:batch*` | primary | bulk write / can't split a batch |

Classification is shared with the runtime router in
[`pgx-router`](https://github.com/amirsalarsafaei/pgx-router) (the `classify`
package), so both agree on every query.

### Override annotations

Force a query to a specific pool with a leading SQL comment:

```sql
-- name: GetAuthorFromPrimary :one
-- rw:write
SELECT * FROM authors WHERE id = $1;
```

Both `rw:write` and `rw_mode:write` (and the `read` forms) are accepted. sqlc
relocates these comments onto the generated method's doc comment; the tool reads
them from there.

## Installation

```bash
go install github.com/amirsalarsafaei/sqlc-pgx-route/cmd/sqlc-pgx-route@latest
```

> Building the tool needs `CGO_ENABLED=1` and a C compiler — it uses
> `pg_query_go` (libpg_query) to parse SQL. Prebuilt binaries are attached to
> each release. The **generated code** has no such requirement; at runtime it
> depends only on pgx.

## Usage

`sqlc.yaml` — just sqlc's built-in Go generator, nothing special:

```yaml
version: '2'
sql:
  - engine: postgresql
    queries: query.sql
    schema: schema.sql
    gen:
      go:
        package: db
        out: db
        sql_package: pgx/v5
```

Wire both steps with `go:generate` (see [`example/gen.go`](example/gen.go)):

```go
//go:generate sqlc generate
//go:generate sqlc-pgx-route ./db
```

```bash
go generate ./...
```

Then route reads and writes through `PoolRouteQueries`:

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

	// writer first, reader second. Pass a nil reader to send everything to the writer.
	q := db.NewPoolRouteQueries(primary, replica)

	authors, _ := q.ListAuthors(ctx)        // → replica
	author, _ := q.CreateAuthor(ctx, db.CreateAuthorParams{Name: "Haruki"}) // → primary

	// A transaction is pinned to one connection, so WithTx runs everything on it:
	tx, _ := primary.Begin(ctx)
	qtx := q.WithTx(tx)
	_, _ = qtx.GetAuthor(ctx, author.ID)    // → tx
	tx.Commit(ctx)
}
```

`*PoolRouteQueries` satisfies the generated `Querier` interface, so it drops into
code already written against `*db.Queries`.

## Supported sqlc options

The tool reads whatever sqlc emits, so it adapts to the Go generator's options:

| Option | Handling |
|---|---|
| `emit_methods_with_db_argument` | Drops the `db DBTX` argument from each method and injects the routed pool per call (wrapper holds `writer`/`reader DBTX`). |
| `emit_interface` | `*PoolRouteQueries` satisfies the generated `Querier`. |
| `emit_prepared_queries` | The backing query const is still discovered in the prepared call. |
| `emit_exported_queries` | Classification keys off the SQL text, not the const name. |
| `emit_result_struct_pointers` / pointer params | Signatures are reproduced verbatim. |
| `build_tags` | The `//go:build` constraint is propagated to `poolroute.go`. |
| `output_*_file_name`, tags, JSON/DB tags | Transparent (the whole package is scanned). |

## Notes / limitations

- pgx only (`sql_package: pgx/v5`); routing read replicas is a pgx concept.
- The read replica is used as-is. If you also want automatic fallback to the
  primary when a replica rejects a read (e.g. a read-only transaction error),
  use [`pgx-router`](https://github.com/amirsalarsafaei/pgx-router)'s runtime
  `Pool` as the writer/reader `DBTX`.

## License

MIT
