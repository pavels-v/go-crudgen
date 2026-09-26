package generator

import (
	"fmt"
	"strconv"

	"go-crudgen/internal/spec"
)

// goType describes how a spec field type renders in Go: the type expression and
// the import path it needs (empty for builtins).
type goType struct {
	expr   string
	imp    string // import path, "" for builtins
	domain bool
}

func (gt goType) outside(s *spec.Spec) goType {
	if !gt.domain {
		return gt
	}
	return goType{expr: fmt.Sprintf(exprQualified, s.Package, gt.expr), imp: s.Module}
}

// Go type expressions emitted for spec field types. scalarType produces these and
// pkParser matches against them, so sharing the constants keeps the two in sync.
const (
	goString  = "string"
	goInt32   = "int32"
	goInt64   = "int64"
	goFloat64 = "float64"
	goBool    = "bool"
	goDecimal = "decimal.Decimal"
	goTime    = "time.Time"
	goDate    = "Date"
	goUUID    = "uuid.UUID"
	goJSON    = "jsontext.Value"
)

// Import paths the generated code needs for particular field types.
const (
	importStrconv = "strconv"
	importTime    = "time"
	importUUID    = "github.com/google/uuid"
	importDecimal = "github.com/shopspring/decimal"
	importJSON    = "encoding/json/jsontext"
)

// PostgreSQL column types emitted for spec field types.
const (
	sqlText        = "TEXT"
	sqlInteger     = "INTEGER"
	sqlBigint      = "BIGINT"
	sqlDouble      = "DOUBLE PRECISION"
	sqlNumeric     = "NUMERIC"
	sqlBoolean     = "BOOLEAN"
	sqlDate        = "DATE"
	sqlTimestamptz = "TIMESTAMPTZ"
	sqlUUID        = "UUID"
	sqlJSONB       = "JSONB"
)

// typeInfo is the complete mapping for one scalar spec field type: its Go type
// expression and the import that type needs, plus its PostgreSQL column type.
type typeInfo struct {
	goExpr   string
	goImport string // "" for builtins
	sqlType  string
	domain   bool
}

// scalarTypes is the single source of truth mapping each non-reference spec field
// type to its Go and SQL representations. scalarType and sqlType both read from
// it, so a new field type is added in exactly one place.
var scalarTypes = map[string]typeInfo{ //nolint:gochecknoglobals // read-only lookup table
	spec.TypeString:   {goExpr: goString, sqlType: sqlText},
	spec.TypeText:     {goExpr: goString, sqlType: sqlText},
	spec.TypeInt32:    {goExpr: goInt32, sqlType: sqlInteger},
	spec.TypeInt64:    {goExpr: goInt64, sqlType: sqlBigint},
	spec.TypeFloat:    {goExpr: goFloat64, sqlType: sqlDouble},
	spec.TypeBool:     {goExpr: goBool, sqlType: sqlBoolean},
	spec.TypeDecimal:  {goExpr: goDecimal, goImport: importDecimal, sqlType: sqlNumeric},
	spec.TypeDate:     {goExpr: goDate, sqlType: sqlDate, domain: true},
	spec.TypeDatetime: {goExpr: goTime, goImport: importTime, sqlType: sqlTimestamptz},
	spec.TypeUUID:     {goExpr: goUUID, goImport: importUUID, sqlType: sqlUUID},
	spec.TypeJSON:     {goExpr: goJSON, goImport: importJSON, sqlType: sqlJSONB},
}

// isNullable reports whether a field maps to a nullable column. It is the single
// source of truth shared by the model (pointer field), the repository (sql.Null
// column), and the migration (absence of a NOT NULL constraint): a column is
// nullable unless it is required or part of the primary key.
func isNullable(f spec.Field) bool {
	return !f.Required && !f.Primary && f.Default == nil && f.Generate == ""
}

func hasRequestDefault(f spec.Field) bool {
	return !f.Required && !f.Primary && f.Default != nil
}

const (
	sqlGenUUID     = "DEFAULT gen_random_uuid()"
	sqlGenIdentity = "GENERATED ALWAYS AS IDENTITY"
)

