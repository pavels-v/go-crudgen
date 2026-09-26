package generator

import (
	"fmt"
	"strings"

	"go-crudgen/internal/spec"
)

// dbData is the template input for postgres/db.gen.go.
type dbData struct {
	Package      string
	Imports      []string
	Domain       string
	DriverName   string // database/sql driver name, e.g. "pgx" or "postgres"
	DriverImport string // blank-imported driver package
}

func dbInfo(s *spec.Spec, driverName, driverImp string) dbData {
	return dbData{
		Package: pkgPostgres,
		Imports: groupImports(map[string]struct{}{
			importErrors: {},
			importFmt:    {},
			importTime:   {},
			importSQLx:   {},
			driverImp:    {},
			s.Module:     {},
		}, s.Module),
		Domain:       s.Package,
		DriverName:   driverName,
		DriverImport: driverImp,
	}
}

const (
	DriverPgx = "pgx"
	DriverPq  = "pq"
)

const (
	sqlDriverPgx = "pgx"
	sqlDriverPq  = "postgres"
)

const (
	importDriverPgx = "github.com/jackc/pgx/v5/stdlib"
	importDriverPq  = "github.com/lib/pq"
)

const (
	importNetHTTP     = "net/http"
	importContext     = "context"
	importDatabaseSQL = "database/sql"
	importErrors      = "errors"
	importFmt         = "fmt"
	importSQLx        = "github.com/jmoiron/sqlx"
	importJSONv2      = "encoding/json/v2"
	importReflect     = "reflect"
	importStrings     = "strings"
	importSlog        = "log/slog"
	importValidator   = "github.com/go-playground/validator/v10"
	importEncoding    = "encoding"
	importMaps        = "maps"
	importSlices      = "slices"
	importNetURL      = "net/url"
)

// driverInfo maps a --driver choice to its database/sql driver name and the
// package that must be blank-imported to register it. An empty choice defaults
// to pgx. ok is false for unsupported drivers.
func driverInfo(driver string) (name, imp string, ok bool) {
	switch driver {
	case "", DriverPgx:
		return sqlDriverPgx, importDriverPgx, true
	case DriverPq:
		return sqlDriverPq, importDriverPq, true
	}
	return "", "", false
}

const (
	colCreatedAt = "created_at"
	colUpdatedAt = "updated_at"
)

const (
	tagDB       = "db:%q"
	tagJSON     = "json:%q"
	tagValidate = " validate:%q"
	jsonOmit    = ",omitzero"
)

const clauseReturning = " RETURNING %s"

const (
	exprNull     = "sql.Null[%s]"
	exprToNull   = "toNull(%s)"
	exprFromNull = "fromNull(%s)"
)

const (
	exprPointer    = "*%s"
	exprModelField = "m.%s"
	exprRowField   = "row.%s"
	exprScanTarget = "&m.%s"
	argSep         = ", "
)

const (
	nameRow    = "%sRow"
	nameNewRow = "new%sRow"
)

// optionColumn describes a column injected by an EntityOptions toggle.
type optionColumn struct {
	Column  string
	GoName  string
	GoType  string
	JSONTag string
}

// optionColumns returns the columns injected by an entity's options, in the
// order they are appended to the model struct.
// renderModel and repoInfo both consume it so the model struct and the
// generated SQL agree on which columns exist and their order.
func optionColumns(o spec.EntityOptions) []optionColumn {
	var cols []optionColumn
	if o.Timestamps {
		cols = append(cols,
			optionColumn{colCreatedAt, pascalCase(colCreatedAt), goTime, fmt.Sprintf(tagJSON, colCreatedAt)},
			optionColumn{colUpdatedAt, pascalCase(colUpdatedAt), goTime, fmt.Sprintf(tagJSON, colUpdatedAt)},
		)
	}
	return cols
}

// scanList renders the scan-target list for a set of columns, e.g. ["created_at"]
// -> "&m.CreatedAt". It shares the column list with the SQL builder so a
// RETURNING clause and its Scan targets can never drift in arity or order.
func scanList(cols []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = fmt.Sprintf(exprScanTarget, pascalCase(c))
	}
	return strings.Join(out, argSep)
}

