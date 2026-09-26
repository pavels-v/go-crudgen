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

const (
	templatesGlob = "templates/*.tmpl"
	tmplModel     = "model.go.tmpl"
	tmplErrors    = "errors.go.tmpl"
	tmplHandler   = "handler.go.tmpl"
	tmplRouter    = "router.go.tmpl"
	tmplRepo      = "repo.go.tmpl"
	tmplNulls     = "nulls.go.tmpl"
	tmplDate      = "date.go.tmpl"
	tmplSort      = "sort.go.tmpl"
	tmplDB        = "db.go.tmpl"
	tmplMigration = "migration.sql.tmpl"
)

const (
	pkgREST     = "restapi"
	pkgPostgres = "postgres"
)

const (
	fileModel     = "%s.gen.go"
	fileErrors    = "errors.gen.go"
	fileDate      = "date.gen.go"
	fileSort      = "sort.gen.go"
	fileHandler   = pkgREST + "/%s.gen.go"
	fileRouter    = pkgREST + "/router.gen.go"
	fileRepo      = pkgPostgres + "/%s.gen.go"
	fileDB        = pkgPostgres + "/db.gen.go"
	fileNulls     = pkgPostgres + "/nulls.gen.go"
	fileMigration = "migrations/%05d_create_%s.sql"
)

const (
	nameRepo          = "%sRepository"
	nameRepoCtor      = "New%sRepository"
	nameCreateRequest = "Create%sRequest"
	nameUpdateRequest = "Update%sRequest"
)

const (
	importPathSep = "/"
	importHostDot = "."
)

const (
	ruleRequired = "required"
	ruleSep      = ","
)

const (
	exprReqField = "req.%s"
	exprValueOr  = "valueOr(%s, %s)"
)

var tmpl = template.Must(template.ParseFS(templates, templatesGlob)) //nolint:gochecknoglobals // parsed once from embedded templates

// Options controls generation output.
type Options struct {
	OutDir string
	DryRun bool
	Driver string // database driver for the connection constructor: "pgx" (default) or "pq"
}

type genFile struct {
	Path string
	Src  []byte
}

