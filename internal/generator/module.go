package generator

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"golang.org/x/mod/modfile"

	"github.com/pavels-v/go-crudgen/internal/spec"
)

func findModuleRoot(outDir string) (string, error) {
	dir, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}

	for {
		_, err := os.Stat(filepath.Join(dir, fileGoMod))
		if err == nil {
			return dir, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("find go.mod: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}

		dir = parent
	}
}

func resolveModule(s *spec.Spec, outDir, modRoot string) error {
	if modRoot == "" {
		if s.Module == "" && outDir == "" {
			return errors.New("module is not set: set module in the spec or pass --out inside a Go module")
		}

		if s.Module == "" {
			return fmt.Errorf("no go.mod above %s to derive the module import path from: set module in the spec", outDir)
		}

		return nil
	}

	want, err := modulePath(outDir, modRoot)
	if err != nil {
		return err
	}

	if s.Module == "" {
		s.Module = want

		return nil
	}

	if s.Module != want {
		return fmt.Errorf("module %q does not match %q, the import path of %s under %s", s.Module, want, outDir, filepath.Join(modRoot, fileGoMod))
	}

	return nil
}

func modulePath(outDir, modRoot string) (string, error) {
	goMod := filepath.Join(modRoot, fileGoMod)

	data, err := os.ReadFile(goMod)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", goMod, err)
	}

	mod := modfile.ModulePath(data)
	if mod == "" {
		return "", fmt.Errorf("%s has no module directive", goMod)
	}

	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}

	rel, err := filepath.Rel(modRoot, absOut)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}

	return path.Join(mod, filepath.ToSlash(rel)), nil
}
