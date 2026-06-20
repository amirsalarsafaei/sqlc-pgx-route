.PHONY: build test clean example

build:
	go build -o bin/sqlc-pgx-route ./cmd/sqlc-pgx-route

test:
	go test ./...

# Regenerate the example: sqlc's built-in Go generator, then the routing wrapper.
example: build
	cd example && sqlc generate && ../bin/sqlc-pgx-route ./db

clean:
	rm -rf bin/