// Generate produces a Go service from the spec: the domain package (models,
// repository interfaces, errors) in the output root, the HTTP layer in restapi/,
// the PostgreSQL repositories in postgres/ and goose migrations in migrations/.
//
// When opts.OutDir is empty the generated code is written to stdout; otherwise
// the files are written under that directory. Progress and diagnostics always go
// to stderr so stdout carries only generated code.
func Generate(s *spec.Spec, opts Options) error {
	driverName, driverImp, ok := driverInfo(opts.Driver)
	if !ok {
		return fmt.Errorf("unknown driver %q (supported: pgx, pq)", opts.Driver)
	}

	for i := range s.Entities {
		if err := checkColumns(&s.Entities[i]); err != nil {
			return err
		}
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

	files, err := renderFiles(s, driverName, driverImp)
	if err != nil {
		return err
	}
	if err := checkCollisions(files); err != nil {
		return err
	}

	for _, f := range files {
		if err := emit(opts.OutDir, f); err != nil {
			return err
		}
	}

	// The generated code imports third-party packages (sqlx, the driver, uuid,
	// ...); the caller must resolve them in the output module.
	if !toStdout {
		fmt.Fprintf(os.Stderr, "go-crudgen: done. Run `go mod tidy` in %s to resolve dependencies.\n", opts.OutDir)
	}
	return nil
}

func emit(outDir string, f genFile) error {
	if outDir == "" {
		if _, err := fmt.Fprintf(os.Stdout, "// file: %s\n%s\n", f.Path, f.Src); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
		return nil
	}
	path := filepath.Join(outDir, filepath.FromSlash(f.Path))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(path, f.Src, 0o644); err != nil { //nolint:gosec // generated source is world-readable
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "  wrote %s\n", path)
	return nil
}

func renderFiles(s *spec.Spec, driverName, driverImp string) ([]genFile, error) {
	byName := make(map[string]*spec.Entity, len(s.Entities))
	for i := range s.Entities {
		byName[s.Entities[i].Name] = &s.Entities[i]
	}

	// Migrations are numbered so that a referenced table is created before the
	// table whose foreign key points at it, regardless of declaration order.
	order, err := migrationOrder(s.Entities, byName)
	if err != nil {
		return nil, err
	}
	migNum := make(map[string]int, len(order))
	for i, e := range order {
		migNum[e.Name] = i + 1
	}

	var files []genFile
	var shared sharedFiles
	for i := range s.Entities {
		e := &s.Entities[i]
		ef, hd, rd, err := renderEntity(s, e, byName, migNum[e.Name])
		if err != nil {
			return nil, err
		}
		files = append(files, ef...)

		shared.Nullable = shared.Nullable || rd.HasNullable
		shared.Date = shared.Date || hasFieldType(e, spec.TypeDate)
		shared.Routes = append(shared.Routes, routerEntity{
			Struct:    hd.Struct,
			Repo:      hd.Repo,
			DepsField: pascalCase(hd.Plural),
		})
	}

	return appendShared(files, s, driverName, driverImp, shared)
}

type sharedFiles struct {
	Routes   []routerEntity
	Nullable bool
	Date     bool
}

func appendShared(files []genFile, s *spec.Spec, driverName, driverImp string, shared sharedFiles) ([]genFile, error) {
	add := func(path string, src []byte) {
		files = append(files, genFile{Path: path, Src: src})
	}

	esrc, err := renderErrors(packageData{Package: s.Package})
	if err != nil {
		return nil, fmt.Errorf("generate errors: %w", err)
	}
	add(fileErrors, esrc)

	sortSrc, err := renderSort(packageData{Package: s.Package})
	if err != nil {
		return nil, fmt.Errorf("generate sort direction: %w", err)
	}
	add(fileSort, sortSrc)

	ssrc, err := renderRouter(routerInfo(s, shared.Routes))
	if err != nil {
		return nil, fmt.Errorf("generate router: %w", err)
	}
	add(fileRouter, ssrc)

	dbsrc, err := renderDB(dbInfo(s, driverName, driverImp))
	if err != nil {
		return nil, fmt.Errorf("generate db connection: %w", err)
	}
	add(fileDB, dbsrc)

	// The sql.Null[T] conversion helpers are only needed when at least one
	// repository has a nullable column to round-trip.
	if shared.Nullable {
		nsrc, err := renderNulls(packageData{Package: pkgPostgres})
		if err != nil {
			return nil, fmt.Errorf("generate null helpers: %w", err)
		}
		add(fileNulls, nsrc)
	}

	if shared.Date {
		dsrc, err := renderDate(packageData{Package: s.Package})
		if err != nil {
			return nil, fmt.Errorf("generate date type: %w", err)
		}
		add(fileDate, dsrc)
	}

	return files, nil
}

func renderEntity(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity, migNum int) ([]genFile, handlerData, repoData, error) {
	base := snakeCase(e.Name)

	src, err := renderModel(s, e, byName)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate model for %q: %w", e.Name, err)
	}

	md, err := migrationInfo(e, byName)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate migration for %q: %w", e.Name, err)
	}
	msrc, err := renderMigration(md)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate migration for %q: %w", e.Name, err)
	}

	hd, err := handlerInfo(s, e, byName)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate handlers for %q: %w", e.Name, err)
	}
	hsrc, err := renderHandler(hd)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate handlers for %q: %w", e.Name, err)
	}

	rd, err := repoInfo(s, e, byName)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate repository for %q: %w", e.Name, err)
	}
	rsrc, err := renderRepo(rd)
	if err != nil {
		return nil, handlerData{}, repoData{}, fmt.Errorf("generate repository for %q: %w", e.Name, err)
	}

	files := []genFile{
		{Path: fmt.Sprintf(fileModel, base), Src: src},
		{Path: fmt.Sprintf(fileMigration, migNum, plural(e.Name, e.Plural)), Src: msrc},
		{Path: fmt.Sprintf(fileHandler, base), Src: hsrc},
		{Path: fmt.Sprintf(fileRepo, base), Src: rsrc},
	}
	return files, hd, rd, nil
}

