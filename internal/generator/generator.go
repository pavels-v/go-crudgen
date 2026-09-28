// Package generator turns a validated spec into Go source files.
package generator

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/pavels-v/go-crudgen/internal/spec"
)

//go:embed templates/*.tmpl
var templates embed.FS

const (
	templatesGlob = "templates/*.tmpl"
	tmplModel     = "model.go.tmpl"
	tmplErrors    = "errors.go.tmpl"
	tmplHandler   = "handler.go.tmpl"
	tmplRequest   = "request.go.tmpl"
	tmplResponse  = "response.go.tmpl"
	tmplQuery     = "query.go.tmpl"
	tmplRoutes    = "routes.go.tmpl"
	tmplRouter    = "router.go.tmpl"
	tmplMain      = "main.go.tmpl"
	tmplEmbed     = "embed.go.tmpl"
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
	fileModel     = "%s.go"
	fileErrors    = "errors.go"
	fileDate      = "date.go"
	fileSort      = "sort.go"
	fileHandler   = pkgREST + "/%s.go"
	fileRequest   = pkgREST + "/request.go"
	fileResponse  = pkgREST + "/response.go"
	fileQuery     = pkgREST + "/query.go"
	fileRoutes    = pkgREST + "/routes.go"
	fileRouter    = pkgREST + "/router.go"
	fileEmbed     = dirMigrations + "/embed.go"
	fileMain      = "main.go"
	fileGoMod     = "go.mod"
	dirCmd        = "cmd"
	fileRepo      = pkgPostgres + "/%s.go"
	fileDB        = pkgPostgres + "/db.go"
	fileNulls     = pkgPostgres + "/nulls.go"
	fileMigration = dirMigrations + "/%s_create_%s.sql"
)

const (
	nameRepo          = "%sRepository"
	nameRepoCtor      = "New%sRepository"
	nameCreateRequest = "Create%sRequest"
	nameUpdateRequest = "Update%sRequest"
	nameDefault       = "default%s%s"
)

const (
	importPathSep  = "/"
	importHostDot  = "."
	importAliasSep = " "
	domainAlias    = "domain"
	typeParamOpen  = "["
)

var listParamFields = []string{listParamDir, listParamAfter, listParamLimit, listParamOffset} //nolint:gochecknoglobals // read-only lookup table

const (
	listParamDir    = "Dir"
	listParamAfter  = "After"
	listParamLimit  = "Limit"
	listParamOffset = "Offset"
)

const (
	destStdout     = "stdout"
	filesPerEntity = 4
)

const (
	dirMigrations          = "migrations"
	migrationVersionLayout = "20060102150405"
	globGo                 = "*.go"
	globAny                = "*"
	suffixTest             = "_test.go"
)

const (
	ruleRequired  = "required"
	ruleExcluded  = "excluded"
	ruleOmitEmpty = "omitempty"
	ruleOmitNil   = "omitnil"
	ruleOmitZero  = "omitzero"
	ruleUUID      = "uuid"
	ruleSep       = ","
	ruleOr        = "|"
	ruleParamSep  = "="
)

const (
	exprReqField = "req.%s"
	exprValueOr  = "valueOr(%s, %s)"
	exprDeref    = "*%s"
)

var tmpl = template.Must(template.ParseFS(templates, templatesGlob)) //nolint:gochecknoglobals // parsed once from embedded templates

