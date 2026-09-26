package generator

import (
	"fmt"

	"go-crudgen/internal/spec"
)

const (
	nameListParams = "%sListParams"
	nameSortType   = "%sSort"
	nameSortAsc    = "%sSort%s"
	nameSortDesc   = "%sSort%sDesc"
	nameQueryParam = "query%s%s"
	sortDescPrefix = "-"
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
	Name    string // domain constant, e.g. "PostSortTitleDesc"
	Value   string // ?sort= value, e.g. "-title"
	OrderBy string // e.g. "title DESC, id"
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
		col, gn := snakeCase(f.Name), pascalCase(f.Name)
		asc, desc := col, fmt.Sprintf(exprOrderDesc, col)
		if !f.Primary {
			asc += argSep + pkCol
			desc += argSep + pkCol
		}
		out = append(out,
			sortOption{Name: fmt.Sprintf(nameSortAsc, name, gn), Value: f.Name, OrderBy: asc},
			sortOption{Name: fmt.Sprintf(nameSortDesc, name, gn), Value: sortDescPrefix + f.Name, OrderBy: desc},
		)
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