// modelData is the template input for a single entity's domain file: the model
// struct and its repository interface.
type modelData struct {
	Package    string
	Imports    []string
	Struct     string
	Lower      string // struct name lower-cased, for the doc comment
	Repo       string // e.g. "PostRepository"
	PKGoType   string
	Fields     []modelField
	ListParams string // e.g. "PostListParams"
	SortType   string // e.g. "PostSort"
	Filters    []modelField
	Sorts      []sortOption
}

type modelField struct {
	GoName string
	GoType string
	Tag    string
}

// handlerData is the template input for one entity's restapi file: the request
// DTOs and the CRUD handlers.
type handlerData struct {
	Package      string
	Imports      []string
	Struct       string
	Lower        string
	Model        string // qualified domain model, e.g. "blog.Post"
	Repo         string // qualified repository interface, e.g. "blog.PostRepository"
	Plural       string // route segment, e.g. "posts"
	CreateName   string
	UpdateName   string
	CreateBody   []modelField // all writable fields (includes a client-supplied primary key)
	UpdateBody   []modelField // writable fields minus the primary key (it comes from the path)
	PK           pkData
	CreateAssign []assign // fields assigned from the create request
	UpdateAssign []assign // fields assigned from the update request (PK excluded)
	ListParams   string   // qualified domain params, e.g. "blog.PostListParams"
	Filters      []handlerFilter
	Sorts        []string // qualified domain sort constants
}

type handlerFilter struct {
	GoName string
	Const  string // query parameter constant, e.g. "queryPostAuthor"
	Name   string // query parameter, e.g. "author"
	Parse  string // func(string) (T, error) for the filter type
}

type pkData struct {
	GoName   string
	Expr     string // parse expression for the {id} path value
	NeedsErr bool
	Cast     string // Go type to convert the parsed value to ("" when none needed)
}

type packageData struct {
	Package string
}

type routerData struct {
	Package  string
	Imports  []string
	Domain   string
	Entities []routerEntity
}

type routerEntity struct {
	Struct    string
	Repo      string // qualified repository interface
	DepsField string // field name in Deps, e.g. "Posts"
}

func routerInfo(s *spec.Spec, entities []routerEntity) routerData {
	return routerData{
		Package: pkgREST,
		Imports: groupImports(map[string]struct{}{
			importJSONv2:    {},
			importJSON:      {},
			importEncoding:  {},
			importErrors:    {},
			importMaps:      {},
			importSlices:    {},
			importNetURL:    {},
			importSlog:      {},
			importNetHTTP:   {},
			importReflect:   {},
			importStrconv:   {},
			importStrings:   {},
			importValidator: {},
			s.Module:        {},
		}, s.Module),
		Domain:   s.Package,
		Entities: entities,
	}
}

func qualified(s *spec.Spec, name string) string {
	return fmt.Sprintf(exprQualified, s.Package, name)
}

