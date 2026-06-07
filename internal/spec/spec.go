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
	Required bool   `yaml:"required"`
	Unique   bool   `yaml:"unique"`
	Index    bool   `yaml:"index"`
	Default  any    `yaml:"default"`
	Validate string `yaml:"validate"` // go-playground/validator rule string
	Target   string `yaml:"target"`   // referenced entity, when Type == "references"
}
