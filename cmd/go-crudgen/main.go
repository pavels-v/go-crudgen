// Command go-crudgen generates RESTful Go services from an entity specification.
package main

import (
	"os"

	"go-crudgen/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
