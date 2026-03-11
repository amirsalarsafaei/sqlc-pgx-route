package main

import (
	"github.com/sqlc-dev/plugin-sdk-go/codegen"

	golang "github.com/amirsalarsafaei/sqlc-pgx-route/internal"
)

func main() {
	codegen.Run(golang.Generate)
}
