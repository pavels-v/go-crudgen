package generator

import (
	"fmt"
	"strings"

	"go-crudgen/internal/spec"
)

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
	importBase64      = "encoding/base64"
	importOS          = "os"
	importOSSignal    = "os/signal"
	importSyscall     = "syscall"
	importGoose       = "github.com/pressly/goose/v3"
)

const (
	tagDB       = "db:%q"
	tagJSON     = "json:%q"
	tagValidate = " validate:%q"
	jsonOmit    = ",omitzero"
)

const (
	clauseReturning = " RETURNING %s"
	exprSetNow      = "%s = " + sqlNow
)

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
	listSep        = ", "
)

const (
	nameRow    = "%sRow"
	nameNewRow = "new%sRow"
)

// dbData is the template input for postgres/db.go.
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
		Domain:       domainAlias,
		DriverName:   driverName,
		DriverImport: driverImp,
	}
}

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

// scanList renders the scan-target list for a set of columns, e.g. ["created_at"]
// -> "&m.CreatedAt". It shares the column list with the SQL builder so a
// RETURNING clause and its Scan targets can never drift in arity or order.
func scanList(goNames []string) string {
	out := make([]string, len(goNames))
	for i, n := range goNames {
		out[i] = fmt.Sprintf(exprScanTarget, n)
	}

	return strings.Join(out, listSep)
}