// Options controls generation output.
type Options struct {
	OutDir string
	DryRun bool
	Driver string // database driver for the connection constructor: "pgx" (default) or "pq"
	Router bool   // also emit restapi.NewRouter and Deps

	MigrationTime time.Time // version of the first migration, the next ones follow a second apart; zero means now
	Main          bool      // emit main.go and migrations/embed.go
	Tests         bool      // emit restapi handler tests with a fake repository
	MainDir       string    // directory for main.go; empty means cmd/<package> next to the nearest go.mod
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

	if err := checkTables(s); err != nil {
		return err
	}

	byName := entitiesByName(s)
	for i := range s.Entities {
		if err := checkColumns(&s.Entities[i]); err != nil {
			return err
		}

		if err := checkRules(&s.Entities[i], byName); err != nil {
			return err
		}
	}

	toStdout := opts.OutDir == ""

	var modRoot string

	if !toStdout {
		root, err := findModuleRoot(opts.OutDir)
		if err != nil {
			return err
		}

		modRoot = root
	}

	if err := resolveModule(s, opts.OutDir, modRoot); err != nil {
		return err
	}

	dest := opts.OutDir
	if toStdout {
		dest = destStdout
	}

	fmt.Fprintf(os.Stderr, "go-crudgen: package %q, %d %s -> %s\n",
		s.Package, len(s.Entities), entityWord(len(s.Entities)), dest)

	for _, e := range s.Entities {
		fmt.Fprintf(os.Stderr, "  - %s (%d fields)\n", e.Name, len(e.Fields))
	}

	if opts.MigrationTime.IsZero() {
		opts.MigrationTime = time.Now()
	}

	if opts.Main && opts.MainDir == "" {
		dir, err := defaultMainDir(opts.OutDir, modRoot, s.Package)
		if err != nil {
			return err
		}

		opts.MainDir = dir
	}

	files, err := renderFiles(s, byName, driverName, driverImp, opts)
	if err != nil {
		return err
	}

	if err := checkCollisions(files); err != nil {
		return err
	}

	if !toStdout {
		if err := checkOutDir(opts.OutDir, files); err != nil {
			return err
		}
	}

	if opts.DryRun {
		for _, f := range files {
			fmt.Fprintf(os.Stderr, "  would write %s\n", f.Path)
		}

		fmt.Fprintln(os.Stderr, "(dry run: no files written)")

		return nil
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

func checkOutDir(dir string, files []genFile) error {
	for _, f := range files {
		target := filepath.Join(dir, filepath.FromSlash(f.Path))

		_, err := os.Stat(target)
		if err == nil {
			return outDirTaken(dir, target)
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("check output directory: %w", err)
		}
	}

	patterns := []string{
		path.Join(pkgREST, globGo),
		path.Join(pkgPostgres, globGo),
		path.Join(dirMigrations, globAny),
	}

	for _, p := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return fmt.Errorf("check output directory: %w", err)
		}

		for _, m := range matches {
			if !strings.HasSuffix(m, suffixTest) {
				return outDirTaken(dir, m)
			}
		}
	}

	return nil
}

func outDirTaken(dir, file string) error {
	return fmt.Errorf("output directory %s already has generated code or migrations (%s): generate into a clean directory", dir, file)
}

func checkTables(s *spec.Spec) error {
	tables := make(map[string]string, len(s.Entities))
	for i := range s.Entities {
		e := &s.Entities[i]

		t := tableName(e)
		if prev, ok := tables[t]; ok {
			return fmt.Errorf("entities %q and %q map to the same table %q: set distinct plurals", prev, e.Name, t)
		}

		tables[t] = e.Name
	}

	return nil
}

func entitiesByName(s *spec.Spec) map[string]*spec.Entity {
	byName := make(map[string]*spec.Entity, len(s.Entities))
	for i := range s.Entities {
		byName[s.Entities[i].Name] = &s.Entities[i]
	}

	return byName
}

type sharedFiles struct {
	Router   bool
	Filters  bool
	Defaults bool
	Routes   []routerEntity
	Parsers  map[string]bool
	Nullable bool
	Date     bool
	Offset   bool
	Cursor   bool
}

