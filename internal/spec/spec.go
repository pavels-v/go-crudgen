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
	Name    string        `yaml:"name"`
	Plural  string        `yaml:"plural"` // optional; defaults to a naive pluralization
	Fields  []Field       `yaml:"fields"`
	Options EntityOptions `yaml:"options"`
}

// EntityOptions toggles per-entity generation behavior.
type EntityOptions struct {
	Timestamps bool `yaml:"timestamps"` // adds created_at / updated_at
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

// Field is a single attribute of an entity.
type Field struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Primary  bool   `yaml:"primary"` // part of the entity's primary key
	Required bool   `yaml:"required"`
	Unique   bool   `yaml:"unique"`
	Index    bool   `yaml:"index"`
	Default  any    `yaml:"default"`
	Validate string `yaml:"validate"` // go-playground/validator rule string
	Target   string `yaml:"target"`   // referenced entity, when Type == "references"
	OnDelete string `yaml:"on_delete"`
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