var generatedKeys = map[string]string{ //nolint:gochecknoglobals // read-only lookup table
	spec.TypeUUID:  sqlGenUUID,
	spec.TypeInt32: sqlGenIdentity,
	spec.TypeInt64: sqlGenIdentity,
}

func keyGenerator(f spec.Field) (string, bool) {
	if !f.Primary {
		return "", false
	}
	g, ok := generatedKeys[f.Type]
	return g, ok
}

const (
	sqlNow = "now()"
	goNow  = "time.Now()"
)

func isNowDefault(f spec.Field) bool {
	return f.Type == spec.TypeDatetime && f.Default == spec.DefaultNow
}

func goDefault(f spec.Field) (string, error) {
	if isNowDefault(f) {
		return goNow, nil
	}
	return goLiteral(f.Default)
}

func goLiteral(v any) (string, error) {
	switch d := v.(type) {
	case bool:
		return strconv.FormatBool(d), nil
	case int:
		return strconv.Itoa(d), nil
	case float64:
		return strconv.FormatFloat(d, 'g', -1, 64), nil
	case string:
		return strconv.Quote(d), nil
	}
	return "", fmt.Errorf("unsupported default value %v (%T)", v, v)
}

// scalarType maps a non-reference field type to its Go representation, reading
// from the shared scalarTypes table. ok is false for an unknown type.
func scalarType(t string) (goType, bool) {
	ti, ok := scalarTypes[t]
	if !ok {
		return goType{}, false
	}
	return goType{expr: ti.goExpr, imp: ti.goImport, domain: ti.domain}, true
}

const exprQualified = "%s.%s"

const (
	exprPathValue = "r.PathValue(pathParamID)"
	exprParseInt  = "strconv.ParseInt(%s, 10, %d)"
	exprParseUUID = "uuid.Parse(%s)"
)

// pkParse describes how a primary key of the given Go type is parsed from the
// `{id}` path segment inside a handler.
type pkParse struct {
	expr     string // expression yielding the id (and an error when needsErr)
	needsErr bool   // false for string, which needs no parsing
	imp      string // import the parse expression needs ("" for none)
	cast     string // Go type to convert the parsed value to ("" when expr already yields the PK type)
}

// pkParser returns how to parse a path id into the given Go primary-key type.
// ok is false for types we do not generate handlers for (decimal, time, json).
func pkParser(goExpr string) (pkParse, bool) {
	id := exprPathValue
	switch goExpr {
	case goString:
		return pkParse{expr: id}, true
	case goInt32:
		// strconv has no parse-to-int32, so parse with a 32-bit size and cast.
		return pkParse{expr: fmt.Sprintf(exprParseInt, id, 32), needsErr: true, imp: importStrconv, cast: goInt32}, true
	case goInt64:
		return pkParse{expr: fmt.Sprintf(exprParseInt, id, 64), needsErr: true, imp: importStrconv}, true
	case goUUID:
		return pkParse{expr: fmt.Sprintf(exprParseUUID, id), needsErr: true, imp: importUUID}, true
	}
	return pkParse{}, false
}

// fieldType resolves a field's Go type. For references it derives the type from
// the target entity's single primary-key field (validation guarantees the
// target exists and is not composite).
func fieldType(f spec.Field, byName map[string]*spec.Entity) (goType, error) {
	if f.Type != spec.TypeReferences {
		gt, ok := scalarType(f.Type)
		if !ok {
			return goType{}, fmt.Errorf("unsupported field type %q", f.Type)
		}
		return gt, nil
	}

	target := byName[f.Target]
	pk := target.PrimaryKey()[0]
	if pk.Type == spec.TypeReferences {
		return goType{}, fmt.Errorf("reference to %q whose primary key %q is itself a reference (not supported)", f.Target, pk.Name)
	}
	gt, ok := scalarType(pk.Type)
	if !ok {
		return goType{}, fmt.Errorf("reference to %q has primary key of unsupported type %q", f.Target, pk.Type)
	}
	return gt, nil
}
