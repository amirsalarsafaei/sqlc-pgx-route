.PHONY: build test clean

build:
	go build -o bin/sqlc-gen-poolroute ./cmd/sqlc-gen-poolroute

test:
	go test ./...

clean:
	rm -rf bin/
