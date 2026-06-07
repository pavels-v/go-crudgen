package spec

import (
	"fmt"
	"strings"
)

// KnownTypes is the set of field types the generator understands.
var KnownTypes = map[string]struct{}{
	"string":     {},
	"text":       {},
	"int":        {},
	"int64":      {},
	"float":      {},
	"decimal":    {},
	"bool":       {},
	"date":       {},
	"datetime":   {},
	"uuid":       {},
	"json":       {},
	"references": {},
}

// Validate checks the spec for structural errors.
func (s *Spec) Validate() error {
	if strings.TrimSpace(s.Package) == "" {
		return fmt.Errorf("missing package name")
	}
	if len(s.Entities) == 0 {
		return fmt.Errorf("no entities defined")
	}

	seen := make(map[string]bool, len(s.Entities))
	for i := range s.Entities {
		e := &s.Entities[i]
		if e.Name == "" {
			return fmt.Errorf("entity #%d has no name", i+1)
		}
		if seen[e.Name] {
			return fmt.Errorf("duplicate entity %q", e.Name)
		}
		seen[e.Name] = true

		if len(e.Fields) == 0 {
			return fmt.Errorf("entity %q has no fields", e.Name)
		}
		for _, f := range e.Fields {
			if f.Name == "" {
				return fmt.Errorf("entity %q has a field with no name", e.Name)
			}
			if _, ok := KnownTypes[f.Type]; !ok {
				return fmt.Errorf("entity %q field %q has unknown type %q", e.Name, f.Name, f.Type)
			}
			if f.Type == "references" && f.Target == "" {
				return fmt.Errorf("entity %q field %q is a reference but has no target", e.Name, f.Name)
			}
		}
	}
	return nil
}
