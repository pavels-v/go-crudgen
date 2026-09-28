package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"go-crudgen/internal/generator"
	"go-crudgen/internal/spec"
)

const (
	flagSpec   = "spec"
	flagOut    = "out"
	flagDryRun = "dry-run"
	flagDriver = "driver"
)

func runGenerate(args []string) int {
	fs := flag.NewFlagSet(cmdGenerate, flag.ContinueOnError)
	specPath := fs.String(flagSpec, "", "path to the YAML entity specification (required)")
	outDir := fs.String(flagOut, "", "output directory for generated code (default: write to stdout)")
	dryRun := fs.Bool(flagDryRun, false, "report what would be generated without writing files")
	driver := fs.String(flagDriver, generator.DriverPgx, "database driver for the generated NewDB constructor: pgx or pq")

	if err := fs.Parse(args); err != nil {
		// An explicit -h/--help is a success, not a usage error; flag has
		// already printed the usage text.
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "generate: -spec is required")
		fs.Usage()
		return 2
	}

	s, err := spec.Load(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load spec: %v\n", err)
		return 1
	}

	if err := generator.Generate(s, generator.Options{OutDir: *outDir, DryRun: *dryRun, Driver: *driver}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate: %v\n", err)
		return 1
	}
	return 0
}
