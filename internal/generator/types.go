package generator

import (
	"encoding/json/jsontext"
	"fmt"
	"strconv"
	"time"

	"github.com/pavels-v/go-crudgen/internal/spec"
)

// Go type expressions emitted for spec field types. scalarType produces these and
// pathAddressable matches against them, so sharing the constants keeps the two in sync.
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

// scalarTypes is the single source of truth mapping each non-reference spec field
// type to its Go and SQL representations. scalarType and sqlType both read from
// it, so a new field type is added in exactly one place.
var scalarTypes = map[string]typeInfo{ //nolint:gochecknoglobals // read-only lookup table
	spec.TypeString:   {goExpr: goString, sqlType: sqlText, sample: ""},
	spec.TypeText:     {goExpr: goString, sqlType: sqlText, sample: ""},
	spec.TypeInt32:    {goExpr: goInt32, sqlType: sqlInteger, sample: int32(0)},
	spec.TypeInt64:    {goExpr: goInt64, sqlType: sqlBigint, sample: int64(0)},
	spec.TypeFloat:    {goExpr: goFloat64, sqlType: sqlDouble, sample: float64(0)},
	spec.TypeBool:     {goExpr: goBool, sqlType: sqlBoolean, sample: false},
	spec.TypeDecimal:  {goExpr: goDecimal, goImport: importDecimal, sqlType: sqlNumeric, sample: struct{}{}},
	spec.TypeDate:     {goExpr: goDate, sqlType: sqlDate, domain: true, sample: time.Time{}},
	spec.TypeDatetime: {goExpr: goTime, goImport: importTime, sqlType: sqlTimestamptz, sample: time.Time{}},
	spec.TypeUUID:     {goExpr: goUUID, goImport: importUUID, sqlType: sqlUUID, sample: [16]byte{}},
	spec.TypeJSON:     {goExpr: goJSON, goImport: importJSON, sqlType: sqlJSONB, sample: jsontext.Value{}},
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

const (
	sqlNow = "now()"
	goNow  = "time.Now()"
)

const exprQualified = "%s.%s"

// goType describes how a spec field type renders in Go: the type expression and
// the import path it needs (empty for builtins).
type goType struct {
	expr   string
	imp    string // import path, "" for builtins
	domain bool
	sample any
}

func (gt goType) outside(s *spec.Spec) goType {
	if !gt.domain {
		return gt
	}

	return goType{expr: fmt.Sprintf(exprQualified, pkgDomain, gt.expr), imp: domainImport(s), sample: gt.sample}
}

// typeInfo is the complete mapping for one scalar spec field type: its Go type
// expression and the import that type needs, plus its PostgreSQL column type.
type typeInfo struct {
	goExpr   string
	goImport string // "" for builtins
	sqlType  string
	domain   bool
	sample   any
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

func keyGenerator(f spec.Field) (string, bool) {
	if !f.Primary {
		return "", false
	}

	g, ok := generatedKeys[f.Type]

	return g, ok
}

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
		return formatFloat(d), nil
	case string:
		return strconv.Quote(d), nil
	}

	return "", fmt.Errorf("unsupported default value %v (%T)", v, v)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// scalarType maps a non-reference field type to its Go representation, reading
// from the shared scalarTypes table. ok is false for an unknown type.
func scalarType(t string) (goType, bool) {
	ti, ok := scalarTypes[t]
	if !ok {
		return goType{}, false
	}

	return goType{expr: ti.goExpr, imp: ti.goImport, domain: ti.domain, sample: ti.sample}, true
}

// pathAddressable reports whether a primary key of the given Go type can be
// parsed from the {id} path segment.
func pathAddressable(goExpr string) bool {
	switch goExpr {
	case goString, goInt32, goInt64, goUUID:
		return true
	}

	return false
}

// fieldType resolves a field's Go type. For references it derives the type from
// the target entity's single primary-key field (validation guarantees the
// target exists and is not composite).
func fieldType(f spec.Field, byName map[string]*spec.Entity) (goType, error) {
	t, err := storedType(f, byName)
	if err != nil {
		return goType{}, err
	}

	gt, ok := scalarType(t)
	if !ok {
		return goType{}, fmt.Errorf("unsupported field type %q", t)
	}

	return gt, nil
}

func storedType(f spec.Field, byName map[string]*spec.Entity) (string, error) {
	if f.Type != spec.TypeReferences {
		return f.Type, nil
	}

	pk := byName[f.Target].PrimaryKey()[0]
	if pk.Type == spec.TypeReferences {
		return "", fmt.Errorf("reference to %q whose primary key %q is itself a reference (not supported)", f.Target, pk.Name)
	}

	return pk.Type, nil
}
