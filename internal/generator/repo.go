package generator

import (
	"fmt"
	"sort"
	"strings"

	"go-crudgen/internal/spec"
)

// dbData is the template input for the package-wide db.gen.go connection file.
type dbData struct {
	Package      string
	DriverName   string // database/sql driver name, e.g. "pgx" or "postgres"
	DriverImport string // blank-imported driver package
}

// driverInfo maps a --driver choice to its database/sql driver name and the
// package that must be blank-imported to register it. An empty choice defaults
// to pgx. ok is false for unsupported drivers.
func driverInfo(driver string) (name, imp string, ok bool) {
	switch driver {
	case "", "pgx":
		return "pgx", "github.com/jackc/pgx/v5/stdlib", true
	case "pq":
		return "postgres", "github.com/lib/pq", true
	}
	return "", "", false
}

// optionColumn describes a column injected by an EntityOptions toggle.
type optionColumn struct {
	Column  string
	GoName  string
	GoType  string
	JSONTag string
}

// optionColumns returns the columns injected by an entity's options, in the
// order they are appended to the model struct (timestamps, then soft-delete).
// renderModel and repoInfo both consume it so the model struct and the
// generated SQL agree on which columns exist and their order.
func optionColumns(o spec.EntityOptions) []optionColumn {
	var cols []optionColumn
	if o.Timestamps {
		cols = append(cols,
			optionColumn{"created_at", "CreatedAt", "time.Time", `json:"created_at"`},
			optionColumn{"updated_at", "UpdatedAt", "time.Time", `json:"updated_at"`},
		)
	}
	if o.SoftDelete {
		cols = append(cols, optionColumn{"deleted_at", "DeletedAt", "*time.Time", `json:"deleted_at,omitempty"`})
	}
	return cols
}

// scanList renders the scan-target list for a set of columns, e.g. ["created_at"]
// -> "&m.CreatedAt". It shares the column list with the SQL builder so a
// RETURNING clause and its Scan targets can never drift in arity or order.
func scanList(cols []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = "&m." + pascalCase(c)
	}
	return strings.Join(out, ", ")
}

// repoData is the template input for one entity's PostgreSQL repository file.
// The SQL statements are built here rather than in the template so the column
// math (placeholders, ordering, soft-delete/timestamp handling) stays testable.
type repoData struct {
	Package         string
	Imports         []string
	Struct          string
	LowerStruct     string // struct name lower-cased, for error messages
	Repo            string // interface name, e.g. "PostRepository"
	Receiver        string // concrete type, e.g. "PostgresPostRepository"
	Constructor     string // e.g. "NewPostgresPostRepository"
	PKGoType        string
	HasTimestamps   bool
	NoUpdateColumns bool // entity has no writable columns; Update is an existence check

	CreateSQL string
	GetSQL    string
	ListSQL   string
	UpdateSQL string
	DeleteSQL string

	InsertArgs string // create args, e.g. "m.ID, m.Title"
	UpdateArgs string // update args: non-PK fields then PK, e.g. "m.Title, m.ID"
	CreateScan string // scan targets for the create RETURNING clause
	UpdateScan string // scan targets for the update RETURNING clause
}

