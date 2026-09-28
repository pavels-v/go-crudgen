// Package cli implements the go-crudgen command-line interface.
package cli

import (
	"fmt"
	"io"
	"runtime/debug"
)

// version is overridable at build time via -ldflags "-X github.com/pavels-v/go-crudgen/internal/cli.version=...".
var version = devVersion

const (
	devVersion   = "0.0.0-dev"
	develVersion = "(devel)"
)

const usage = `go-crudgen - generate RESTful Go services from an entity spec

Usage:
  go-crudgen <command> [flags]

Commands:
  generate    Generate a service from a YAML spec file
  version     Print the version

Run "go-crudgen <command> -h" for command-specific flags.
`

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

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
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case cmdGenerate:
		return runGenerate(rest, stderr)
	case cmdVersion, cmdVersionLong, cmdVersionShort:
		_, _ = fmt.Fprintln(stdout, "go-crudgen", resolveVersion())
		return exitOK
	case cmdHelp, cmdHelpShort, cmdHelpLong:
		_, _ = fmt.Fprint(stdout, usage)
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return exitUsage
	}
}

func resolveVersion() string {
	if version != devVersion {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == develVersion {
		return version
	}

	return info.Main.Version
}
