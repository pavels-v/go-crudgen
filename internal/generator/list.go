package generator

import (
	"fmt"

	"github.com/pavels-v/go-crudgen/internal/spec"
)

const (
	nameListParams = "%sListParams"
	nameQueryParam = "query%s%s"
	nameCursor     = "%sCursor"
	nameSortDir    = "SortDir"
)

const (
	exprWhereEqual = "%s = ?"
	exprOrderDesc  = "%s DESC"
	exprParseText  = parseText + "[%s]"
	exprAfterKey   = "%s %s ?"
	exprAfterPair  = "(%s, %s) %s (?, ?)"
	exprAfterField = "p.After.%s"
	opGreater      = ">"
	opLess         = "<"
)

const (
	parseString = "parseString"
	parseInt32  = "parseInt32"
	parseInt64  = "parseInt64"
	parseText   = "parseText"
	parseBool   = "strconv.ParseBool"
)

type listFilter struct {
	Field  spec.Field
	GoName string
	Type   goType
}

type ordering struct {
	Asc       string // ORDER BY, e.g. "title, id"
	Desc      string // ORDER BY, e.g. "title DESC, id DESC"
	AfterAsc  string // cursor WHERE, e.g. "(title, id) > (?, ?)"
	AfterDesc string // cursor WHERE, e.g. "(title, id) < (?, ?)"
	AfterArgs string // cursor WHERE args, e.g. "p.After.Title, p.After.ID"
}

func listFilters(e *spec.Entity, byName map[string]*spec.Entity) ([]listFilter, error) {
	var out []listFilter
	for _, f := range e.Fields {
		if !f.Filter {
			continue
		}

		gt, err := fieldType(f, byName)
		if err != nil {
			return nil, err
		}

		out = append(out, listFilter{Field: f, GoName: pascalCase(f.Name), Type: gt})
	}

	return out, nil
}

func listOrder(e *spec.Entity) ordering {
	pk := e.PrimaryKey()[0]

	f, _ := e.OrderField()
	if f.Primary {
		return keyOrder(pk)
	}

	return fieldOrder(f, pk)
}

func keyOrder(pk spec.Field) ordering {
	col := sqlIdent(snakeCase(pk.Name))

	return ordering{
		Asc:       col,
		Desc:      fmt.Sprintf(exprOrderDesc, col),
		AfterAsc:  fmt.Sprintf(exprAfterKey, col, opGreater),
		AfterDesc: fmt.Sprintf(exprAfterKey, col, opLess),
		AfterArgs: fmt.Sprintf(exprAfterField, pascalCase(pk.Name)),
	}
}

func fieldOrder(f, pk spec.Field) ordering {
	col, pkCol := sqlIdent(snakeCase(f.Name)), sqlIdent(snakeCase(pk.Name))

	return ordering{
		Asc:       col + listSep + pkCol,
		Desc:      fmt.Sprintf(exprOrderDesc, col) + listSep + fmt.Sprintf(exprOrderDesc, pkCol),
		AfterAsc:  fmt.Sprintf(exprAfterPair, col, pkCol, opGreater),
		AfterDesc: fmt.Sprintf(exprAfterPair, col, pkCol, opLess),
		AfterArgs: fmt.Sprintf(exprAfterField, pascalCase(f.Name)) + listSep + fmt.Sprintf(exprAfterField, pascalCase(pk.Name)),
	}
}

func cursorFields(e *spec.Entity) []spec.Field {
	pk := e.PrimaryKey()[0]

	f, _ := e.OrderField()
	if f.Primary {
		return []spec.Field{pk}
	}

	return []spec.Field{f, pk}
}

func queryParser(gt goType, s *spec.Spec) (parse, imp string) {
	switch gt.expr {
	case goString:
		return parseString, ""
	case goInt32:
		return parseInt32, ""
	case goInt64:
		return parseInt64, ""
	case goBool:
		return parseBool, importStrconv
	}

	q := gt.outside(s)

	return fmt.Sprintf(exprParseText, q.expr), q.imp
}