func checkColumns(e *spec.Entity) error {
	names := make([]string, 0, len(e.Fields)+3)
	for _, f := range e.Fields {
		names = append(names, f.Name)
	}

	columns := make(map[string]string, len(names))
	goNames := make(map[string]string, len(names))
	for _, n := range names {
		col, goName := snakeCase(n), pascalCase(n)
		if prev, ok := columns[col]; ok {
			return fmt.Errorf("entity %q: %q and %q map to the same column %q", e.Name, prev, n, col)
		}
		if prev, ok := goNames[goName]; ok {
			return fmt.Errorf("entity %q: %q and %q map to the same Go field %q", e.Name, prev, n, goName)
		}
		columns[col], goNames[goName] = n, n
	}
	return nil
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

// handlerInfo builds the restapi template data for an entity.
func handlerInfo(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) (handlerData, error) {
	pk, _, pp, err := serveKey(e, byName)
	if err != nil {
		return handlerData{}, err
	}
	filters, err := listFilters(e, byName)
	if err != nil {
		return handlerData{}, err
	}

	name := pascalCase(e.Name)
	data := handlerData{
		Package:    pkgREST,
		Struct:     name,
		Lower:      strings.ToLower(name),
		Model:      qualified(s, name),
		Repo:       qualified(s, fmt.Sprintf(nameRepo, name)),
		Plural:     plural(e.Name, e.Plural),
		CreateName: fmt.Sprintf(nameCreateRequest, name),
		UpdateName: fmt.Sprintf(nameUpdateRequest, name),
		ListParams: qualified(s, fmt.Sprintf(nameListParams, name)),
		PK: pkData{
			GoName:   pascalCase(pk.Name),
			Expr:     pp.expr,
			NeedsErr: pp.needsErr,
			Cast:     pp.cast,
		},
	}
	imports := map[string]struct{}{importNetHTTP: {}, s.Module: {}}
	for _, f := range e.Fields {
		if f.Generate != "" {
			continue
		}
		gn := pascalCase(f.Name)
		gt, err := fieldType(f, byName)
		if err != nil {
			return handlerData{}, err
		}
		gt = gt.outside(s)

		// The create body accepts every writable field; the update body omits the
		// primary key because it is addressed by the request path.
		mf := modelField{GoName: gn, GoType: modelType(f, gt.expr), Tag: fieldTag(f)}
		expr := fmt.Sprintf(exprReqField, gn)
		if hasRequestDefault(f) {
			mf.GoType = fmt.Sprintf(exprPointer, gt.expr)
			lit, err := goDefault(f)
			if err != nil {
				return handlerData{}, fmt.Errorf("field %q: %w", f.Name, err)
			}
			expr = fmt.Sprintf(exprValueOr, expr, lit)
			if isNowDefault(f) {
				imports[importTime] = struct{}{}
			}
		}
		a := assign{Field: gn, Expr: expr}

		if !f.Primary {
			if gt.imp != "" {
				imports[gt.imp] = struct{}{}
			}
			data.CreateBody = append(data.CreateBody, mf)
			data.UpdateBody = append(data.UpdateBody, mf)
			data.CreateAssign = append(data.CreateAssign, a)
			data.UpdateAssign = append(data.UpdateAssign, a)
			continue
		}
		if _, generated := keyGenerator(f); !generated {
			if gt.imp != "" {
				imports[gt.imp] = struct{}{}
			}
			key := f
			key.Required = true
			mf.Tag = fieldTag(key)
			data.CreateBody = append(data.CreateBody, mf)
			data.CreateAssign = append(data.CreateAssign, a)
		}
	}

	if pp.imp != "" {
		imports[pp.imp] = struct{}{}
	}
	for _, lf := range filters {
		parse, imp := queryParser(lf.Type, s)
		if imp != "" {
			imports[imp] = struct{}{}
		}
		data.Filters = append(data.Filters, handlerFilter{
			GoName: lf.GoName,
			Const:  fmt.Sprintf(nameQueryParam, name, lf.GoName),
			Name:   lf.Field.Name,
			Parse:  parse,
		})
	}
	for _, so := range sortOptions(e) {
		data.Sorts = append(data.Sorts, qualified(s, so.Name))
	}
	data.Imports = groupImports(imports, s.Module)
	return data, nil
}

// renderTemplate executes the named template with data and gofmt-formats it.
func renderTemplate(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}
	return formatted, nil
}

// renderMigration executes the goose migration template. Unlike the Go
// templates it skips gofmt: the output is SQL, not Go source.
func renderMigration(data migrationData) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, tmplMigration, data); err != nil {
		return nil, fmt.Errorf("render migration: %w", err)
	}
	return buf.Bytes(), nil
}

