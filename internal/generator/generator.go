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
	Driver string // database driver for the connection constructor: "pgx" (default) or "pq"
}

// Generate produces a Go service from the spec. It emits a model file per
// entity, a CRUD handler file for each serveable entity, and a package-wide
// http.gen.go with the router and shared helpers (see the roadmap in README.md
// for what's next).
//
// When opts.OutDir is empty the generated code is written to stdout; otherwise
// one file per entity is written into that directory. Progress and diagnostics
// always go to stderr so stdout carries only generated code.
func Generate(s *spec.Spec, opts Options) error {
	driverName, driverImp, ok := driverInfo(opts.Driver)
	if !ok {
		return fmt.Errorf("unknown driver %q (supported: pgx, pq)", opts.Driver)
	}

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

	// Migrations are numbered so that a referenced table is created before the
	// table whose foreign key points at it, regardless of declaration order.
	order, err := migrationOrder(s.Entities, byName)
	if err != nil {
		return err
	}
	migNum := make(map[string]int, len(order))
	for i, e := range order {
		migNum[e.Name] = i + 1
	}

	emit := func(file string, src []byte) error {
		if toStdout {
			fmt.Fprintf(os.Stdout, "// file: %s\n%s\n", file, src)
			return nil
		}
		path := filepath.Join(opts.OutDir, file)
		if dir := filepath.Dir(path); dir != opts.OutDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", dir, err)
			}
		}
		if err := os.WriteFile(path, src, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "  wrote %s\n", path)
		return nil
	}

	var serveable []sharedEntity
	var anyNullable bool
	for i := range s.Entities {
		e := &s.Entities[i]
		base := snakeCase(e.Name)

		src, err := renderModel(s, e, byName)
		if err != nil {
			return fmt.Errorf("generating model for %q: %w", e.Name, err)
		}
		if err := emit(base+".gen.go", src); err != nil {
			return err
		}

		// Every entity needs a table, including ones whose primary-key type gets no
		// handler or repository. Migrations are numbered in dependency order (see
		// migrationOrder) so foreign keys resolve when goose applies them.
		md, err := migrationInfo(e, byName)
		if err != nil {
			return fmt.Errorf("generating migration for %q: %w", e.Name, err)
		}
		msrc, err := renderMigration(md)
		if err != nil {
			return fmt.Errorf("generating migration for %q: %w", e.Name, err)
		}
		migFile := fmt.Sprintf("migrations/%05d_create_%s.sql", migNum[e.Name], plural(e.Name, e.Plural))
		if err := emit(migFile, msrc); err != nil {
			return err
		}

		hd, err := handlerInfo(s, e, byName)
		if err != nil {
			return fmt.Errorf("generating handlers for %q: %w", e.Name, err)
		}
		hsrc, err := renderHandler(hd)
		if err != nil {
			return fmt.Errorf("generating handlers for %q: %w", e.Name, err)
		}
		if err := emit(base+"_handler.gen.go", hsrc); err != nil {
			return err
		}

		rd, err := repoInfo(s, e, byName)
		if err != nil {
			return fmt.Errorf("generating repository for %q: %w", e.Name, err)
		}
		rsrc, err := renderRepo(rd)
		if err != nil {
			return fmt.Errorf("generating repository for %q: %w", e.Name, err)
		}
		if err := emit(base+"_repo.gen.go", rsrc); err != nil {
			return err
		}
		anyNullable = anyNullable || rd.HasNullable

		serveable = append(serveable, sharedEntity{
			Struct:    hd.Struct,
			Repo:      hd.Repo,
			DepsField: pascalCase(hd.Plural),
		})
	}

	if len(serveable) > 0 {
		ssrc, err := renderShared(sharedData{Package: s.Package, Entities: serveable})
		if err != nil {
			return fmt.Errorf("generating router: %w", err)
		}
		if err := emit("http.gen.go", ssrc); err != nil {
			return err
		}

		dbsrc, err := renderDB(dbData{Package: s.Package, DriverName: driverName, DriverImport: driverImp})
		if err != nil {
			return fmt.Errorf("generating db connection: %w", err)
		}
		if err := emit("db.gen.go", dbsrc); err != nil {
			return err
		}

		// The sql.Null[T] conversion helpers are only needed when at least one
		// repository has a nullable column to round-trip.
		if anyNullable {
			nsrc, err := renderNulls(nullsData{Package: s.Package})
			if err != nil {
				return fmt.Errorf("generating null helpers: %w", err)
			}
			if err := emit("nulls.gen.go", nsrc); err != nil {
				return err
			}
		}
	}

	// The generated code imports third-party packages (sqlx, the driver, uuid,
	// ...); the caller must resolve them in the output module.
	if !toStdout {
		fmt.Fprintf(os.Stderr, "go-crudgen: done. Run `go mod tidy` in %s to resolve dependencies.\n", opts.OutDir)
	}
	return nil
}

