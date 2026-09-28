package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/pavels-v/go-crudgen/internal/generator"
	"github.com/pavels-v/go-crudgen/internal/spec"
)

const (
	flagSpec     = "spec"
	flagOut      = "out"
	flagDryRun   = "dry-run"
	flagDriver   = "driver"
	flagNoRouter = "no-router"
	flagMain     = "main"
	flagNoMain   = "no-main"
	flagNoTests  = "no-tests"
)

const envSourceDateEpoch = "SOURCE_DATE_EPOCH"

func runGenerate(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmdGenerate, flag.ContinueOnError)
	fs.SetOutput(stderr)

	specPath := fs.String(flagSpec, "", "path to the YAML entity specification (required)")
	outDir := fs.String(flagOut, "", "output directory for generated code (default: write to stdout)")
	dryRun := fs.Bool(flagDryRun, false, "report what would be generated without writing files")
	driver := fs.String(flagDriver, generator.DriverPgx, "database driver for the generated NewDB constructor: pgx or pq")
	noRouter := fs.Bool(flagNoRouter, false, "skip restapi.NewRouter and Deps; wire the routes yourself")
	mainDir := fs.String(flagMain, "", "directory for main.go (default: cmd/<package> next to the nearest go.mod)")
	noMain := fs.Bool(flagNoMain, false, "skip main.go and migrations/embed.go")
	noTests := fs.Bool(flagNoTests, false, "skip restapi handler tests and their fake repository")

	if err := fs.Parse(args); err != nil {
		// An explicit -h/--help is a success, not a usage error; flag has
		// already printed the usage text.
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}

		return exitUsage
	}

	if *specPath == "" {
		fmt.Fprintln(stderr, "generate: -spec is required")
		fs.Usage()

		return exitUsage
	}

	if *noMain && *mainDir != "" {
		fmt.Fprintln(stderr, "generate: -main and -no-main are mutually exclusive")
		fs.Usage()

		return exitUsage
	}

	s, err := spec.Load(*specPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load spec: %v\n", err)
		return exitError
	}

	migrationTime, err := sourceDateEpoch()
	if err != nil {
		fmt.Fprintf(stderr, "failed to read %s: %v\n", envSourceDateEpoch, err)
		return exitUsage
	}

	opts := generator.Options{OutDir: *outDir, DryRun: *dryRun, Driver: *driver, Router: !*noRouter, Main: !*noMain, MainDir: *mainDir, Tests: !*noTests, MigrationTime: migrationTime}
	if err := generator.Generate(s, opts); err != nil {
		fmt.Fprintf(stderr, "failed to generate: %v\n", err)
		return exitError
	}

	return exitOK
}

func sourceDateEpoch() (time.Time, error) {
	v, ok := os.LookupEnv(envSourceDateEpoch)
	if !ok {
		return time.Time{}, nil
	}

	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse unix seconds: %w", err)
	}

	return time.Unix(sec, 0), nil
}
