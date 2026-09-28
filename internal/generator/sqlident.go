package generator

import (
	"strconv"

	"github.com/pavels-v/go-crudgen/internal/spec"
)

var reservedSQL = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
	"all": {}, "analyse": {}, "analyze": {}, "and": {}, "any": {}, "array": {}, "as": {}, "asc": {},
	"asymmetric": {}, "authorization": {}, "binary": {}, "both": {}, "case": {}, "cast": {}, "check": {},
	"collate": {}, "collation": {}, "column": {}, "concurrently": {}, "constraint": {}, "create": {},
	"cross": {}, "current_catalog": {}, "current_date": {}, "current_role": {}, "current_schema": {},
	"current_time": {}, "current_timestamp": {}, "current_user": {}, "default": {}, "deferrable": {},
	"desc": {}, "distinct": {}, "do": {}, "else": {}, "end": {}, "except": {}, "false": {}, "fetch": {},
	"for": {}, "foreign": {}, "freeze": {}, "from": {}, "full": {}, "grant": {}, "group": {}, "having": {},
	"ilike": {}, "in": {}, "initially": {}, "inner": {}, "intersect": {}, "into": {}, "is": {}, "isnull": {},
	"join": {}, "lateral": {}, "leading": {}, "left": {}, "like": {}, "limit": {}, "localtime": {},
	"localtimestamp": {}, "natural": {}, "not": {}, "notnull": {}, "null": {}, "offset": {}, "on": {},
	"only": {}, "or": {}, "order": {}, "outer": {}, "overlaps": {}, "placing": {}, "primary": {},
	"references": {}, "returning": {}, "right": {}, "select": {}, "session_user": {}, "similar": {},
	"some": {}, "symmetric": {}, "system_user": {}, "table": {}, "tablesample": {}, "then": {}, "to": {},
	"trailing": {}, "true": {}, "union": {}, "unique": {}, "user": {}, "using": {}, "variadic": {},
	"verbose": {}, "when": {}, "where": {}, "window": {}, "with": {},
}

func sqlIdent(name string) string {
	if _, ok := reservedSQL[name]; ok {
		return strconv.Quote(name)
	}

	return name
}

func tableName(e *spec.Entity) string {
	return snakeCase(plural(e.Name, e.Plural))
}
