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
			{Name: "Post", Pagination: PaginationCursor, Order: "created_at", Fields: []Field{
				{Name: "id", Type: TypeUUID, Primary: true},
				{Name: "author", Type: TypeReferences, Target: "Author", OnDelete: OnDeleteCascade, Filter: true},
				{Name: "created_at", Type: TypeDatetime, Generate: GenerateOnCreate},
				{Name: "updated_at", Type: TypeDatetime, Generate: GenerateOnWrite},
			}},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	t.Parallel()

	require.NoError(t, validSpec().Validate())
}

func TestValidate_OrderByNotNullFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field Field
	}{
		{"primary", Field{Name: "code", Type: TypeString, Primary: true}},
		{"required", Field{Name: "title", Type: TypeString, Required: true}},
		{"default", Field{Name: "views", Type: TypeInt64, Default: 0}},
		{"generated", Field{Name: "created_at", Type: TypeDatetime, Generate: GenerateOnCreate}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := validSpec()

			s.Entities[0].Order = tc.field.Name
			if tc.field.Primary {
				s.Entities[0].Fields = []Field{tc.field}
			} else {
				s.Entities[0].Fields = append(s.Entities[0].Fields, tc.field)
			}

			require.NoError(t, s.Validate())
		})
	}
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
		{"on_delete on non-reference", func(s *Spec) { s.Entities[1].Fields[0].OnDelete = OnDeleteCascade }},
		{"unknown on_delete", func(s *Spec) { s.Entities[1].Fields[1].OnDelete = "set_null" }},
		{"entity without primary key", func(s *Spec) { s.Entities[0].Fields[0].Primary = false }},
		{"composite primary key", func(s *Spec) {
			s.Entities[0].Fields = []Field{
				{Name: "a", Type: TypeUUID, Primary: true},
				{Name: "b", Type: TypeUUID, Primary: true},
			}
		}},
		{"decimal primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = TypeDecimal }},
		{"bool primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = TypeBool }},
		{"datetime primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = TypeDatetime }},
		{"json primary key", func(s *Spec) { s.Entities[0].Fields[0].Type = TypeJSON }},
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
		{"filter on primary key", func(s *Spec) { s.Entities[0].Fields[0].Filter = true }},
		{"filter on json", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "meta", Type: TypeJSON, Filter: true})
		}},
		{"filter on float", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "score", Type: TypeFloat, Filter: true})
		}},
		{"filter named like a paging parameter", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: queryLimit, Type: TypeInt32, Filter: true})
		}},
		{"filter named like the direction parameter", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: queryDir, Type: TypeString, Filter: true})
		}},
		{"order by bool", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "done", Type: TypeBool, Default: false})
			s.Entities[1].Order = "done"
		}},
		{"unknown generate", func(s *Spec) { s.Entities[1].Fields[2].Generate = "on_update" }},
		{"generate on non-datetime", func(s *Spec) { s.Entities[1].Fields[2].Type = TypeDate }},
		{"generate with default", func(s *Spec) { s.Entities[1].Fields[2].Default = DefaultNow }},
		{"generate with required", func(s *Spec) { s.Entities[1].Fields[3].Required = true }},
		{"unknown pagination", func(s *Spec) { s.Entities[1].Pagination = "keyset" }},
		{"order by nullable field", func(s *Spec) { s.Entities[1].Order = "author" }},
		{"order by unknown field", func(s *Spec) { s.Entities[1].Order = "ghost" }},
		{"filter named like the cursor parameter", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: queryCursor, Type: TypeString, Filter: true})
		}},
		{"order by json", func(s *Spec) {
			s.Entities[1].Fields = append(s.Entities[1].Fields, Field{Name: "meta", Type: TypeJSON, Required: true})
			s.Entities[1].Order = "meta"
		}},
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