func renderFiles(s *spec.Spec, byName map[string]*spec.Entity, driverName, driverImp string, opts Options) ([]genFile, error) {
	// Migrations are versioned so that a referenced table is created before the
	// table whose foreign key points at it, regardless of declaration order.
	order, err := migrationOrder(s.Entities, byName)
	if err != nil {
		return nil, err
	}

	base := opts.MigrationTime.UTC()

	migVersion := make(map[string]string, len(order))
	for i, e := range order {
		migVersion[e.Name] = base.Add(time.Duration(i) * time.Second).Format(migrationVersionLayout)
	}

	files := make([]genFile, 0, len(s.Entities)*filesPerEntity)
	shared := sharedFiles{Parsers: make(map[string]bool), Router: opts.Router}
	v := validator.New()

	for i := range s.Entities {
		e := &s.Entities[i]

		ef, hd, rd, err := renderEntity(s, e, byName, migVersion[e.Name])
		if err != nil {
			return nil, err
		}

		files = append(files, ef...)

		if opts.Tests {
			tsrc, err := renderHandlerTest(s, e, byName, v)
			if err != nil {
				return nil, fmt.Errorf("generate handler tests for %q: %w", e.Name, err)
			}

			files = append(files, genFile{Path: fmt.Sprintf(fileHandlerTest, snakeCase(e.Name)), Src: tsrc})
		}

		shared.Nullable = shared.Nullable || rd.HasNullable
		shared.Date = shared.Date || hasFieldType(e, spec.TypeDate)
		shared.Cursor = shared.Cursor || e.CursorPagination()
		shared.Offset = shared.Offset || !e.CursorPagination()
		shared.Filters = shared.Filters || len(hd.Filters) > 0
		shared.Defaults = shared.Defaults || hd.usesValueOr

		for _, p := range hd.parsers {
			helper, _, _ := strings.Cut(p, typeParamOpen)
			shared.Parsers[helper] = true
		}

		shared.Routes = append(shared.Routes, routerEntity{
			Struct:    hd.Struct,
			Repo:      hd.Repo,
			RepoCtor:  fmt.Sprintf(nameRepoCtor, hd.Struct),
			DepsField: pascalCase(hd.Plural),
		})
	}

	if opts.Tests {
		fsrc, err := renderFakeTest(s)
		if err != nil {
			return nil, fmt.Errorf("generate fake repository: %w", err)
		}

		files = append(files, genFile{Path: fileFakeTest, Src: fsrc})
	}

	if opts.Main {
		mf, err := mainFiles(s, shared, opts)
		if err != nil {
			return nil, err
		}

		files = append(files, mf...)
	}

	return appendShared(files, s, driverName, driverImp, shared)
}

type mainData struct {
	Imports  []string
	Router   bool
	Entities []routerEntity
}

func mainFiles(s *spec.Spec, shared sharedFiles, opts Options) ([]genFile, error) {
	mainPath, err := mainFilePath(opts.OutDir, opts.MainDir)
	if err != nil {
		return nil, err
	}

	imports := map[string]struct{}{
		importContext:                      {},
		importDatabaseSQL:                  {},
		importErrors:                       {},
		importFmt:                          {},
		importSlog:                         {},
		importNetHTTP:                      {},
		importOS:                           {},
		importOSSignal:                     {},
		importSyscall:                      {},
		importTime:                         {},
		importGoose:                        {},
		importSQLx:                         {},
		path.Join(s.Module, dirMigrations): {},
		path.Join(s.Module, pkgPostgres):   {},
		path.Join(s.Module, pkgREST):       {},
	}

	msrc, err := renderTemplate(tmplMain, mainData{
		Imports:  groupImports(imports, s.Module),
		Router:   shared.Router,
		Entities: shared.Routes,
	})
	if err != nil {
		return nil, fmt.Errorf("generate main: %w", err)
	}

	esrc, err := renderTemplate(tmplEmbed, packageData{Package: dirMigrations})
	if err != nil {
		return nil, fmt.Errorf("generate migrations embed: %w", err)
	}

	return []genFile{{Path: fileEmbed, Src: esrc}, {Path: mainPath, Src: msrc}}, nil
}

func defaultMainDir(outDir, modRoot, pkg string) (string, error) {
	if outDir == "" {
		return filepath.Join(dirCmd, pkg), nil
	}

	if modRoot == "" {
		return "", fmt.Errorf("no go.mod above %s to place main.go next to: pass --main <dir> or --no-main", outDir)
	}

	return filepath.Join(modRoot, dirCmd, pkg), nil
}

func mainFilePath(outDir, mainDir string) (string, error) {
	if outDir == "" {
		return path.Join(filepath.ToSlash(filepath.Clean(mainDir)), fileMain), nil
	}

	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}

	absMain, err := filepath.Abs(mainDir)
	if err != nil {
		return "", fmt.Errorf("resolve main directory: %w", err)
	}

	rel, err := filepath.Rel(absOut, absMain)
	if err != nil {
		return "", fmt.Errorf("resolve main directory: %w", err)
	}

	return path.Join(filepath.ToSlash(rel), fileMain), nil
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

	rest, err := restFiles(s, shared)
	if err != nil {
		return nil, err
	}

	files = append(files, rest...)

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

