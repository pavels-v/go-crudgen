package generator

import (
	"fmt"

	"go-crudgen/internal/spec"
)

// goType describes how a spec field type renders in Go: the type expression and
// the import path it needs (empty for builtins).
type goType struct {
	expr string
	imp  string // import path, "" for builtins
}

// isNullable reports whether a field maps to a nullable column. It is the single
// source of truth shared by the model (pointer field), the repository (sql.Null
// column), and the migration (absence of a NOT NULL constraint): a column is
// nullable unless it is required or part of the primary key.
func isNullable(f spec.Field) bool {
	return !f.Required && !f.Primary
}

// scalarType maps a non-reference field type to its Go representation. Stdlib
// types are used where they fit; uuid and decimal use the established libraries
// because the stdlib has no equivalent.
func scalarType(t string) (goType, bool) {
	switch t {
	case "string", "text":
		return goType{expr: "string"}, true
	case "int":
		return goType{expr: "int"}, true
	case "int64":
		return goType{expr: "int64"}, true
	case "float":
		return goType{expr: "float64"}, true
	case "bool":
		return goType{expr: "bool"}, true
	case "decimal":
		return goType{expr: "decimal.Decimal", imp: "github.com/shopspring/decimal"}, true
	case "date", "datetime":
		return goType{expr: "time.Time", imp: "time"}, true
	case "uuid":
		return goType{expr: "uuid.UUID", imp: "github.com/google/uuid"}, true
	case "json":
		return goType{expr: "json.RawMessage", imp: "encoding/json"}, true
	}
	return goType{}, false
}

// pkParse describes how a primary key of the given Go type is parsed from the
// `{id}` path segment inside a handler.
type pkParse struct {
	expr     string // expression yielding the id (and an error when needsErr)
	needsErr bool   // false for string, which needs no parsing
	imp      string // import the parse expression needs ("" for none)
}

// pkParser returns how to parse a path id into the given Go primary-key type.
// ok is false for types we do not generate handlers for (decimal, time, json).
func pkParser(goExpr string) (pkParse, bool) {
	switch goExpr {
	case "string":
		return pkParse{expr: `r.PathValue("id")`}, true
	case "int":
		return pkParse{expr: `strconv.Atoi(r.PathValue("id"))`, needsErr: true, imp: "strconv"}, true
	case "int64":
		return pkParse{expr: `strconv.ParseInt(r.PathValue("id"), 10, 64)`, needsErr: true, imp: "strconv"}, true
	case "uuid.UUID":
		return pkParse{expr: `uuid.Parse(r.PathValue("id"))`, needsErr: true, imp: "github.com/google/uuid"}, true
	}
	return pkParse{}, false
}

// fieldType resolves a field's Go type. For references it derives the type from
// the target entity's single primary-key field (validation guarantees the
// target exists and is not composite).
func fieldType(f spec.Field, byName map[string]*spec.Entity) (goType, error) {
	if f.Type != "references" {
		gt, ok := scalarType(f.Type)
		if !ok {
			return goType{}, fmt.Errorf("unsupported field type %q", f.Type)
		}
		return gt, nil
	}

	target := byName[f.Target]
	pk := target.PrimaryKey()[0]
	if pk.Type == "references" {
		return goType{}, fmt.Errorf("reference to %q whose primary key %q is itself a reference (not supported)", f.Target, pk.Name)
	}
	gt, ok := scalarType(pk.Type)
	if !ok {
		return goType{}, fmt.Errorf("reference to %q has primary key of unsupported type %q", f.Target, pk.Type)
	}
	return gt, nil
}
