// Package generator turns a validated spec into Go source files.
package generator

import (
	"fmt"

	"go-crudgen/internal/spec"
)

// Options controls generation output.
type Options struct {
	OutDir string
	DryRun bool
}

// Generate produces a Go service from the spec.
//
// NOTE: file emission is not implemented yet. This currently reports the plan
// so the CLI and spec pipeline can be exercised end-to-end. Use -dry-run until
// emission lands (see the roadmap in README.md).
func Generate(s *spec.Spec, opts Options) error {
	fmt.Printf("go-crudgen: package %q, %d %s -> %s\n",
		s.Package, len(s.Entities), entityWord(len(s.Entities)), opts.OutDir)
	for _, e := range s.Entities {
		fmt.Printf("  - %s (%d fields)\n", e.Name, len(e.Fields))
	}

	if opts.DryRun {
		fmt.Println("(dry run: no files written)")
		return nil
	}
	return fmt.Errorf("code generation not implemented yet; re-run with -dry-run")
}

func entityWord(n int) string {
	if n == 1 {
		return "entity"
	}
	return "entities"
}
