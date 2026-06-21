.PHONY: build test test-integration clean example

build:
	go build -o bin/sqlc-pgx-route ./cmd/sqlc-pgx-route

test:
	go test ./...

# Real-database routing tests: two Postgres containers (primary + replica) via
# testcontainers. Requires Docker.
test-integration:
	cd example && go test -tags integration ./...

# Regenerate the example: sqlc's built-in Go generator, then the routing wrapper.
example: build
	cd example && sqlc generate && ../bin/sqlc-pgx-route ./db

clean:
	rm -rf bin/
