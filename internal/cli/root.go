// Package cli implements the go-crudgen command-line interface.
package cli

import (
	"fmt"
	"os"
)

// version is overridable at build time via -ldflags "-X go-crudgen/internal/cli.version=...".
var version = "0.0.0-dev"

const usage = `go-crudgen - generate RESTful Go services from an entity spec

Usage:
  go-crudgen <command> [flags]

Commands:
  generate    Generate a service from a YAML spec file
  version     Print the version

Run "go-crudgen <command> -h" for command-specific flags.
`

const (
	cmdGenerate     = "generate"
	cmdVersion      = "version"
	cmdVersionLong  = "--version"
	cmdVersionShort = "-v"
	cmdHelp         = "help"
	cmdHelpShort    = "-h"
	cmdHelpLong     = "--help"
)

// Run dispatches a subcommand and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case cmdGenerate:
		return runGenerate(rest)
	case cmdVersion, cmdVersionLong, cmdVersionShort:
		fmt.Println("go-crudgen", version)
		return 0
	case cmdHelp, cmdHelpShort, cmdHelpLong:
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}