// repoInfo builds the repository template data for an entity, reporting ok=false
// for the same entities handlerInfo skips (see serveKey): a repository whose
// interface is never generated would have nothing to implement.
func repoInfo(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) (repoData, bool, error) {
	pk, gt, _, ok, err := serveKey(e, byName)
	if err != nil || !ok {
		return repoData{}, ok, err
	}

	name := pascalCase(e.Name)
	table := plural(e.Name, e.Plural)
	pkCol := snakeCase(pk.Name)
	pkGoName := pascalCase(pk.Name)
	soft := e.Options.SoftDelete
	ts := e.Options.Timestamps

	// Persisted columns in struct order: spec fields first, then the
	// option-injected columns (shared with renderModel via optionColumns).
	type col struct{ Column, GoName string }
	var specCols, update []col
	for _, f := range e.Fields {
		c := col{Column: snakeCase(f.Name), GoName: pascalCase(f.Name)}
		specCols = append(specCols, c) // the primary key is supplied by the caller
		if !f.Primary {
			update = append(update, c)
		}
	}
	selectCols := make([]string, 0, len(specCols)+2)
	for _, c := range specCols {
		selectCols = append(selectCols, c.Column)
	}
	for _, oc := range optionColumns(e.Options) {
		selectCols = append(selectCols, oc.Column)
	}

	// INSERT: spec fields take placeholders; timestamps default to now().
	insCols := make([]string, len(specCols))
	insPh := make([]string, len(specCols))
	insArgs := make([]string, len(specCols))
	for i, c := range specCols {
		insCols[i] = c.Column
		insPh[i] = fmt.Sprintf("$%d", i+1)
		insArgs[i] = "m." + c.GoName
	}
	var createScan string
	if ts {
		insCols = append(insCols, "created_at", "updated_at")
		insPh = append(insPh, "now()", "now()")
	}
	createSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(insCols, ", "), strings.Join(insPh, ", "))
	if ts {
		ret := []string{"created_at", "updated_at"}
		createSQL += " RETURNING " + strings.Join(ret, ", ")
		createScan = scanList(ret)
	}

	softFilter := ""
	if soft {
		softFilter = " AND deleted_at IS NULL"
	}
	getSQL := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1%s",
		strings.Join(selectCols, ", "), table, pkCol, softFilter)

	listWhere := ""
	if soft {
		listWhere = " WHERE deleted_at IS NULL"
	}
	listSQL := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s LIMIT $1 OFFSET $2",
		strings.Join(selectCols, ", "), table, listWhere, pkCol)

	// UPDATE: non-PK fields get placeholders $1..$n, the PK gets $n+1.
	setClauses := make([]string, 0, len(update)+1)
	updArgs := make([]string, 0, len(update)+1)
	for i, c := range update {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", c.Column, i+1))
		updArgs = append(updArgs, "m."+c.GoName)
	}
	if ts {
		setClauses = append(setClauses, "updated_at = now()")
	}
	updArgs = append(updArgs, "m."+pkGoName)

	var updateSQL, updateScan string
	noUpdateColumns := len(setClauses) == 0
	if noUpdateColumns {
		// Nothing writable (a primary-key-only entity without timestamps): an
		// empty SET would be invalid SQL, so Update degrades to an existence
		// check by primary key that still returns ErrNotFound for a missing row.
		updateSQL = fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1%s", table, pkCol, softFilter)
	} else {
		updateSQL = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d%s",
			table, strings.Join(setClauses, ", "), pkCol, len(update)+1, softFilter)
		if ts {
			ret := []string{"updated_at"}
			updateSQL += " RETURNING " + strings.Join(ret, ", ")
			updateScan = scanList(ret)
		}
	}

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", table, pkCol)
	if soft {
		deleteSQL = fmt.Sprintf("UPDATE %s SET deleted_at = now() WHERE %s = $1 AND deleted_at IS NULL", table, pkCol)
	}

	// The primary-key type (uuid.UUID, etc.) appears in the Get/Delete signatures,
	// so its package must be imported alongside database/sql and sqlx.
	imports := []string{"context", "database/sql", "errors", "fmt"}
	if gt.imp != "" {
		imports = append(imports, gt.imp)
	}
	imports = append(imports, "github.com/jmoiron/sqlx")
	sort.Strings(imports)

	return repoData{
		Package:         s.Package,
		Struct:          name,
		LowerStruct:     strings.ToLower(name),
		Repo:            name + "Repository",
		Receiver:        "Postgres" + name + "Repository",
		Constructor:     "NewPostgres" + name + "Repository",
		PKGoType:        gt.expr,
		HasTimestamps:   ts,
		NoUpdateColumns: noUpdateColumns,
		CreateSQL:       createSQL,
		GetSQL:          getSQL,
		ListSQL:         listSQL,
		UpdateSQL:       updateSQL,
		DeleteSQL:       deleteSQL,
		InsertArgs:      strings.Join(insArgs, ", "),
		UpdateArgs:      strings.Join(updArgs, ", "),
		CreateScan:      createScan,
		UpdateScan:      updateScan,
		Imports:         imports,
	}, true, nil
}
