package wrapper

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// genAndBuild writes the given package files (name -> source) into a temp
// module under a db/ directory, runs the generator, compiles the result, and
// returns the generated poolroute.go source. Compilation failing fails the test
// — that is the strongest guarantee the wrapper is correct for the layout.
func genAndBuild(t *testing.T, files map[string]string, tags ...string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module wraptest\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(root, "db")
	if err := os.Mkdir(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Run(Config{Dir: pkgDir}); err != nil {
		t.Fatalf("generator: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(pkgDir, "poolroute.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}

	buildArgs := []string{"build"}
	if len(tags) > 0 {
		buildArgs = append(buildArgs, "-tags", strings.Join(tags, ","))
	}
	buildArgs = append(buildArgs, "./...")
	cmd := exec.Command("go", buildArgs...)
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated package did not compile: %v\n%s\n--- poolroute.go ---\n%s", err, b, out)
	}
	return string(out)
}

func mustContain(t *testing.T, src, want string) {
	t.Helper()
	if !strings.Contains(src, want) {
		t.Errorf("generated source missing %q\n--- got ---\n%s", want, src)
	}
}

func mustNotContain(t *testing.T, src, notWant string) {
	t.Helper()
	if strings.Contains(src, notWant) {
		t.Errorf("generated source unexpectedly contains %q\n--- got ---\n%s", notWant, src)
	}
}

// standardDB is db.go for the default sqlc layout (stored connection).
const standardDB = `package db

import "context"

type DBTX interface {
	Exec(context.Context, string, ...any) error
	Query(context.Context, string, ...any) error
	QueryRow(context.Context, string, ...any) error
}

func New(db DBTX) *Queries { return &Queries{db: db} }

type Queries struct{ db DBTX }

func (q *Queries) WithTx(tx DBTX) *Queries { return &Queries{db: tx} }

type Author struct {
	ID   int64
	Name string
}

type CreateAuthorParams struct{ Name string }
`

func TestEmbeddedMode(t *testing.T) {
	query := `package db

import "context"

const getAuthor = ` + "`" + `-- name: GetAuthor :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

func (q *Queries) GetAuthor(ctx context.Context, id int64) (Author, error) {
	_ = q.db.QueryRow(ctx, getAuthor, id)
	return Author{}, nil
}

const listAuthors = ` + "`" + `-- name: ListAuthors :many
SELECT id, name FROM authors` + "`" + `

func (q *Queries) ListAuthors(ctx context.Context) ([]Author, error) {
	_ = q.db.Query(ctx, listAuthors)
	return nil, nil
}

const createAuthor = ` + "`" + `-- name: CreateAuthor :one
INSERT INTO authors (name) VALUES ($1) RETURNING id, name` + "`" + `

func (q *Queries) CreateAuthor(ctx context.Context, arg CreateAuthorParams) (Author, error) {
	_ = q.db.QueryRow(ctx, createAuthor, arg.Name)
	return Author{}, nil
}

const lockAuthor = ` + "`" + `-- name: LockAuthor :one
SELECT id, name FROM authors WHERE id = $1 FOR UPDATE` + "`" + `

func (q *Queries) LockAuthor(ctx context.Context, id int64) (Author, error) {
	_ = q.db.QueryRow(ctx, lockAuthor, id)
	return Author{}, nil
}

const getAuthorPrimary = ` + "`" + `-- name: GetAuthorPrimary :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

// rw:write
func (q *Queries) GetAuthorPrimary(ctx context.Context, id int64) (Author, error) {
	_ = q.db.QueryRow(ctx, getAuthorPrimary, id)
	return Author{}, nil
}
`
	// An emit_interface Querier plus a compile-time assertion that the wrapper
	// satisfies it.
	querier := `package db

import "context"

type Querier interface {
	GetAuthor(ctx context.Context, id int64) (Author, error)
	ListAuthors(ctx context.Context) ([]Author, error)
	CreateAuthor(ctx context.Context, arg CreateAuthorParams) (Author, error)
	LockAuthor(ctx context.Context, id int64) (Author, error)
	GetAuthorPrimary(ctx context.Context, id int64) (Author, error)
}

var _ Querier = (*PoolRouteQueries)(nil)
`

	src := genAndBuild(t, map[string]string{
		"db.go":        standardDB,
		"query.sql.go": query,
		"querier.go":   querier,
	})

	// Pure reads are overridden to the replica.
	mustContain(t, src, "func (q *PoolRouteQueries) GetAuthor(ctx context.Context, id int64) (Author, error)")
	mustContain(t, src, "return q.reader.GetAuthor(ctx, id)")
	mustContain(t, src, "func (q *PoolRouteQueries) ListAuthors(ctx context.Context) ([]Author, error)")
	// Writes, FOR UPDATE, and rw:write overrides are NOT overridden (inherited writer).
	mustNotContain(t, src, "CreateAuthor(ctx context.Context")
	mustNotContain(t, src, "LockAuthor(ctx context.Context")
	mustNotContain(t, src, "GetAuthorPrimary(ctx context.Context")
	// Embedded-writer shape + WithTx override.
	mustContain(t, src, "*Queries")
	mustContain(t, src, "reader *Queries")
	mustContain(t, src, "func (q *PoolRouteQueries) WithTx(tx DBTX) *PoolRouteQueries")
}

func TestDBArgMode(t *testing.T) {
	db := `package db

import "context"

type DBTX interface {
	Exec(context.Context, string, ...any) error
	Query(context.Context, string, ...any) error
	QueryRow(context.Context, string, ...any) error
}

func New() *Queries { return &Queries{} }

type Queries struct{}

type Author struct {
	ID   int64
	Name string
}

type CreateAuthorParams struct{ Name string }
`
	query := `package db

import "context"

const getAuthor = ` + "`" + `-- name: GetAuthor :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

func (q *Queries) GetAuthor(ctx context.Context, db DBTX, id int64) (Author, error) {
	_ = db.QueryRow(ctx, getAuthor, id)
	return Author{}, nil
}

const createAuthor = ` + "`" + `-- name: CreateAuthor :one
INSERT INTO authors (name) VALUES ($1) RETURNING id, name` + "`" + `

func (q *Queries) CreateAuthor(ctx context.Context, db DBTX, arg CreateAuthorParams) (Author, error) {
	_ = db.QueryRow(ctx, createAuthor, arg.Name)
	return Author{}, nil
}
`

	src := genAndBuild(t, map[string]string{
		"db.go":        db,
		"query.sql.go": query,
	})

	// Every method is wrapped, the db argument is dropped, and the routed pool
	// is injected.
	mustContain(t, src, "writer DBTX")
	mustContain(t, src, "reader DBTX")
	mustContain(t, src, "New()")
	mustContain(t, src, "func (q *PoolRouteQueries) GetAuthor(ctx context.Context, id int64) (Author, error)")
	mustContain(t, src, "return q.Queries.GetAuthor(ctx, q.reader, id)")
	mustContain(t, src, "func (q *PoolRouteQueries) CreateAuthor(ctx context.Context, arg CreateAuthorParams) (Author, error)")
	mustContain(t, src, "return q.Queries.CreateAuthor(ctx, q.writer, arg)")
	// The db argument must not survive in the wrapper signatures.
	mustNotContain(t, src, "db DBTX, id int64")
}

func TestExportedConsts(t *testing.T) {
	// emit_exported_queries: const names are exported. Classification keys off
	// the SQL value, not the const name, so this must still work.
	query := `package db

import "context"

const GetAuthor = ` + "`" + `-- name: GetAuthor :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

func (q *Queries) GetAuthor(ctx context.Context, id int64) (Author, error) {
	_ = q.db.QueryRow(ctx, GetAuthor, id)
	return Author{}, nil
}
`
	src := genAndBuild(t, map[string]string{
		"db.go":        standardDB,
		"query.sql.go": query,
	})
	mustContain(t, src, "func (q *PoolRouteQueries) GetAuthor(ctx context.Context, id int64) (Author, error)")
	mustContain(t, src, "return q.reader.GetAuthor(ctx, id)")
}

func TestPreparedCallShape(t *testing.T) {
	// emit_prepared_queries: the query runs through q.queryRow(ctx, stmt, sql, …).
	// The SQL const must still be discovered among the call arguments.
	db := `package db

import "context"

type DBTX interface {
	Exec(context.Context, string, ...any) error
	Query(context.Context, string, ...any) error
	QueryRow(context.Context, string, ...any) error
}

type stmt struct{}

func New(db DBTX) *Queries { return &Queries{db: db} }

type Queries struct {
	db             DBTX
	getAuthorStmt  *stmt
}

func (q *Queries) queryRow(ctx context.Context, s *stmt, sql string, args ...any) error { return nil }

type Author struct {
	ID   int64
	Name string
}
`
	query := `package db

import "context"

const getAuthor = ` + "`" + `-- name: GetAuthor :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

func (q *Queries) GetAuthor(ctx context.Context, id int64) (Author, error) {
	_ = q.queryRow(ctx, q.getAuthorStmt, getAuthor, id)
	return Author{}, nil
}
`
	src := genAndBuild(t, map[string]string{
		"db.go":        db,
		"query.sql.go": query,
	})
	mustContain(t, src, "func (q *PoolRouteQueries) GetAuthor(ctx context.Context, id int64) (Author, error)")
	mustContain(t, src, "return q.reader.GetAuthor(ctx, id)")
}

func TestPointerResults(t *testing.T) {
	// emit_result_struct_pointers: read returns *Author. The signature must be
	// reproduced verbatim.
	query := `package db

import "context"

const getAuthor = ` + "`" + `-- name: GetAuthor :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

func (q *Queries) GetAuthor(ctx context.Context, id int64) (*Author, error) {
	_ = q.db.QueryRow(ctx, getAuthor, id)
	return nil, nil
}
`
	src := genAndBuild(t, map[string]string{
		"db.go":        standardDB,
		"query.sql.go": query,
	})
	mustContain(t, src, "func (q *PoolRouteQueries) GetAuthor(ctx context.Context, id int64) (*Author, error)")
}

func TestBuildTagPropagated(t *testing.T) {
	// build_tags: sqlc puts a //go:build constraint on every file; the wrapper
	// must carry it too or it would compile in the wrong configurations.
	query := `//go:build custom

package db

import "context"

const getAuthor = ` + "`" + `-- name: GetAuthor :one
SELECT id, name FROM authors WHERE id = $1` + "`" + `

func (q *Queries) GetAuthor(ctx context.Context, id int64) (Author, error) {
	_ = q.db.QueryRow(ctx, getAuthor, id)
	return Author{}, nil
}
`
	dbTagged := "//go:build custom\n\n" + standardDB
	src := genAndBuild(t, map[string]string{
		"db.go":        dbTagged,
		"query.sql.go": query,
	}, "custom")
	mustContain(t, src, "//go:build custom")
}
