package spec

import (
	"errors"
	"fmt"
	"strings"
)

// KnownTypes is the set of field types the generator understands.
var KnownTypes = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
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
var PrimaryKeyTypes = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
	TypeString:     {},
	TypeText:       {},
	TypeInt32:      {},
	TypeInt64:      {},
	TypeUUID:       {},
	TypeReferences: {},
}

var unfilterableTypes = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
	TypeFloat: {},
	TypeJSON:  {},
}

var unsortableTypes = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
	TypeBool: {},
	TypeJSON: {},
}

const (
	queryLimit  = "limit"
	queryOffset = "offset"
	querySort   = "sort"
)

var reservedQueryNames = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
	queryLimit:  {},
	queryOffset: {},
	querySort:   {},
}

// Validate checks the spec for structural errors.
func (s *Spec) Validate() error {
	if strings.TrimSpace(s.Package) == "" {
		return errors.New("missing package name")
	}
	if strings.TrimSpace(s.Module) == "" {
		return errors.New("missing module import path")
	}
	if len(s.Entities) == 0 {
		return errors.New("no entities defined")
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
			if f.OnDelete != "" && f.Type != TypeReferences {
				return fmt.Errorf("entity %q field %q has on_delete but is not a reference", e.Name, f.Name)
			}
			if f.OnDelete != "" && f.OnDelete != OnDeleteCascade {
				return fmt.Errorf("entity %q field %q has unknown on_delete %q: use %q", e.Name, f.Name, f.OnDelete, OnDeleteCascade)
			}
			if err := validateListModifiers(e, f); err != nil {
				return err
			}
			if f.Default != nil && f.Primary {
				return fmt.Errorf("entity %q primary key %q cannot have a default", e.Name, f.Name)
			}
			if f.Default != nil && !defaultFits(f.Type, f.Default) {
				return fmt.Errorf("entity %q field %q has default %v that does not fit type %q", e.Name, f.Name, f.Default, f.Type)
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

func validateListModifiers(e *Entity, f Field) error {
	if f.Filter {
		if f.Primary {
			return fmt.Errorf("entity %q primary key %q cannot be a filter: use GET /{id}", e.Name, f.Name)
		}
		if _, ok := unfilterableTypes[f.Type]; ok {
			return fmt.Errorf("entity %q field %q of type %q cannot be a filter", e.Name, f.Name, f.Type)
		}
		if _, ok := reservedQueryNames[f.Name]; ok {
			return fmt.Errorf("entity %q field %q cannot be a filter: the name is a reserved query parameter", e.Name, f.Name)
		}
	}
	if _, ok := unsortableTypes[f.Type]; ok && f.Sort {
		return fmt.Errorf("entity %q field %q of type %q cannot be sorted", e.Name, f.Name, f.Type)
	}
	return nil
}

func defaultFits(fieldType string, v any) bool {
	switch v.(type) {
	case string:
		return fieldType == TypeString || fieldType == TypeText || (fieldType == TypeDatetime && v == DefaultNow)
	case int:
		return fieldType == TypeInt32 || fieldType == TypeInt64 || fieldType == TypeFloat
	case float64:
		return fieldType == TypeFloat
	case bool:
		return fieldType == TypeBool
	}
	return false
}
