// Command sqlc-pgx-route generates a read/write-routing wrapper around an
// sqlc-generated *Queries type.
//
// Run sqlc first (its built-in Go generator, unmodified), then run this tool
// pointed at the generated package directory:
//
//	sqlc generate
//	sqlc-pgx-route ./internal/db
//
// or via go:generate inside the generated package:
//
//	//go:generate sqlc-pgx-route .
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/amirsalarsafaei/sqlc-pgx-route/internal/wrapper"
)

func main() {
	cfg := wrapper.Config{}
	flag.StringVar(&cfg.OutFile, "out", "poolroute.go", "output file name (within the package dir)")
	flag.StringVar(&cfg.TypeName, "type", "PoolRouteQueries", "name of the generated wrapper type")
	flag.StringVar(&cfg.ConstructorName, "constructor", "NewPoolRouteQueries", "name of the generated constructor")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: sqlc-pgx-route [flags] [dir]\n\n")
		fmt.Fprintf(os.Stderr, "Reads the sqlc-generated package in dir (default \".\") and writes a\n")
		fmt.Fprintf(os.Stderr, "routing wrapper that sends reads to a replica and writes to the primary.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg.Dir = "."
	if flag.NArg() > 0 {
		cfg.Dir = flag.Arg(0)
	}

	out, err := wrapper.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlc-pgx-route: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", out)
}
