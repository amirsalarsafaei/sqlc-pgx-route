// Package example demonstrates wiring sqlc with sqlc-pgx-route.
//
// After installing both tools, run `go generate ./...`:
//
//	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
//	go install github.com/amirsalarsafaei/sqlc-pgx-route/cmd/sqlc-pgx-route@latest
//	go generate ./...
//
// sqlc emits db.go / models.go / query.sql.go; sqlc-pgx-route then reads that
// package and writes db/poolroute.go.
//
//go:generate sqlc generate
//go:generate sqlc-pgx-route ./db
package example
