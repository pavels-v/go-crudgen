package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// validSpec returns a minimal spec that passes Validate; tests mutate a copy to
// exercise individual failure cases.
func validSpec() *Spec {
	return &Spec{
		Package: "blog",
		Entities: []Entity{
			{Name: "Author", Fields: []Field{
				{Name: "id", Type: "uuid", Primary: true},
			}},
			{Name: "Post", Fields: []Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "author", Type: "references", Target: "Author"},
			}},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	t.Parallel()

	require.NoError(t, validSpec().Validate())
}

func TestValidate_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Spec)
	}{
		{"missing package", func(s *Spec) { s.Package = "" }},
		{"no entities", func(s *Spec) { s.Entities = nil }},
		{"entity without name", func(s *Spec) { s.Entities[0].Name = "" }},
		{"duplicate entity", func(s *Spec) { s.Entities[1].Name = "Author" }},
		{"entity without fields", func(s *Spec) { s.Entities[0].Fields = nil }},
		{"field without name", func(s *Spec) { s.Entities[0].Fields[0].Name = "" }},
		{"unknown field type", func(s *Spec) { s.Entities[0].Fields[0].Type = "bogus" }},
		{"reference without target", func(s *Spec) { s.Entities[1].Fields[1].Target = "" }},
		{"reference to unknown entity", func(s *Spec) { s.Entities[1].Fields[1].Target = "Ghost" }},
		{"entity without primary key", func(s *Spec) { s.Entities[0].Fields[0].Primary = false }},
		{"composite primary key", func(s *Spec) {
			s.Entities[0].Fields = []Field{
				{Name: "a", Type: "uuid", Primary: true},
				{Name: "b", Type: "uuid", Primary: true},
			}
		}},
		{"decimal primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "decimal" }},
		{"bool primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "bool" }},
		{"datetime primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "datetime" }},
		{"json primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "json" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := validSpec()
			tc.mutate(s)
			require.Errorf(t, s.Validate(), "expected error for %q", tc.name)
		})
	}
}
