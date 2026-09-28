package spec

import (
	"errors"
	"fmt"
	"go/token"
	"math"
	"strings"
)

// knownTypes is the set of field types the generator understands.
var knownTypes = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
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

// primaryKeyTypes is the set of field types allowed for a primary key: those the
// generator can parse from a URL path segment to address a single row via /{id}.
// The remaining numeric and structural types (decimal, float, bool, date,
// datetime, json) have no path parser and are rejected. (references is allowed:
// it resolves to the target's primary key, which this same rule guarantees is
// path-addressable.)
var primaryKeyTypes = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
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
	pkgMain    = "main"
	blankIdent = "_"
)

const pluralChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"

const (
	queryLimit  = "limit"
	queryOffset = "offset"
	queryDir    = "dir"
	queryCursor = "cursor"
)

var reservedQueryNames = map[string]struct{}{ //nolint:gochecknoglobals // read-only lookup table
	queryLimit:  {},
	queryOffset: {},
	queryDir:    {},
	queryCursor: {},
}

// Validate checks the spec for structural errors.
func (s *Spec) Validate() error {
	if strings.TrimSpace(s.Package) == "" {
		return errors.New("missing package name")
	}

	if !token.IsIdentifier(s.Package) || s.Package == pkgMain || s.Package == blankIdent {
		return fmt.Errorf("package %q is not an importable Go package name", s.Package)
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

		if e.Plural != "" && strings.Trim(e.Plural, pluralChars) != "" {
			return fmt.Errorf("entity %q has plural %q: use letters, digits, - and _", e.Name, e.Plural)
		}

		if e.Pagination != "" && e.Pagination != PaginationOffset && e.Pagination != PaginationCursor {
			return fmt.Errorf("entity %q has unknown pagination %q: use %q or %q", e.Name, e.Pagination, PaginationOffset, PaginationCursor)
		}

		for _, f := range e.Fields {
			if f.Name == "" {
				return fmt.Errorf("entity %q has a field with no name", e.Name)
			}

			if _, ok := knownTypes[f.Type]; !ok {
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

			if err := validateGenerate(e, f); err != nil {
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
			if _, ok := primaryKeyTypes[pk[0].Type]; !ok {
				return fmt.Errorf("entity %q primary key %q has type %q, which cannot address a row via /{id}: use one of string, text, int32, int64, or uuid", e.Name, pk[0].Name, pk[0].Type)
			}
		case 0:
			return fmt.Errorf("entity %q has no primary key: mark exactly one field with primary: true", e.Name)
		default:
			return fmt.Errorf("entity %q has a composite primary key, which is not supported: mark exactly one field with primary: true", e.Name)
		}

		if err := validateOrder(e); err != nil {
			return err
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

			target, ok := byName[f.Target]
			if !ok {
				return fmt.Errorf("entity %q field %q references unknown entity %q", e.Name, f.Name, f.Target)
			}

			if f.Target != e.Name && target.PrimaryKey()[0].Type == TypeReferences {
				return fmt.Errorf("entity %q field %q references %q, whose primary key is itself a reference", e.Name, f.Name, f.Target)
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

	return nil
}

func validateOrder(e *Entity) error {
	f, ok := e.OrderField()
	switch {
	case !ok:
		return fmt.Errorf("entity %q orders by unknown field %q", e.Name, e.Order)
	case isUnsortable(f.Type):
		return fmt.Errorf("entity %q cannot order by field %q of type %q", e.Name, f.Name, f.Type)
	case !f.Primary && !f.Required && f.Default == nil && f.Generate == "":
		return fmt.Errorf("entity %q cannot order by field %q, which can be NULL: make it required or give it a default", e.Name, f.Name)
	}

	return nil
}

func isUnsortable(fieldType string) bool {
	_, ok := unsortableTypes[fieldType]
	return ok
}

func validateGenerate(e *Entity, f Field) error {
	switch {
	case f.Generate == "":
		return nil
	case f.Generate != GenerateOnCreate && f.Generate != GenerateOnWrite:
		return fmt.Errorf("entity %q field %q has unknown generate %q: use %q or %q", e.Name, f.Name, f.Generate, GenerateOnCreate, GenerateOnWrite)
	case f.Type != TypeDatetime:
		return fmt.Errorf("entity %q field %q has generate but is not a datetime", e.Name, f.Name)
	case f.Primary, f.Required, f.Default != nil, f.Validate != "":
		return fmt.Errorf("entity %q field %q has generate, which excludes primary, required, default and validate", e.Name, f.Name)
	}

	return nil
}

func defaultFits(fieldType string, v any) bool {
	switch d := v.(type) {
	case string:
		return fieldType == TypeString || fieldType == TypeText || (fieldType == TypeDatetime && v == DefaultNow)
	case int:
		if fieldType == TypeInt32 {
			return d >= math.MinInt32 && d <= math.MaxInt32
		}

		return fieldType == TypeInt64 || fieldType == TypeFloat
	case float64:
		return fieldType == TypeFloat && !math.IsInf(d, 0) && !math.IsNaN(d)
	case bool:
		return fieldType == TypeBool
	}

	return false
}
