package generator

import (
	"fmt"

	"go-crudgen/internal/spec"
)

const (
	nameListParams = "%sListParams"
	nameSortType   = "%sSort"
	nameSortField  = "%sSort%s"
	nameQueryParam = "query%s%s"
)

const (
	exprWhereEqual = "%s = ?"
	exprOrderDesc  = "%s DESC"
	exprParseText  = "parseText[%s]"
)

const (
	parseString = "parseString"
	parseInt32  = "parseInt32"
	parseInt64  = "parseInt64"
	parseBool   = "strconv.ParseBool"
)

type listFilter struct {
	Field  spec.Field
	GoName string
	Type   goType
}

type sortOption struct {
	Name  string // domain constant, e.g. "PostSortTitle"
	Value string // ?sort= value, e.g. "title"
	Asc   string // ORDER BY, e.g. "title, id"
	Desc  string // ORDER BY, e.g. "title DESC, id DESC"
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

func sortOptions(e *spec.Entity) []sortOption {
	name := pascalCase(e.Name)
	pkCol := snakeCase(e.PrimaryKey()[0].Name)
	var out []sortOption
	for _, f := range e.Fields {
		if !f.Sort {
			continue
		}
		col := snakeCase(f.Name)
		asc, desc := col, fmt.Sprintf(exprOrderDesc, col)
		if !f.Primary {
			asc += argSep + pkCol
			desc += argSep + fmt.Sprintf(exprOrderDesc, pkCol)
		}
		out = append(out, sortOption{
			Name:  fmt.Sprintf(nameSortField, name, pascalCase(f.Name)),
			Value: f.Name,
			Asc:   asc,
			Desc:  desc,
		})
	}
	return out
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