// repoData is the template input for one entity's PostgreSQL repository file.
// The SQL statements are built here rather than in the template so the column
// math (placeholders, ordering, timestamp handling) stays testable.
type repoData struct {
	Package         string
	Imports         []string
	Domain          string
	Struct          string
	Model           string // qualified domain model, e.g. "blog.Post"
	LowerStruct     string // struct name lower-cased, for error messages
	Repo            string // qualified interface name, e.g. "blog.PostRepository"
	Receiver        string // concrete type, e.g. "PostRepository"
	Constructor     string // e.g. "NewPostRepository"
	PKGoType        string
	HasTimestamps   bool
	GenerateUUID    bool // the uuid primary key is generated in Go before insert
	PKGoName        string
	NoUpdateColumns bool // entity has no writable columns; Update is an existence check
	HasNullable     bool // any persisted column is nullable (so sql.Null helpers are used)

	RowStruct  string     // db-mapped scan type, e.g. "postRow"
	NewRowFunc string     // model -> row constructor, e.g. "newPostRow"
	RowFields  []rowField // the row struct's fields, in column order
	ToRow      []assign   // spec-field assignments for NewRowFunc (model -> row)
	ToModel    []assign   // all persisted columns for toModel (row -> model)

	CreateSQL string
	GetSQL    string
	ListSQL   string // full query, or the SELECT ... FROM prefix when ListDynamic
	UpdateSQL string
	DeleteSQL string

	ListParams  string // qualified domain params, e.g. "blog.PostListParams"
	ListDynamic bool   // WHERE and ORDER BY are assembled from ListFilters and ListSorts
	ListFilters []repoFilter
	ListSorts   []sortOption // Name is the qualified domain constant
	ListOrder   string       // default ORDER BY

	InsertArgs string // create args, e.g. "row.ID, row.Title"
	UpdateArgs string // update args: non-PK fields then PK, e.g. "row.Title, row.ID"
	CreateScan string // scan targets for the create RETURNING clause
	UpdateScan string // scan targets for the update RETURNING clause
}

type repoFilter struct {
	GoName string
	Clause string // e.g. "author = ?"
}

// rowField is one field of the repository's row struct.
type rowField struct {
	GoName string
	GoType string
	Tag    string // db struct tag, e.g. `db:"title"`
}

// assign is a single keyed field assignment in a generated struct literal.
type assign struct {
	Field string
	Expr  string
}