func renderHandler(data handlerData) ([]byte, error) { return renderTemplate(tmplHandler, data) }
func renderRepo(data repoData) ([]byte, error)       { return renderTemplate(tmplRepo, data) }
func renderNulls(data packageData) ([]byte, error)   { return renderTemplate(tmplNulls, data) }
func renderDB(data dbData) ([]byte, error)           { return renderTemplate(tmplDB, data) }
func renderRouter(data routerData) ([]byte, error)   { return renderTemplate(tmplRouter, data) }
func renderDate(data packageData) ([]byte, error)    { return renderTemplate(tmplDate, data) }
func renderErrors(data packageData) ([]byte, error)  { return renderTemplate(tmplErrors, data) }
func renderSort(data packageData) ([]byte, error)    { return renderTemplate(tmplSort, data) }

// renderModel builds, executes, and gofmt-formats the domain file for one entity.
func renderModel(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) ([]byte, error) {
	_, pkType, _, err := serveKey(e, byName)
	if err != nil {
		return nil, err
	}

	filters, err := listFilters(e, byName)
	if err != nil {
		return nil, err
	}

	name := pascalCase(e.Name)
	data := modelData{
		Package:    s.Package,
		Struct:     name,
		Lower:      strings.ToLower(name),
		Repo:       fmt.Sprintf(nameRepo, name),
		PKGoType:   pkType.expr,
		ListParams: fmt.Sprintf(nameListParams, name),
		SortType:   fmt.Sprintf(nameSortType, name),
		Sorts:      sortOptions(e),
	}
	imports := map[string]struct{}{importContext: {}}
	for _, lf := range filters {
		data.Filters = append(data.Filters, modelField{GoName: lf.GoName, GoType: fmt.Sprintf(exprPointer, lf.Type.expr)})
	}

	// The model carries only json tags: request validation lives on the restapi
	// DTOs and sqlx column mapping on the postgres row struct.
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
		add(f.Name, modelType(f, gt.expr), gt.imp, jsonTag(f))
	}

	data.Imports = groupImports(imports, s.Module)

	return renderTemplate(tmplModel, data)
}

// modelType returns a field's Go type in the API model and DTOs: a pointer for
// nullable fields so a missing or NULL value is distinguishable from a zero
// value, and the bare type otherwise.
func modelType(f spec.Field, base string) string {
	if isNullable(f) {
		return fmt.Sprintf(exprPointer, base)
	}
	return base
}

// jsonTag builds a field's json struct tag, with omitzero for nullable fields,
// whose pointer is nil when absent.
func jsonTag(f spec.Field) string {
	jsonName := f.Name
	if isNullable(f) {
		jsonName += jsonOmit
	}
	return fmt.Sprintf(tagJSON, jsonName)
}

// fieldTag builds a DTO field's struct tag: the json tag plus a validate rule
// that merges the `required` modifier with any explicit `validate` string.
func fieldTag(f spec.Field) string {
	tag := jsonTag(f)

	var rules []string
	if f.Required && !strings.Contains(f.Validate, ruleRequired) {
		rules = append(rules, ruleRequired)
	}
	if f.Validate != "" {
		rules = append(rules, f.Validate)
	}
	if len(rules) > 0 {
		tag += fmt.Sprintf(tagValidate, strings.Join(rules, ruleSep))
	}
	return tag
}

func groupImports(set map[string]struct{}, module string) []string {
	var std, ext, local []string
	for imp := range set {
		host, _, _ := strings.Cut(imp, importPathSep)
		switch {
		case imp == module || strings.HasPrefix(imp, module+importPathSep):
			local = append(local, imp)
		case strings.Contains(host, importHostDot):
			ext = append(ext, imp)
		default:
			std = append(std, imp)
		}
	}

	var out []string
	for _, group := range [][]string{std, ext, local} {
		if len(group) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		sort.Strings(group)
		out = append(out, group...)
	}
	return out
}

func hasFieldType(e *spec.Entity, typ string) bool {
	for _, f := range e.Fields {
		if f.Type == typ {
			return true
		}
	}
	return false
}

func entityWord(n int) string {
	if n == 1 {
		return "entity"
	}
	return "entities"
}