// repoData is the template input for one entity's PostgreSQL repository file.
// The SQL statements are built here rather than in the template so the column
// math (placeholders, ordering, generated values) stays testable.
type repoData struct {
	Package         string
	Imports         []string
	Domain          string
	Struct          string
	Model           string // qualified domain model, e.g. "domain.Post"
	LowerStruct     string // struct name lower-cased, for error messages
	Repo            string // qualified interface name, e.g. "domain.PostRepository"
	Receiver        string // concrete type, e.g. "PostRepository"
	Constructor     string // e.g. "NewPostRepository"
	PKGoType        string
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
	ListSQL   string // SELECT ... FROM prefix; WHERE, ORDER BY and paging are appended
	UpdateSQL string
	DeleteSQL string

	ListParams  string // qualified domain params, e.g. "domain.PostListParams"
	ListDynamic bool   // WHERE is assembled from ListFilters and the cursor
	ListFilters []repoFilter
	ListOrder   ordering
	Cursor      bool // keyset pagination by p.After instead of OFFSET

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
	pk, gt, err := serveKey(e, byName)
	if err != nil {
		return repoData{}, err
	}

	filters, err := listFilters(e, byName)
	if err != nil {
		return repoData{}, err
	}

	name := pascalCase(e.Name)
	table := sqlIdent(tableName(e))
	pkCol := snakeCase(pk.Name)
	pkSQL := sqlIdent(pkCol)
	pkGoName := pascalCase(pk.Name)

	impSet := map[string]struct{}{
		importContext: {}, importDatabaseSQL: {}, importErrors: {}, importFmt: {}, importSQLx: {}, s.Module: {},
	}

	// Persisted columns in struct order. Each column contributes a row-struct
	// field and a model<->row conversion; nullable columns become sql.Null[T] on
	// the row (pointer on the model), bridged by the generic toNull/fromNull
	// helpers. Generated columns are set by the SQL, so they are never written
	// from the model.
	type col struct {
		Column   string
		SQL      string
		GoName   string
		Generate string
	}
	specCols := make([]col, 0, len(e.Fields))
	rowFields := make([]rowField, 0, len(e.Fields))
	toRow := make([]assign, 0, len(e.Fields))
	toModel := make([]assign, 0, len(e.Fields))
	var update []col
	var hasNullable bool

	for _, f := range e.Fields {
		c := col{Column: snakeCase(f.Name), GoName: pascalCase(f.Name), Generate: f.Generate}
		c.SQL = sqlIdent(c.Column)

		specCols = append(specCols, c) // the primary key is supplied by the caller
		if !f.Primary && f.Generate == "" {
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
		if f.Generate == "" {
			toRow = append(toRow, assign{Field: c.GoName, Expr: toRowExpr})
		}

		toModel = append(toModel, assign{Field: c.GoName, Expr: toModelExpr})
	}

	selectCols := make([]string, 0, len(specCols))
	var generated, generatedGo []string

	for _, c := range specCols {
		selectCols = append(selectCols, c.SQL)
		if c.Generate != "" {
			generated = append(generated, c.SQL)
			generatedGo = append(generatedGo, c.GoName)
		}
	}

	// INSERT: fields take placeholders; generated columns take now() and come
	// back through RETURNING along with a database-generated key.
	_, pkGenerated := keyGenerator(pk)
	pkInCode := pkGenerated && pk.Type == spec.TypeUUID
	pkInDB := pkGenerated && !pkInCode
	insCols := make([]string, 0, len(specCols))
	insPh := make([]string, 0, len(specCols))
	insArgs := make([]string, 0, len(specCols))
	var ret, retGo []string

	for _, c := range specCols {
		switch {
		case pkInDB && c.Column == pkCol:
			ret = append(ret, c.SQL)
			retGo = append(retGo, c.GoName)

			continue
		case c.Generate != "":
			insCols = append(insCols, c.SQL)
			insPh = append(insPh, sqlNow)
			ret = append(ret, c.SQL)
			retGo = append(retGo, c.GoName)

			continue
		}

		insCols = append(insCols, c.SQL)
		insPh = append(insPh, fmt.Sprintf("$%d", len(insArgs)+1))
		insArgs = append(insArgs, fmt.Sprintf(exprRowField, c.GoName))
	}

	createSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(insCols, listSep), strings.Join(insPh, listSep))
	if len(insCols) == 0 {
		createSQL = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", table)
	}

	var createScan string
	if len(ret) > 0 {
		createSQL += fmt.Sprintf(clauseReturning, strings.Join(ret, listSep))
		createScan = scanList(retGo)
	}

	getSQL := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1",
		strings.Join(selectCols, listSep), table, pkSQL)

	var listFilterData []repoFilter
	for _, lf := range filters {
		listFilterData = append(listFilterData, repoFilter{GoName: lf.GoName, Clause: fmt.Sprintf(exprWhereEqual, sqlIdent(snakeCase(lf.Field.Name)))})
	}

	listDynamic := len(listFilterData) > 0 || e.CursorPagination()
	if listDynamic {
		impSet[importStrings] = struct{}{}
	}

	listOrd := listOrder(e)
	listSQL := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectCols, listSep), table)

	// UPDATE: non-PK fields get placeholders $1..$n, the PK gets $n+1; on_write
	// columns are reset to now(), and every generated column is returned so the
	// response carries the stored values.
	setClauses := make([]string, 0, len(update)+len(generated))
	updArgs := make([]string, 0, len(update)+1)

	for i, c := range update {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", c.SQL, i+1))
		updArgs = append(updArgs, fmt.Sprintf(exprRowField, c.GoName))
	}

	for _, c := range specCols {
		if c.Generate == spec.GenerateOnWrite {
			setClauses = append(setClauses, fmt.Sprintf(exprSetNow, c.SQL))
		}
	}

	updArgs = append(updArgs, fmt.Sprintf(exprRowField, pkGoName))

	var updateSQL, updateScan string

	noUpdateColumns := len(setClauses) == 0 && len(generated) == 0
	switch {
	case noUpdateColumns:
		// Nothing writable (a primary-key-only entity): an empty SET would be
		// invalid SQL, so Update degrades to an existence check by primary key
		// that still returns ErrNotFound for a missing row.
		updateSQL = fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1", table, pkSQL)
	case len(setClauses) == 0:
		// Only on_create columns besides the key: nothing to write, but the
		// response still needs their stored values.
		updateSQL = fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1", strings.Join(generated, listSep), table, pkSQL)
		updateScan = scanList(generatedGo)
	default:
		updateSQL = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d",
			table, strings.Join(setClauses, listSep), pkSQL, len(update)+1)
		if len(generated) > 0 {
			updateSQL += fmt.Sprintf(clauseReturning, strings.Join(generated, listSep))
			updateScan = scanList(generatedGo)
		}
	}

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", table, pkSQL)

	// Imports were gathered from every row-field type above (the primary-key type
	// among them, for the Get/Delete signatures) alongside the always-needed
	// context/database/sql/errors/fmt/sqlx packages.
	imports := groupImports(impSet, s.Module)

	return repoData{
		Package:         pkgPostgres,
		Domain:          domainAlias,
		Struct:          name,
		Model:           qualified(name),
		LowerStruct:     strings.ToLower(name),
		Repo:            qualified(fmt.Sprintf(nameRepo, name)),
		Receiver:        fmt.Sprintf(nameRepo, name),
		Constructor:     fmt.Sprintf(nameRepoCtor, name),
		PKGoType:        gt.expr,
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
		ListParams:      qualified(fmt.Sprintf(nameListParams, name)),
		ListDynamic:     listDynamic,
		ListFilters:     listFilterData,
		ListOrder:       listOrd,
		Cursor:          e.CursorPagination(),
		UpdateSQL:       updateSQL,
		DeleteSQL:       deleteSQL,
		InsertArgs:      strings.Join(insArgs, listSep),
		UpdateArgs:      strings.Join(updArgs, listSep),
		CreateScan:      createScan,
		UpdateScan:      updateScan,
		Imports:         imports,
	}, nil
}
