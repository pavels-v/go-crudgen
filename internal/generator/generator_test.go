package generator

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"go-crudgen/internal/spec"
)

// render runs renderModel for the named entity and returns the generated
// source, failing the test on error or if the output is not valid Go.
func render(t *testing.T, s *spec.Spec, entity string) string {
	t.Helper()

	byName := make(map[string]*spec.Entity, len(s.Entities))
	for i := range s.Entities {
		byName[s.Entities[i].Name] = &s.Entities[i]
	}
	var e *spec.Entity
	for i := range s.Entities {
		if s.Entities[i].Name == entity {
			e = &s.Entities[i]
		}
	}
	if e == nil {
		t.Fatalf("entity %q not found in spec", entity)
	}

	src, err := renderModel(s, e, byName)
	if err != nil {
		t.Fatalf("renderModel(%s): %v", entity, err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "", src, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse: %v\n%s", err, src)
	}
	return string(src)
}

// wantContains asserts the generated code contains want, comparing with runs of
// whitespace collapsed so gofmt's tab alignment does not matter.
func wantContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(strings.Join(strings.Fields(got), " "), want) {
		t.Errorf("generated code missing %q\n--- got ---\n%s", want, got)
	}
}

func TestRenderModel_ScalarTypesAndTags(t *testing.T) {
	s := &spec.Spec{
		Package: "shop",
		Entities: []spec.Entity{{
			Name: "Product",
			Fields: []spec.Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "name", Type: "string", Required: true, Validate: "min=1"},
				{Name: "price", Type: "decimal"},
				{Name: "in_stock", Type: "bool"},
				{Name: "released_at", Type: "datetime"},
				{Name: "metadata", Type: "json"},
			},
		}},
	}

	got := render(t, s, "Product")
	for _, want := range []string{
		"package shop",
		`"github.com/google/uuid"`,
		`"github.com/shopspring/decimal"`,
		`"encoding/json"`,
		`"time"`,
		"// Product represents a product.",
		"ID uuid.UUID",
		`json:"id"`,
		"Name string",
		`json:"name" validate:"required,min=1"`, // required merged ahead of validate
		"Price decimal.Decimal",
		"InStock bool", // snake_case -> PascalCase
		`json:"in_stock"`,
		"ReleasedAt time.Time",
		"Metadata json.RawMessage",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderModel_ReferenceDerivesTargetPKType(t *testing.T) {
	s := &spec.Spec{
		Package: "blog",
		Entities: []spec.Entity{
			{Name: "Author", Fields: []spec.Field{
				{Name: "id", Type: "int64", Primary: true},
			}},
			{Name: "Post", Fields: []spec.Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "author", Type: "references", Target: "Author"},
			}},
		},
	}

	got := render(t, s, "Post")
	// The foreign key takes the Go type of Author's primary key (int64), not uuid.
	wantContains(t, got, "Author int64")
	wantContains(t, got, `json:"author"`)
}

func TestRenderModel_OptionsTimestampsAndSoftDelete(t *testing.T) {
	s := &spec.Spec{
		Package: "app",
		Entities: []spec.Entity{{
			Name:    "Session",
			Fields:  []spec.Field{{Name: "id", Type: "uuid", Primary: true}},
			Options: spec.EntityOptions{Timestamps: true, SoftDelete: true},
		}},
	}

	got := render(t, s, "Session")
	wantContains(t, got, "CreatedAt time.Time")
	wantContains(t, got, `json:"created_at"`)
	wantContains(t, got, "UpdatedAt time.Time")
	wantContains(t, got, "DeletedAt *time.Time")
	wantContains(t, got, `json:"deleted_at,omitempty"`)
}

func TestRenderModel_CompositePrimaryKey(t *testing.T) {
	s := &spec.Spec{
		Package: "rel",
		Entities: []spec.Entity{{
			Name: "Membership",
			Fields: []spec.Field{
				{Name: "user_id", Type: "uuid", Primary: true},
				{Name: "group_id", Type: "uuid", Primary: true},
				{Name: "role", Type: "string"},
			},
		}},
	}

	got := render(t, s, "Membership")
	wantContains(t, got, "UserID uuid.UUID")
	wantContains(t, got, "GroupID uuid.UUID")
	wantContains(t, got, "Role string")
}

func TestPascalCase(t *testing.T) {
	cases := map[string]string{
		"title":      "Title",
		"created_at": "CreatedAt",
		"id":         "ID",
		"user_id":    "UserID",
		"api_url":    "APIURL",
	}
	for in, want := range cases {
		if got := pascalCase(in); got != want {
			t.Errorf("pascalCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"Post":     "post",
		"Author":   "author",
		"BlogPost": "blog_post",
	}
	for in, want := range cases {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}
