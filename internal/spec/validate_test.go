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
		Module:  "example.com/blog",
		Entities: []Entity{
			{Name: "Author", Fields: []Field{
				{Name: "id", Type: TypeUUID, Primary: true},
			}},
			{Name: "Post", Fields: []Field{
				{Name: "id", Type: TypeUUID, Primary: true},
				{Name: "author", Type: TypeReferences, Target: "Author"},
			}},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	t.Parallel()

	require.NoError(t, validSpec().Validate())
}

func TestValidate_Defaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field Field
	}{
		{"string", Field{Name: "s", Type: TypeString, Default: "x"}},
		{"text", Field{Name: "s", Type: TypeText, Default: "x"}},
		{"int32", Field{Name: "n", Type: TypeInt32, Default: 1}},
		{"int64", Field{Name: "n", Type: TypeInt64, Default: 1}},
		{"float from int", Field{Name: "f", Type: TypeFloat, Default: 1}},
		{"float", Field{Name: "f", Type: TypeFloat, Default: 1.5}},
		{"bool", Field{Name: "b", Type: TypeBool, Default: false}},
		{"datetime now", Field{Name: "at", Type: TypeDatetime, Default: DefaultNow}},
		{"string now", Field{Name: "s", Type: TypeString, Default: DefaultNow}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := validSpec()
			s.Entities[1].Fields = append(s.Entities[1].Fields, tc.field)
			require.NoError(t, s.Validate())
		})
	}
}

func TestValidate_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Spec)
	}{
		{"missing package", func(s *Spec) { s.Package = "" }},
		{"missing module", func(s *Spec) { s.Module = "" }},
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
				{Name: "a", Type: TypeUUID, Primary: true},
				{Name: "b", Type: TypeUUID, Primary: true},
			}
		}},
		{"decimal primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "decimal" }},
		{"bool primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "bool" }},
		{"datetime primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "datetime" }},
		{"json primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = "json" }},
		{"primary key with default", func(s *Spec) { s.Entities[0].Fields[0].Default = "x" }},
		{"string default on int", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "n", Type: TypeInt32, Default: "1"})
		}},
		{"int default on bool", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "b", Type: TypeBool, Default: 1})
		}},
		{"float default on int", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "n", Type: TypeInt64, Default: 1.5})
		}},
		{"literal default on datetime", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "at", Type: TypeDatetime, Default: "2020-01-01"})
		}},
		{"now default on date", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "on", Type: TypeDate, Default: DefaultNow})
		}},
		{"default on reference", func(s *Spec) { s.Entities[1].Fields[1].Default = "x" }},
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