// repoInfo builds the repository template data for an entity.
func repoInfo(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) (repoData, error) {
	pk, gt, _, err := serveKey(e, byName)
	if err != nil {
		return repoData{}, err
	}
	filters, err := listFilters(e, byName)
	if err != nil {
		return repoData{}, err
	}

	name := pascalCase(e.Name)
	table := plural(e.Name, e.Plural)
	pkCol := snakeCase(pk.Name)
	pkGoName := pascalCase(pk.Name)
	ts := e.Options.Timestamps

	impSet := map[string]struct{}{
		importContext: {}, importDatabaseSQL: {}, importErrors: {}, importFmt: {}, importSQLx: {}, s.Module: {},
	}

	// Persisted columns in struct order: spec fields first, then the
	// option-injected columns (shared with renderModel via optionColumns). Each
	// column contributes a row-struct field and a model<->row conversion; nullable
	// columns become sql.Null[T] on the row (pointer on the model), bridged by the
	// generic toNull/fromNull helpers.
	type col struct {
		Column string
		GoName string
	}
	var specCols, update []col
	var rowFields []rowField
	var toRow, toModel []assign
	hasNullable := false
	for _, f := range e.Fields {
		c := col{Column: snakeCase(f.Name), GoName: pascalCase(f.Name)}
		specCols = append(specCols, c) // the primary key is supplied by the caller
		if !f.Primary {
			update = append(update, c)
		}

		ft, err := fieldType(f, byName)
		if err != nil {
			return repoData{}, err
		}
		ft = ft.outside(s)
		if ft.imp != "" {
			impSet[ft.imp] = struct{}{}
		}
		rowType := ft.expr
		toRowExpr := fmt.Sprintf(exprModelField, c.GoName)
		toModelExpr := fmt.Sprintf(exprRowField, c.GoName)
		if isNullable(f) {
			rowType = fmt.Sprintf(exprNull, ft.expr)
			toRowExpr = fmt.Sprintf(exprToNull, toRowExpr)
			toModelExpr = fmt.Sprintf(exprFromNull, toModelExpr)
			hasNullable = true
		}
		rowFields = append(rowFields, rowField{GoName: c.GoName, GoType: rowType, Tag: fmt.Sprintf(tagDB, c.Column)})
		toRow = append(toRow, assign{Field: c.GoName, Expr: toRowExpr})
		toModel = append(toModel, assign{Field: c.GoName, Expr: toModelExpr})
	}

	// Option-injected columns belong to the row and the model, but not the insert
	// path (the SQL sets timestamps via now()).
	for _, oc := range optionColumns(e.Options) {
		gn := pascalCase(oc.Column)
		impSet[importTime] = struct{}{}
		rowFields = append(rowFields, rowField{GoName: gn, GoType: goTime, Tag: fmt.Sprintf(tagDB, oc.Column)})
		toModel = append(toModel, assign{Field: gn, Expr: fmt.Sprintf(exprRowField, gn)})
	}

	selectCols := make([]string, 0, len(specCols)+2)
	for _, c := range specCols {
		selectCols = append(selectCols, c.Column)
	}
	for _, oc := range optionColumns(e.Options) {
		selectCols = append(selectCols, oc.Column)
	}

	// INSERT: spec fields take placeholders; timestamps default to now().
	_, pkGenerated := keyGenerator(pk)
	pkInCode := pkGenerated && pk.Type == spec.TypeUUID
	pkInDB := pkGenerated && !pkInCode
	insCols := make([]string, 0, len(specCols)+2)
	insPh := make([]string, 0, len(specCols)+2)
	insArgs := make([]string, 0, len(specCols))
	var ret []string
	for _, c := range specCols {
		if pkInDB && c.Column == pkCol {
			ret = append(ret, c.Column)
			continue
		}
		insCols = append(insCols, c.Column)
		insPh = append(insPh, fmt.Sprintf("$%d", len(insPh)+1))
		insArgs = append(insArgs, fmt.Sprintf(exprRowField, c.GoName))
	}
	if ts {
		insCols = append(insCols, colCreatedAt, colUpdatedAt)
		insPh = append(insPh, "now()", "now()")
		ret = append(ret, colCreatedAt, colUpdatedAt)
	}
	createSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(insCols, ", "), strings.Join(insPh, ", "))
	var createScan string
	if len(ret) > 0 {
		createSQL += fmt.Sprintf(clauseReturning, strings.Join(ret, ", "))
		createScan = scanList(ret)
	}

	getSQL := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1",
		strings.Join(selectCols, ", "), table, pkCol)

	var listFilterData []repoFilter
	for _, lf := range filters {
		listFilterData = append(listFilterData, repoFilter{GoName: lf.GoName, Clause: fmt.Sprintf(exprWhereEqual, snakeCase(lf.Field.Name))})
	}
	if len(listFilterData) > 0 {
		impSet[importStrings] = struct{}{}
	}
	listSorts := sortOptions(e)
	for i := range listSorts {
		listSorts[i].Name = qualified(s, listSorts[i].Name)
	}
	listDynamic := len(listFilterData) > 0 || len(listSorts) > 0

	listSQL := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT $1 OFFSET $2",
		strings.Join(selectCols, ", "), table, pkCol)
	if listDynamic {
		listSQL = fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectCols, ", "), table)
	}

	// UPDATE: non-PK fields get placeholders $1..$n, the PK gets $n+1.
	setClauses := make([]string, 0, len(update)+1)
	updArgs := make([]string, 0, len(update)+1)
	for i, c := range update {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", c.Column, i+1))
		updArgs = append(updArgs, fmt.Sprintf(exprRowField, c.GoName))
	}
	if ts {
		setClauses = append(setClauses, colUpdatedAt+" = now()")
	}
	updArgs = append(updArgs, fmt.Sprintf(exprRowField, pkGoName))

	var updateSQL, updateScan string
	noUpdateColumns := len(setClauses) == 0
	if noUpdateColumns {
		// Nothing writable (a primary-key-only entity without timestamps): an
		// empty SET would be invalid SQL, so Update degrades to an existence
		// check by primary key that still returns ErrNotFound for a missing row.
		updateSQL = fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1", table, pkCol)
	} else {
		updateSQL = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d",
			table, strings.Join(setClauses, ", "), pkCol, len(update)+1)
		if ts {
			ret := []string{colCreatedAt, colUpdatedAt}
			updateSQL += fmt.Sprintf(clauseReturning, strings.Join(ret, ", "))
			updateScan = scanList(ret)
		}
	}

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", table, pkCol)

	// Imports were gathered from every row-field type above (the primary-key type
	// among them, for the Get/Delete signatures) alongside the always-needed
	// context/database/sql/errors/fmt/sqlx packages.
	imports := groupImports(impSet, s.Module)

	return repoData{
		Package:         pkgPostgres,
		Domain:          s.Package,
		Struct:          name,
		Model:           qualified(s, name),
		LowerStruct:     strings.ToLower(name),
		Repo:            qualified(s, fmt.Sprintf(nameRepo, name)),
		Receiver:        fmt.Sprintf(nameRepo, name),
		Constructor:     fmt.Sprintf(nameRepoCtor, name),
		PKGoType:        gt.expr,
		HasTimestamps:   ts,
		GenerateUUID:    pkInCode,
		PKGoName:        pkGoName,
		NoUpdateColumns: noUpdateColumns,
		HasNullable:     hasNullable,
		RowStruct:       fmt.Sprintf(nameRow, unexport(name)),
		NewRowFunc:      fmt.Sprintf(nameNewRow, name),
		RowFields:       rowFields,
		ToRow:           toRow,
		ToModel:         toModel,
		CreateSQL:       createSQL,
		GetSQL:          getSQL,
		ListSQL:         listSQL,
		ListParams:      qualified(s, fmt.Sprintf(nameListParams, name)),
		ListDynamic:     listDynamic,
		ListFilters:     listFilterData,
		ListSorts:       listSorts,
		ListOrder:       pkCol,
		UpdateSQL:       updateSQL,
		DeleteSQL:       deleteSQL,
		InsertArgs:      strings.Join(insArgs, argSep),
		UpdateArgs:      strings.Join(updArgs, argSep),
		CreateScan:      createScan,
		UpdateScan:      updateScan,
		Imports:         imports,
	}, nil
}
