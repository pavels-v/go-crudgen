// Package spec defines the entity specification model and its loader.
package spec

// Spec is the top-level entity specification (see examples/blog.yaml for an example).
type Spec struct {
	Version  int      `yaml:"version"`
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
	SoftDelete bool `yaml:"soft_delete"`
}

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
}

// PrimaryKey returns the fields that make up the entity's primary key, in
// declaration order. A spec is invalid (rejected by Validate) if this is empty.
func (e *Entity) PrimaryKey() []Field {
	var pk []Field
	for _, f := range e.Fields {
		if f.Primary {
			pk = append(pk, f)
		}
	}
	return pk
}
