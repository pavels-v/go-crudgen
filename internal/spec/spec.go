// Package spec defines the entity specification model and its loader.
package spec

// Spec is the top-level entity specification (see examples/blog.yaml for an example).
type Spec struct {
	Package  string   `yaml:"package"`
	Module   string   `yaml:"module"`
	Entities []Entity `yaml:"entities"`
}

// Entity describes a single resource to generate CRUD endpoints for.
type Entity struct {
	Name       string  `yaml:"name"`
	Plural     string  `yaml:"plural"`     // optional; defaults to a naive pluralization
	Pagination string  `yaml:"pagination"` // offset (default) or cursor
	Order      string  `yaml:"order"`      // field List orders by; defaults to the primary key
	Fields     []Field `yaml:"fields"`
}

const (
	TypeString     = "string"
	TypeText       = "text"
	TypeInt32      = "int32"
	TypeInt64      = "int64"
	TypeFloat      = "float"
	TypeDecimal    = "decimal"
	TypeBool       = "bool"
	TypeDate       = "date"
	TypeDatetime   = "datetime"
	TypeUUID       = "uuid"
	TypeJSON       = "json"
	TypeReferences = "references"
)

const DefaultNow = "now"

const OnDeleteCascade = "cascade"

const (
	PaginationOffset = "offset"
	PaginationCursor = "cursor"
)

const (
	GenerateOnCreate = "on_create"
	GenerateOnWrite  = "on_write"
)

// Field is a single attribute of an entity.
type Field struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Primary  bool   `yaml:"primary"` // part of the entity's primary key
	Required bool   `yaml:"required"`
	Unique   bool   `yaml:"unique"`
	Index    bool   `yaml:"index"`
	Filter   bool   `yaml:"filter"` // List accepts ?<name>= for equality
	Default  any    `yaml:"default"`
	Validate string `yaml:"validate"` // go-playground/validator rule string
	Target   string `yaml:"target"`   // referenced entity, when Type == "references"
	OnDelete string `yaml:"on_delete"`
	Generate string `yaml:"generate"` // the server sets now() on create, or on every write
}

func (e *Entity) CursorPagination() bool {
	return e.Pagination == PaginationCursor
}

func (e *Entity) OrderField() (Field, bool) {
	if e.Order == "" {
		return e.PrimaryKey()[0], true
	}

	for _, f := range e.Fields {
		if f.Name == e.Order {
			return f, true
		}
	}
	return Field{}, false
}

// PrimaryKey returns the fields marked primary, in declaration order. A valid
// spec (per Validate) has exactly one; the slice lets Validate detect and reject
// missing or composite keys before any code relies on the single-key invariant.
func (e *Entity) PrimaryKey() []Field {
	var pk []Field
	for _, f := range e.Fields {
		if f.Primary {
			pk = append(pk, f)
		}
	}
	return pk
}
