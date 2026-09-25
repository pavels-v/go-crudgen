package generator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"
)

const (
	extGo      = ".go"
	blankIdent = "_"
)

func checkCollisions(files []genFile) error {
	fset := token.NewFileSet()
	paths := make(map[string]string, len(files))
	decls := make(map[string]map[string]string)
	for _, f := range files {
		key := strings.ToLower(f.Path)
		if prev, ok := paths[key]; ok {
			return fmt.Errorf("generated files %s and %s share one path", prev, f.Path)
		}
		paths[key] = f.Path

		if !strings.HasSuffix(f.Path, extGo) {
			continue
		}
		file, err := parser.ParseFile(fset, f.Path, f.Src, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", f.Path, err)
		}
		dir := path.Dir(f.Path)
		if decls[dir] == nil {
			decls[dir] = make(map[string]string)
		}
		for _, name := range topLevelNames(file) {
			if prev, ok := decls[dir][name]; ok {
				return fmt.Errorf("%s is declared in both %s and %s", name, prev, f.Path)
			}
			decls[dir][name] = f.Path
		}
	}
	return nil
}

func topLevelNames(f *ast.File) []string {
	var names []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					names = append(names, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.Name != blankIdent {
							names = append(names, n.Name)
						}
					}
				}
			}
		}
	}
	return names
}
