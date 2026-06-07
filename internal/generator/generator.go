// Package generator turns a validated spec into Go source files.
package generator

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"go-crudgen/internal/spec"
)

//go:embed templates/*.tmpl
var templates embed.FS

var tmpl = template.Must(template.ParseFS(templates, "templates/*.tmpl"))

// Options controls generation output.
type Options struct {
	OutDir string
	DryRun bool
}

// Generate produces a Go service from the spec. Currently it emits the model
// structs for each entity (see the roadmap in README.md for what's next).
//
// When opts.OutDir is empty the generated code is written to stdout; otherwise
// one file per entity is written into that directory. Progress and diagnostics
// always go to stderr so stdout carries only generated code.
func Generate(s *spec.Spec, opts Options) error {
	toStdout := opts.OutDir == ""

	dest := opts.OutDir
	if toStdout {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "go-crudgen: package %q, %d %s -> %s\n",
		s.Package, len(s.Entities), entityWord(len(s.Entities)), dest)
	for _, e := range s.Entities {
		fmt.Fprintf(os.Stderr, "  - %s (%d fields)\n", e.Name, len(e.Fields))
	}

	if opts.DryRun {
		fmt.Fprintln(os.Stderr, "(dry run: no files written)")
		return nil
	}

	if !toStdout {
		if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
			return fmt.Errorf("creating output dir: %w", err)
		}
	}

	byName := make(map[string]*spec.Entity, len(s.Entities))
	for i := range s.Entities {
		byName[s.Entities[i].Name] = &s.Entities[i]
	}

	for i := range s.Entities {
		e := &s.Entities[i]
		src, err := renderModel(s, e, byName)
		if err != nil {
			return fmt.Errorf("generating model for %q: %w", e.Name, err)
		}
		if toStdout {
			fmt.Fprintf(os.Stdout, "// file: %s.gen.go\n%s\n", snakeCase(e.Name), src)
			continue
		}
		path := filepath.Join(opts.OutDir, snakeCase(e.Name)+".gen.go")
		if err := os.WriteFile(path, src, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "  wrote %s\n", path)
	}
	return nil
}

// modelData is the template input for a single entity's model file.
type modelData struct {
	Package string
	Imports []string
	Struct  string
	Lower   string // struct name lower-cased, for the doc comment
	Fields  []modelField
}

type modelField struct {
	GoName string
	GoType string
	Tag    string
}

// renderModel builds, executes, and gofmt-formats the model file for one entity.
func renderModel(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) ([]byte, error) {
	name := pascalCase(e.Name)
	data := modelData{Package: s.Package, Struct: name, Lower: strings.ToLower(name)}
	imports := make(map[string]struct{})

	add := func(name, goExpr, imp string, tag string) {
		if imp != "" {
			imports[imp] = struct{}{}
		}
		data.Fields = append(data.Fields, modelField{GoName: pascalCase(name), GoType: goExpr, Tag: tag})
	}

	for _, f := range e.Fields {
		gt, err := fieldType(f, byName)
		if err != nil {
			return nil, err
		}
		add(f.Name, gt.expr, gt.imp, fieldTag(f))
	}

	if e.Options.Timestamps {
		add("created_at", "time.Time", "time", `json:"created_at"`)
		add("updated_at", "time.Time", "time", `json:"updated_at"`)
	}
	if e.Options.SoftDelete {
		add("deleted_at", "*time.Time", "time", `json:"deleted_at,omitempty"`)
	}

	for imp := range imports {
		data.Imports = append(data.Imports, imp)
	}
	sort.Strings(data.Imports)

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "model.go.tmpl", data); err != nil {
		return nil, fmt.Errorf("rendering template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated source: %w", err)
	}
	return formatted, nil
}

// fieldTag builds the struct tag for a field: a json name plus a validate rule
// that merges the `required` modifier with any explicit `validate` string.
func fieldTag(f spec.Field) string {
	tag := fmt.Sprintf("json:%q", f.Name)

	var rules []string
	if f.Required && !strings.Contains(f.Validate, "required") {
		rules = append(rules, "required")
	}
	if f.Validate != "" {
		rules = append(rules, f.Validate)
	}
	if len(rules) > 0 {
		tag += fmt.Sprintf(" validate:%q", strings.Join(rules, ","))
	}
	return tag
}

func entityWord(n int) string {
	if n == 1 {
		return "entity"
	}
	return "entities"
}