func renderEntity(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity, migVersion string) ([]genFile, handlerData, repoData, error) {
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
		{Path: fmt.Sprintf(fileMigration, migVersion, tableName(e)), Src: msrc},
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
	Filters    []modelField
	CursorType string // e.g. "PostCursor", empty under offset pagination
	Cursor     []modelField
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
	Model        string // qualified domain model, e.g. "domain.Post"
	Repo         string // qualified repository interface, e.g. "domain.PostRepository"
	Plural       string // route segment, e.g. "posts"
	CreateName   string
	UpdateName   string
	CreateBody   []modelField // all writable fields (includes a client-supplied primary key)
	UpdateBody   []modelField // writable fields minus the primary key (it comes from the path)
	PK           pkData
	CreateAssign []assign // fields assigned from the create request
	UpdateAssign []assign // fields assigned from the update request (PK excluded)
	ListParams   string   // qualified domain params, e.g. "domain.PostListParams"
	Filters      []handlerFilter
	SortDir      string   // qualified, e.g. "domain.SortDir"
	Cursor       string   // wire cursor type, e.g. "postCursor", empty under offset pagination
	CursorType   string   // qualified domain cursor, e.g. "domain.PostCursor"
	CursorFields []string // Go names copied from the last item into the next cursor
	Defaults     []defaultConst
	parsers      []string // request.go parse helpers the handlers call
	usesValueOr  bool
}

type defaultConst struct {
	Name  string
	Value string
}

type handlerFilter struct {
	GoName string
	Const  string // query parameter constant, e.g. "queryPostAuthor"
	Name   string // query parameter, e.g. "author"
	Parse  string // func(string) (T, error) for the filter type
}

type pkData struct {
	GoName string
	Parse  string // func(string) (T, error) for the {id} path value
}

type packageData struct {
	Package string
}

type restData struct {
	Package  string
	Imports  []string
	Domain   string
	Entities []routerEntity
	Offset   bool // some entity pages by offset
	Cursor   bool // some entity pages by cursor
	Filters  bool // some entity has a List filter
	Defaults bool // some request field has a default

	ParseString bool
	ParseInt32  bool
	ParseInt64  bool
	ParseText   bool
}

type routerEntity struct {
	Struct    string
	Repo      string // qualified repository interface
	RepoCtor  string // postgres repository constructor, e.g. "NewPostRepository"
	DepsField string // field name in Deps, e.g. "Posts"
}

type restFile struct {
	path    string
	tmpl    string
	imports []string
}

func restFiles(s *spec.Spec, shared sharedFiles) ([]genFile, error) {
	data := restData{
		Package:     pkgREST,
		Domain:      domainAlias,
		Entities:    shared.Routes,
		Offset:      shared.Offset,
		Cursor:      shared.Cursor,
		Filters:     shared.Filters,
		Defaults:    shared.Defaults,
		ParseString: shared.Parsers[parseString],
		ParseInt32:  shared.Parsers[parseInt32],
		ParseInt64:  shared.Parsers[parseInt64],
		ParseText:   shared.Parsers[parseText],
	}

	request := []string{importJSONv2, importNetHTTP, importReflect, importStrings, importValidator}
	if data.ParseInt32 || data.ParseInt64 {
		request = append(request, importStrconv)
	}

	if data.ParseText {
		request = append(request, importEncoding)
	}

	query := []string{importMaps, importNetHTTP, importNetURL, importSlices, importStrconv, s.Module}
	if data.Cursor {
		query = append(query, importBase64, importFmt, importJSONv2)
	}

	parts := []restFile{
		{path: fileRequest, tmpl: tmplRequest, imports: request},
		{path: fileResponse, tmpl: tmplResponse, imports: []string{importJSON, importJSONv2, importErrors, importSlog, importNetHTTP, importValidator, s.Module}},
		{path: fileQuery, tmpl: tmplQuery, imports: query},
		{path: fileRoutes, tmpl: tmplRoutes, imports: []string{importNetHTTP}},
	}
	if shared.Router {
		parts = append(parts, restFile{path: fileRouter, tmpl: tmplRouter, imports: []string{importNetHTTP, s.Module}})
	}

	files := make([]genFile, 0, len(parts))
	for _, p := range parts {
		set := make(map[string]struct{}, len(p.imports))
		for _, imp := range p.imports {
			set[imp] = struct{}{}
		}

		data.Imports = groupImports(set, s.Module)

		src, err := renderTemplate(p.tmpl, data)
		if err != nil {
			return nil, fmt.Errorf("generate %s: %w", p.path, err)
		}

		files = append(files, genFile{Path: p.path, Src: src})
	}

	return files, nil
}

