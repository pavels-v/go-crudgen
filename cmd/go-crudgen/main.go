// Command go-crudgen generates RESTful Go services from an entity specification.
package main

import (
	"os"

	"github.com/pavels-v/go-crudgen/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
