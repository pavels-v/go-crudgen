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

// repoData is the template input for one entity's PostgreSQL repository file.
// The SQL statements are built here rather than in the template so the column
// math (placeholders, ordering, soft-delete/timestamp handling) stays testable.
type repoData struct {
	Package       string
	Imports       []string
	Struct        string
	LowerStruct   string // struct name lower-cased, for error messages
	Repo          string // interface name, e.g. "PostRepository"
	Receiver      string // concrete type, e.g. "PostgresPostRepository"
	Constructor   string // e.g. "NewPostgresPostRepository"
	PKGoType      string
	HasTimestamps bool

	CreateSQL string
	GetSQL    string
	ListSQL   string
	UpdateSQL string
	DeleteSQL string

	InsertArgs  string // create args, e.g. "m.ID, m.Title"
	UpdateArgs  string // update args: non-PK fields then PK, e.g. "m.Title, m.ID"
	ScanTargets string // all columns, e.g. "&m.ID, &m.Title, &m.CreatedAt"
}

// repoInfo builds the repository template data for an entity, reporting ok=false
// for the same entities handlerInfo skips: a repository whose interface is never
// generated would have nothing to implement.
func repoInfo(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity) (repoData, bool, error) {
	pk := e.PrimaryKey()
	if len(pk) != 1 {
		return repoData{}, false, nil
	}
	gt, err := fieldType(pk[0], byName)
	if err != nil {
		return repoData{}, false, err
	}
	if _, ok := pkParser(gt.expr); !ok {
		return repoData{}, false, nil
	}

	name := pascalCase(e.Name)
	table := plural(e.Name, e.Plural)
	pkCol := snakeCase(pk[0].Name)
	soft := e.Options.SoftDelete
	ts := e.Options.Timestamps

	// Persisted columns in struct order: spec fields first, then the
	// option-injected timestamp and soft-delete columns.
	type col struct{ Column, GoName string }
	var all, insert, update []col
	for _, f := range e.Fields {
		c := col{Column: snakeCase(f.Name), GoName: pascalCase(f.Name)}
		all = append(all, c)
		insert = append(insert, c) // the primary key is supplied by the caller
		if !f.Primary {
			update = append(update, c)
		}
	}
	if ts {
		all = append(all, col{"created_at", "CreatedAt"}, col{"updated_at", "UpdatedAt"})
	}
	if soft {
		all = append(all, col{"deleted_at", "DeletedAt"})
	}

	selectCols := make([]string, len(all))
	scan := make([]string, len(all))
	for i, c := range all {
		selectCols[i] = c.Column
		scan[i] = "&m." + c.GoName
	}

	// INSERT: spec fields take placeholders; timestamps default to now().
	insCols := make([]string, len(insert))
	insPh := make([]string, len(insert))
	insArgs := make([]string, len(insert))
	for i, c := range insert {
		insCols[i] = c.Column
		insPh[i] = fmt.Sprintf("$%d", i+1)
		insArgs[i] = "m." + c.GoName
	}
	if ts {
		insCols = append(insCols, "created_at", "updated_at")
		insPh = append(insPh, "now()", "now()")
	}
	createSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(insCols, ", "), strings.Join(insPh, ", "))
	if ts {
		createSQL += " RETURNING created_at, updated_at"
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
	n := 0
	for _, c := range update {
		n++
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", c.Column, n))
		updArgs = append(updArgs, "m."+c.GoName)
	}
	if ts {
		setClauses = append(setClauses, "updated_at = now()")
	}
	n++
	updateSQL := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d%s",
		table, strings.Join(setClauses, ", "), pkCol, n, softFilter)
	updArgs = append(updArgs, "m."+pascalCase(pk[0].Name))
	if ts {
		updateSQL += " RETURNING updated_at"
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
		Package:       s.Package,
		Struct:        name,
		LowerStruct:   strings.ToLower(name),
		Repo:          name + "Repository",
		Receiver:      "Postgres" + name + "Repository",
		Constructor:   "NewPostgres" + name + "Repository",
		PKGoType:      gt.expr,
		HasTimestamps: ts,
		CreateSQL:     createSQL,
		GetSQL:        getSQL,
		ListSQL:       listSQL,
		UpdateSQL:     updateSQL,
		DeleteSQL:     deleteSQL,
		InsertArgs:    strings.Join(insArgs, ", "),
		UpdateArgs:    strings.Join(updArgs, ", "),
		ScanTargets:   strings.Join(scan, ", "),
		Imports:       imports,
	}, true, nil
}