func qualified(name string) string {
	return fmt.Sprintf(exprQualified, domainAlias, name)
}

func checkColumns(e *spec.Entity) error {
	if name := pascalCase(e.Name); !token.IsIdentifier(name) {
		return fmt.Errorf("entity %q does not map to a Go identifier (%q)", e.Name, name)
	}

	columns := make(map[string]string, len(e.Fields))
	goNames := make(map[string]string, len(e.Fields))

	for _, f := range e.Fields {
		col, goName := snakeCase(f.Name), pascalCase(f.Name)
		if !token.IsIdentifier(goName) {
			return fmt.Errorf("entity %q field %q does not map to a Go identifier (%q)", e.Name, f.Name, goName)
		}

		if f.Filter && slices.Contains(listParamFields, goName) {
			return fmt.Errorf("entity %q field %q cannot be a filter: %s is a List parameter", e.Name, f.Name, goName)
		}

		if prev, ok := columns[col]; ok {
			return fmt.Errorf("entity %q: %q and %q map to the same column %q", e.Name, prev, f.Name, col)
		}

		if prev, ok := goNames[goName]; ok {
			return fmt.Errorf("entity %q: %q and %q map to the same Go field %q", e.Name, prev, f.Name, goName)
		}

		columns[col], goNames[goName] = f.Name, f.Name
	}

	return nil
}

// serveKey resolves the path-addressable primary key used to build an entity's
// handler and repository. Validation guarantees exactly one primary key of a
// path-addressable type, so the pathAddressable miss below is a defensive check against
// an unvalidated spec, not a normal skip. handlerInfo and repoInfo share this so
// their notion of the primary key can never drift apart.
func serveKey(e *spec.Entity, byName map[string]*spec.Entity) (pk spec.Field, gt goType, err error) {
	pk = e.PrimaryKey()[0]

	gt, err = fieldType(pk, byName)
	if err != nil {
		return spec.Field{}, goType{}, err
	}

	if !pathAddressable(gt.expr) {
		return spec.Field{}, goType{}, fmt.Errorf("entity %q primary key %q has non-path-addressable type %q (validation should have rejected it)", e.Name, pk.Name, pk.Type)
	}

	return pk, gt, nil
}