// modelData is the template input for a single entity's model file: the model
// struct plus the create/update request DTOs derived from the spec fields.
type modelData struct {
	Package    string
	Imports    []string
	Struct     string
	Lower      string // struct name lower-cased, for the doc comment
	Fields     []modelField
	CreateName string       // e.g. "CreatePostRequest"
	UpdateName string       // e.g. "UpdatePostRequest"
	CreateBody []modelField // all writable fields (includes the primary key)
	UpdateBody []modelField // writable fields minus the primary key (it comes from the path)
}

type modelField struct {
	GoName string
	GoType string
	Tag    string
}

// handlerData is the template input for one entity's handler file.
type handlerData struct {
	Package      string
	Imports      []string
	Struct       string
	Repo         string // e.g. "PostRepository"
	Plural       string // route segment, e.g. "posts"
	CreateName   string
	UpdateName   string
	PK           pkData
	CreateAssign []string // field GoNames assigned from the create request
	UpdateAssign []string // field GoNames assigned from the update request (PK excluded)
}

type pkData struct {
	GoName   string
	GoType   string
	Expr     string // parse expression for the {id} path value
	NeedsErr bool
	Cast     string // Go type to convert the parsed value to ("" when none needed)
}

// sharedData is the template input for the package-wide http.gen.go file.
type sharedData struct {
	Package  string
	Entities []sharedEntity
}

type sharedEntity struct {
	Struct    string
	Repo      string
	DepsField string // field name in Deps, e.g. "Posts"
}

// serveKey resolves the path-addressable primary key used to build an entity's
// handler and repository. Validation guarantees exactly one primary key of a
// path-addressable type, so the pkParser miss below is a defensive check against
// an unvalidated spec, not a normal skip. handlerInfo and repoInfo share this so
// their notion of the primary key can never drift apart.
func serveKey(e *spec.Entity, byName map[string]*spec.Entity) (pk spec.Field, gt goType, pp pkParse, err error) {
	pk = e.PrimaryKey()[0]
	gt, err = fieldType(pk, byName)
	if err != nil {
		return spec.Field{}, goType{}, pkParse{}, err
	}
	pp, ok := pkParser(gt.expr)
	if !ok {
		return spec.Field{}, goType{}, pkParse{}, fmt.Errorf("entity %q primary key %q has non-path-addressable type %q (validation should have rejected it)", e.Name, pk.Name, pk.Type)
	}
	return pk, gt, pp, nil
}

