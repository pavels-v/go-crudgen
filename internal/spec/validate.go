package spec

import (
	"fmt"
	"strings"
)

// KnownTypes is the set of field types the generator understands.
var KnownTypes = map[string]struct{}{
	TypeString:     {},
	TypeText:       {},
	TypeInt32:      {},
	TypeInt64:      {},
	TypeFloat:      {},
	TypeDecimal:    {},
	TypeBool:       {},
	TypeDate:       {},
	TypeDatetime:   {},
	TypeUUID:       {},
	TypeJSON:       {},
	TypeReferences: {},
}

// PrimaryKeyTypes is the set of field types allowed for a primary key: those the
// generator can parse from a URL path segment to address a single row via /{id}.
// The remaining numeric and structural types (decimal, float, bool, date,
// datetime, json) have no path parser and are rejected. (references is allowed:
// it resolves to the target's primary key, which this same rule guarantees is
// path-addressable.)
var PrimaryKeyTypes = map[string]struct{}{
	TypeString:     {},
	TypeText:       {},
	TypeInt32:      {},
	TypeInt64:      {},
	TypeUUID:       {},
	TypeReferences: {},
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
			if f.Type == TypeReferences && f.Target == "" {
				return fmt.Errorf("entity %q field %q is a reference but has no target", e.Name, f.Name)
			}
		}
		switch pk := e.PrimaryKey(); len(pk) {
		case 1:
			if _, ok := PrimaryKeyTypes[pk[0].Type]; !ok {
				return fmt.Errorf("entity %q primary key %q has type %q, which cannot address a row via /{id}: use one of string, text, int32, int64, or uuid", e.Name, pk[0].Name, pk[0].Type)
			}
		case 0:
			return fmt.Errorf("entity %q has no primary key: mark exactly one field with primary: true", e.Name)
		default:
			return fmt.Errorf("entity %q has a composite primary key, which is not supported: mark exactly one field with primary: true", e.Name)
		}
	}

	// Reference targets are validated in a second pass so they may point at any
	// entity regardless of declaration order. The first pass guarantees every
	// entity has exactly one primary key, so a target's key is always addressable.
	byName := make(map[string]*Entity, len(s.Entities))
	for i := range s.Entities {
		byName[s.Entities[i].Name] = &s.Entities[i]
	}
	for i := range s.Entities {
		e := &s.Entities[i]
		for _, f := range e.Fields {
			if f.Type != TypeReferences {
				continue
			}
			if _, ok := byName[f.Target]; !ok {
				return fmt.Errorf("entity %q field %q references unknown entity %q", e.Name, f.Name, f.Target)
			}
		}
	}
	return nil
}