// handlerInfo builds the restapi template data for an entity.
func handlerInfo(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) (handlerData, error) {
	pk, pkType, err := serveKey(e, byName)
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
		Model:      qualified(name),
		Repo:       qualified(fmt.Sprintf(nameRepo, name)),
		Plural:     plural(e.Name, e.Plural),
		CreateName: fmt.Sprintf(nameCreateRequest, name),
		UpdateName: fmt.Sprintf(nameUpdateRequest, name),
		ListParams: qualified(fmt.Sprintf(nameListParams, name)),
		SortDir:    qualified(nameSortDir),
	}

	pkParse, pkImp := queryParser(pkType, s)
	data.PK = pkData{GoName: pascalCase(pk.Name), Parse: pkParse}
	data.parsers = append(data.parsers, pkParse)

	imports := map[string]struct{}{importNetHTTP: {}, s.Module: {}}
	if pkImp != "" {
		imports[pkImp] = struct{}{}
	}

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
		req := f
		req.Required = f.Required || f.Primary
		mf := modelField{GoName: gn, GoType: modelType(f, gt.expr), Tag: fieldTag(req)}

		expr := fmt.Sprintf(exprReqField, gn)
		if requiresPresence(req, gt) {
			mf.GoType = fmt.Sprintf(exprPointer, gt.expr)
			expr = fmt.Sprintf(exprDeref, expr)
		}

		if hasRequestDefault(f) {
			data.usesValueOr = true
			mf.GoType = fmt.Sprintf(exprPointer, gt.expr)

			lit, err := goDefault(f)
			if err != nil {
				return handlerData{}, fmt.Errorf("field %q: %w", f.Name, err)
			}

			if isNowDefault(f) {
				imports[importTime] = struct{}{}
			} else {
				c := defaultConst{Name: fmt.Sprintf(nameDefault, name, gn), Value: lit}
				data.Defaults = append(data.Defaults, c)
				lit = c.Name
			}

			expr = fmt.Sprintf(exprValueOr, expr, lit)
		}

		a := assign{Field: gn, Expr: expr}

		if gt.imp != "" {
			imports[gt.imp] = struct{}{}
		}

		if !f.Primary {
			data.CreateBody = append(data.CreateBody, mf)
			data.UpdateBody = append(data.UpdateBody, mf)
			data.CreateAssign = append(data.CreateAssign, a)
			data.UpdateAssign = append(data.UpdateAssign, a)
			continue
		}

		if _, generated := keyGenerator(f); !generated {
			data.CreateBody = append(data.CreateBody, mf)
			data.CreateAssign = append(data.CreateAssign, a)
		}
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
		data.parsers = append(data.parsers, parse)
	}

	if e.CursorPagination() {
		data.Cursor = unexport(fmt.Sprintf(nameCursor, name))
		data.CursorType = qualified(fmt.Sprintf(nameCursor, name))

		for _, f := range cursorFields(e) {
			data.CursorFields = append(data.CursorFields, pascalCase(f.Name))
		}
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
func renderDate(data packageData) ([]byte, error)    { return renderTemplate(tmplDate, data) }
func renderErrors(data packageData) ([]byte, error)  { return renderTemplate(tmplErrors, data) }
func renderSort(data packageData) ([]byte, error)    { return renderTemplate(tmplSort, data) }

// renderModel builds, executes, and gofmt-formats the domain file for one entity.
func renderModel(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) ([]byte, error) {
	_, pkType, err := serveKey(e, byName)
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
	}
	imports := map[string]struct{}{importContext: {}}

	for _, lf := range filters {
		data.Filters = append(data.Filters, modelField{GoName: lf.GoName, GoType: fmt.Sprintf(exprPointer, lf.Type.expr)})
	}

	// The model carries only json tags: request validation lives on the restapi
	// DTOs and sqlx column mapping on the postgres row struct.
	add := func(field, goExpr, imp, tag string) {
		if imp != "" {
			imports[imp] = struct{}{}
		}

		data.Fields = append(data.Fields, modelField{GoName: pascalCase(field), GoType: goExpr, Tag: tag})
	}

	for _, f := range e.Fields {
		gt, err := fieldType(f, byName)
		if err != nil {
			return nil, err
		}

		add(f.Name, modelType(f, gt.expr), gt.imp, jsonTag(f))
	}

	if e.CursorPagination() {
		data.CursorType = fmt.Sprintf(nameCursor, name)
		for _, f := range cursorFields(e) {
			gt, err := fieldType(f, byName)
			if err != nil {
				return nil, err
			}

			data.Cursor = append(data.Cursor, modelField{GoName: pascalCase(f.Name), GoType: gt.expr, Tag: jsonTag(f)})
		}
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
	if f.Required && !slices.Contains(strings.Split(f.Validate, ruleSep), ruleRequired) {
		rules = append(rules, ruleRequired)
	}

	if f.Validate != "" && (isNullable(f) || hasRequestDefault(f)) {
		rules = append(rules, ruleOmitNil)
	}

	if f.Validate != "" {
		rules = append(rules, f.Validate)
	}

	if len(rules) > 0 {
		tag += fmt.Sprintf(tagValidate, strings.Join(rules, ruleSep))
	}

	return tag
}

func requiresPresence(f spec.Field, gt goType) bool {
	_, isString := gt.sample.(string)
	return f.Required && !isString
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

		for _, imp := range group {
			out = append(out, importSpec(imp, module))
		}
	}

	return out
}

func importSpec(imp, module string) string {
	quoted := strconv.Quote(imp)
	if imp == module {
		return domainAlias + importAliasSep + quoted
	}

	return quoted
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