// handlerInfo builds the handler template data for an entity.
func handlerInfo(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) (handlerData, error) {
	pk, gt, pp, err := serveKey(e, byName)
	if err != nil {
		return handlerData{}, err
	}

	name := pascalCase(e.Name)
	data := handlerData{
		Package:    s.Package,
		Struct:     name,
		Repo:       name + "Repository",
		Plural:     plural(e.Name, e.Plural),
		CreateName: "Create" + name + "Request",
		UpdateName: "Update" + name + "Request",
		PK: pkData{
			GoName:   pascalCase(pk.Name),
			GoType:   gt.expr,
			Expr:     pp.expr,
			NeedsErr: pp.needsErr,
			Cast:     pp.cast,
		},
	}
	for _, f := range e.Fields {
		gn := pascalCase(f.Name)
		data.CreateAssign = append(data.CreateAssign, gn)
		if !f.Primary {
			data.UpdateAssign = append(data.UpdateAssign, gn)
		}
	}

	imports := map[string]struct{}{importContext: {}, importNetHTTP: {}}
	if pp.imp != "" {
		imports[pp.imp] = struct{}{}
	}
	for imp := range imports {
		data.Imports = append(data.Imports, imp)
	}
	sort.Strings(data.Imports)
	return data, nil
}

// renderTemplate executes the named template with data and gofmt-formats it.
func renderTemplate(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", name, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated source: %w", err)
	}
	return formatted, nil
}

// renderMigration executes the goose migration template. Unlike the Go
// templates it skips gofmt: the output is SQL, not Go source.
func renderMigration(data migrationData) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "migration.sql.tmpl", data); err != nil {
		return nil, fmt.Errorf("rendering migration: %w", err)
	}
	return buf.Bytes(), nil
}

func renderHandler(data handlerData) ([]byte, error) { return renderTemplate("handler.go.tmpl", data) }
func renderRepo(data repoData) ([]byte, error)       { return renderTemplate("repo.go.tmpl", data) }
func renderNulls(data nullsData) ([]byte, error)     { return renderTemplate("nulls.go.tmpl", data) }
func renderDB(data dbData) ([]byte, error)           { return renderTemplate("db.go.tmpl", data) }
func renderShared(data sharedData) ([]byte, error)   { return renderTemplate("http.go.tmpl", data) }

// renderModel builds, executes, and gofmt-formats the model file for one entity.
func renderModel(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) ([]byte, error) {
	name := pascalCase(e.Name)
	data := modelData{
		Package:    s.Package,
		Struct:     name,
		Lower:      strings.ToLower(name),
		CreateName: "Create" + name + "Request",
		UpdateName: "Update" + name + "Request",
	}
	imports := make(map[string]struct{})

	// The API model carries only json/validate tags; sqlx column mapping lives on
	// the repository's row struct, so the model stays a pure transport type.
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
		mt := modelType(f, gt.expr)
		add(f.Name, mt, gt.imp, fieldTag(f))

		// DTOs carry only spec fields (not the option-injected timestamp fields).
		// The create body accepts every field; the update body omits primary-key
		// fields because they are addressed by the request path.
		mf := modelField{GoName: pascalCase(f.Name), GoType: mt, Tag: fieldTag(f)}
		data.CreateBody = append(data.CreateBody, mf)
		if !f.Primary {
			data.UpdateBody = append(data.UpdateBody, mf)
		}
	}

	// Option-injected columns (timestamps, soft-delete) share one definition
	// with repoInfo so the model struct and the generated SQL never disagree.
	for _, oc := range optionColumns(e.Options) {
		add(oc.Column, oc.GoType, importTime, oc.JSONTag)
	}

	for imp := range imports {
		data.Imports = append(data.Imports, imp)
	}
	sort.Strings(data.Imports)

	return renderTemplate("model.go.tmpl", data)
}

// modelType returns a field's Go type in the API model and DTOs: a pointer for
// nullable fields so a missing or NULL value is distinguishable from a zero
// value, and the bare type otherwise.
func modelType(f spec.Field, base string) string {
	if isNullable(f) {
		return "*" + base
	}
	return base
}

// fieldTag builds the struct tag for a field: a json name (with omitempty for
// nullable fields, whose pointer is nil when absent) plus a validate rule that
// merges the `required` modifier with any explicit `validate` string.
func fieldTag(f spec.Field) string {
	jsonName := f.Name
	if isNullable(f) {
		jsonName += ",omitempty"
	}
	tag := fmt.Sprintf(tagJSON, jsonName)

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
