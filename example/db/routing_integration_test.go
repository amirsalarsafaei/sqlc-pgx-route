//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// These tests prove the generated PoolRouteQueries actually routes to the right
// pool, against real PostgreSQL. Routing is made observable by giving the
// "primary" and "replica" containers DIVERGENT data for the same row: a query
// that lands on the replica sees the replica's value, one that lands on the
// primary sees the primary's. (They are two independent databases, not a real
// streaming replica — that is exactly what makes the destination observable.)
//
// Run with: go test -tags integration ./...   (requires Docker)

func startPostgres(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctr, err := postgres.Run(ctx, "postgres:16",
		postgres.WithDatabase("app"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.WithInitScripts("../schema.sql"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func countAuthors(ctx context.Context, t *testing.T, pool *pgxpool.Pool, name string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM authors WHERE name = $1`, name).Scan(&n); err != nil {
		t.Fatalf("count authors %q: %v", name, err)
	}
	return n
}

func TestPoolRouteQueriesRouting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	primary := startPostgres(ctx, t)
	replica := startPostgres(ctx, t)

	// Fresh BIGSERIAL → both rows get id 1, but with different names. The
	// sequence sits at 1 on each, so the first CreateAuthor below gets id 2
	// (no PK clash with the seed).
	if _, err := primary.Exec(ctx, `INSERT INTO authors (name) VALUES ('from-primary')`); err != nil {
		t.Fatalf("seed primary: %v", err)
	}
	if _, err := replica.Exec(ctx, `INSERT INTO authors (name) VALUES ('from-replica')`); err != nil {
		t.Fatalf("seed replica: %v", err)
	}

	q := NewPoolRouteQueries(primary, replica)

	t.Run("plain SELECT routes to the replica", func(t *testing.T) {
		a, err := q.GetAuthor(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if a.Name != "from-replica" {
			t.Fatalf("GetAuthor read %q; expected from-replica (a read must hit the replica)", a.Name)
		}
	})

	t.Run("rw:write override sends a SELECT to the primary", func(t *testing.T) {
		a, err := q.GetAuthorFromPrimary(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if a.Name != "from-primary" {
			t.Fatalf("GetAuthorFromPrimary read %q; expected from-primary (rw:write override)", a.Name)
		}
	})

	t.Run("SELECT ... FOR UPDATE routes to the primary", func(t *testing.T) {
		a, err := q.LockAuthor(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if a.Name != "from-primary" {
			t.Fatalf("LockAuthor read %q; expected from-primary (locking read is a write)", a.Name)
		}
	})

	t.Run("INSERT routes to the primary only", func(t *testing.T) {
		if _, err := q.CreateAuthor(ctx, CreateAuthorParams{Name: "created-on-primary"}); err != nil {
			t.Fatal(err)
		}
		if got := countAuthors(ctx, t, primary, "created-on-primary"); got != 1 {
			t.Fatalf("primary has %d rows named created-on-primary; want 1", got)
		}
		if got := countAuthors(ctx, t, replica, "created-on-primary"); got != 0 {
			t.Fatalf("replica has %d rows named created-on-primary; want 0 (a write must never hit the replica)", got)
		}
	})

	t.Run("a read does not observe a primary-only write", func(t *testing.T) {
		authors, err := q.ListAuthors(ctx)
		if err != nil {
			t.Fatal(err)
		}
		names := make(map[string]bool, len(authors))
		for _, a := range authors {
			names[a.Name] = true
		}
		if !names["from-replica"] {
			t.Fatal("ListAuthors did not see replica data; expected it to read the replica")
		}
		if names["created-on-primary"] {
			t.Fatal("ListAuthors saw a primary-only write; a read must not hit the primary")
		}
	})
}
