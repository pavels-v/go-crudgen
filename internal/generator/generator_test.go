package generator

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.NotNilf(t, e, "entity %q not found in spec", entity)

	src, err := renderModel(s, e, byName)
	require.NoErrorf(t, err, "renderModel(%s)", entity)
	requireParses(t, src)
	return string(src)
}

// requireParses fails the test when src is not valid Go source.
func requireParses(t *testing.T, src []byte) {
	t.Helper()

	_, err := parser.ParseFile(token.NewFileSet(), "", src, parser.AllErrors)
	require.NoErrorf(t, err, "generated code does not parse:\n%s", src)
}

// wantContains asserts the generated code contains want, comparing with runs of
// whitespace collapsed so gofmt's tab alignment does not matter.
func wantContains(t *testing.T, got, want string) {
	t.Helper()
	require.Containsf(t, strings.Join(strings.Fields(got), " "), want,
		"generated code missing %q", want)
}

func TestRenderModel_ScalarTypesAndTags(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "shop",
		Module:  "example.com/shop",
		Entities: []spec.Entity{{
			Name: "Product",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "name", Type: spec.TypeString, Required: true, Validate: "min=1"},
				{Name: "price", Type: spec.TypeDecimal},
				{Name: "in_stock", Type: spec.TypeBool},
				{Name: "released_at", Type: spec.TypeDatetime},
				{Name: "launched_on", Type: spec.TypeDate},
				{Name: "metadata", Type: spec.TypeJSON},
			},
		}},
	}

	got := render(t, s, "Product")
	for _, want := range []string{
		"package shop",
		`"github.com/google/uuid"`,
		`"github.com/shopspring/decimal"`,
		`"encoding/json/jsontext"`,
		`"time"`,
		"// Product is the API model of the product entity.",
		"ID uuid.UUID", // primary key -> non-null value type
		`json:"id"`,
		"Name string",            // required -> non-null value type
		`json:"name"`,            // the model carries no validate tag
		"Price *decimal.Decimal", // nullable -> pointer
		"InStock *bool",          // snake_case -> PascalCase, nullable -> pointer
		`json:"in_stock,omitzero"`,
		"ReleasedAt *time.Time",
		"LaunchedOn *Date",
		"Metadata *jsontext.Value",
		"type ProductRepository interface {",
		"Get(ctx context.Context, id uuid.UUID) (*Product, error)",
	} {
		wantContains(t, got, want)
	}
	require.NotContains(t, got, `db:"`, "API model should not carry db struct tags")
	require.NotContains(t, got, `validate:"`, "API model should not carry validate tags")
}

func TestRenderModel_ReferenceDerivesTargetPKType(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{
			{Name: "Author", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeInt64, Primary: true},
			}},
			{Name: "Post", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "author", Type: spec.TypeReferences, Target: "Author"},
			}},
		},
	}

	got := render(t, s, "Post")
	// The foreign key takes the Go type of Author's primary key (int64), not uuid;
	// it is not required, so it is nullable and rendered as a pointer.
	wantContains(t, got, "Author *int64")
	wantContains(t, got, `json:"author,omitzero"`)
}

func TestRenderModel_OptionsTimestamps(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "app",
		Module:  "example.com/app",
		Entities: []spec.Entity{{
			Name:    "Session",
			Fields:  []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}},
			Options: spec.EntityOptions{Timestamps: true},
		}},
	}

	got := render(t, s, "Session")
	wantContains(t, got, "CreatedAt time.Time")
	wantContains(t, got, `json:"created_at"`)
	wantContains(t, got, "UpdatedAt time.Time")
}

// renderHandlerSrc runs handlerInfo + renderHandler for the named entity and
// returns the generated source, failing if generation errors or the output is
// not valid Go.
func renderHandlerSrc(t *testing.T, s *spec.Spec, entity string) string {
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
	require.NotNilf(t, e, "entity %q not found in spec", entity)

	hd, err := handlerInfo(s, e, byName)
	require.NoErrorf(t, err, "handlerInfo(%s)", entity)

	src, err := renderHandler(hd)
	require.NoErrorf(t, err, "renderHandler(%s)", entity)
	requireParses(t, src)
	return string(src)
}

func TestRenderHandler_DTOs(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{{
			Name: "Post",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true},
			},
			Options: spec.EntityOptions{Timestamps: true},
		}},
	}

	got := renderHandlerSrc(t, s, "Post")
	for _, want := range []string{
		"type CreatePostRequest struct {",
		"type UpdatePostRequest struct {",
		"Title string",
		`json:"title" validate:"required"`,
	} {
		wantContains(t, got, want)
	}
	// The update body omits the primary key (it comes from the path)...
	require.NotContains(t, strings.Join(strings.Fields(got), " "),
		"type UpdatePostRequest struct { ID uuid.UUID",
		"UpdatePostRequest should not contain the primary key field")
	// ...and DTOs never carry the option-injected timestamp fields.
	_, create, ok := strings.Cut(got, "type CreatePostRequest")
	require.True(t, ok, "CreatePostRequest should be generated")
	require.NotContains(t, create, "CreatedAt", "DTOs should not contain timestamp fields")
	require.NotContains(t, create, "ID uuid.UUID", "a generated key is not part of CreatePostRequest")
}

func TestRenderModel_ClientKeyAndDefaults(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "cat",
		Module:  "example.com/cat",
		Entities: []spec.Entity{{
			Name: "Tag",
			Fields: []spec.Field{
				{Name: "slug", Type: spec.TypeString, Primary: true},
				{Name: "published", Type: spec.TypeBool, Default: false},
			},
		}},
	}

	wantContains(t, render(t, s, "Tag"), "type Tag struct { Slug string `json:\"slug\"` Published bool `json:\"published\"` }")

	handler := renderHandlerSrc(t, s, "Tag")
	for _, want := range []string{
		"type CreateTagRequest struct { Slug string `json:\"slug\" validate:\"required\"` Published *bool `json:\"published\"` }",
		"type UpdateTagRequest struct { Published *bool `json:\"published\"` }",
		"Slug: req.Slug,",
		"Published: valueOr(req.Published, false),",
	} {
		wantContains(t, handler, want)
	}

	repo := renderRepoSrc(t, s, "Tag")
	wantContains(t, repo, "r.db.ExecContext(ctx, `INSERT INTO tags (slug, published) VALUES ($1, $2)`, row.Slug, row.Published)")
}

func TestRenderHandler_GeneratedKeyNotAssigned(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{{
			Name: "Post",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true},
			},
		}},
	}

	got := renderHandlerSrc(t, s, "Post")
	require.NotContains(t, got, "req.ID")
	wantContains(t, got, "Title: req.Title,")
}

func TestRenderHandler_RoutesAndStatusCodes(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{{
			Name:   "Post",
			Plural: "posts",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true},
			},
		}},
	}

	got := renderHandlerSrc(t, s, "Post")
	for _, want := range []string{
		`mux.HandleFunc("POST /posts", h.Create)`,
		`mux.HandleFunc("GET /posts", h.List)`,
		`mux.HandleFunc("GET /posts/{id}", h.Get)`,
		`mux.HandleFunc("PUT /posts/{id}", h.Update)`,
		`mux.HandleFunc("DELETE /posts/{id}", h.Delete)`,
		`"example.com/blog"`,
		"repo blog.PostRepository",
		"m := blog.Post{",
		"id, err := uuid.Parse(r.PathValue(pathParamID))",
		"writeDecodeError(w, err)",
		"writeValidationError(w, r, err)",
		"writeInvalidID(w)",
		"writeBody(w, http.StatusCreated, m)",
		"q := newListQuery(r) p := blog.PostListParams{} p.Limit, p.Offset = q.page()",
		"items, err := h.repo.List(r.Context(), p)",
		"writeBody(w, http.StatusOK, page[blog.Post]{Items: items, Limit: p.Limit, Offset: p.Offset})",
		"writeError(w, http.StatusBadRequest, codeInvalidQuery,",
		"w.WriteHeader(http.StatusNoContent)",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderHandler_StringPKNeedsNoParse(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "cat",
		Module:  "example.com/cat",
		Entities: []spec.Entity{{
			Name:   "Tag",
			Fields: []spec.Field{{Name: "slug", Type: spec.TypeString, Primary: true}},
		}},
	}

	got := renderHandlerSrc(t, s, "Tag")
	wantContains(t, got, `id := r.PathValue(pathParamID)`)
}

func TestRenderHandler_Int32PKParsesAndCasts(t *testing.T) {
	t.Parallel()

	// int32 has no single-expression strconv parser: the handler parses with a
	// 32-bit size and casts the int64 result down to the int32 key type.
	s := &spec.Spec{
		Package: "shop",
		Module:  "example.com/shop",
		Entities: []spec.Entity{{
			Name:   "Widget",
			Fields: []spec.Field{{Name: "id", Type: spec.TypeInt32, Primary: true}},
		}},
	}

	got := renderHandlerSrc(t, s, "Widget")
	for _, want := range []string{
		`idRaw, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 32)`,
		"id := int32(idRaw)",
	} {
		wantContains(t, got, want)
	}
	wantContains(t, render(t, s, "Widget"), "Get(ctx context.Context, id int32) (*Widget, error)")
}

// renderRepoSrc runs repoInfo + renderRepo for the named entity and returns the
// generated source, failing if generation errors or the output is not valid Go.
func renderRepoSrc(t *testing.T, s *spec.Spec, entity string) string {
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
	require.NotNilf(t, e, "entity %q not found in spec", entity)

	rd, err := repoInfo(s, e, byName)
	require.NoErrorf(t, err, "repoInfo(%s)", entity)

	src, err := renderRepo(rd)
	require.NoErrorf(t, err, "renderRepo(%s)", entity)
	requireParses(t, src)
	return string(src)
}

func TestRenderRepo_SQLAndInterfaceSatisfaction(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{{
			Name:   "Post",
			Plural: "posts",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true},
				{Name: "body", Type: spec.TypeText},
			},
			Options: spec.EntityOptions{Timestamps: true},
		}},
	}

	got := renderRepoSrc(t, s, "Post")
	for _, want := range []string{
		// compile-time check that the concrete type implements the interface
		"package postgres",
		`"example.com/blog"`,
		"var _ blog.PostRepository = (*PostRepository)(nil)",
		"func NewPostRepository(db *sqlx.DB) *PostRepository",
		"func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*blog.Post, error)",
		"return nil, blog.ErrNotFound",
		// the primary-key type's package is imported for the Get/Delete signatures
		`"github.com/google/uuid"`,
		`"github.com/jmoiron/sqlx"`,
		// the row struct carries db tags; nullable body becomes sql.Null[T]
		"type postRow struct {",
		"Body sql.Null[string]",
		"func newPostRow(m *blog.Post) postRow",
		"Body: toNull(m.Body)",
		"Body: fromNull(row.Body)",
		// sqlx scans into the row, which is then converted to the API model
		"r.db.GetContext(ctx, &row, `SELECT id, title, body, created_at, updated_at FROM posts WHERE id = $1`, id)",
		"m := row.toModel()",
		"errors.Is(err, sql.ErrNoRows)",
		// timestamps default to now() on insert and are returned into the struct
		"INSERT INTO posts (id, title, body, created_at, updated_at) VALUES ($1, $2, $3, now(), now()) RETURNING created_at, updated_at",
		"m.ID = uuid.New() row := newPostRow(m)",
		"r.db.QueryRowContext(ctx, `INSERT INTO posts (id, title, body, created_at, updated_at) VALUES ($1, $2, $3, now(), now()) RETURNING created_at, updated_at`, row.ID, row.Title, row.Body).Scan(&m.CreatedAt, &m.UpdatedAt)",
		"SELECT id, title, body, created_at, updated_at FROM posts WHERE id = $1",
		"ORDER BY id LIMIT $1 OFFSET $2",
		// the primary key is the trailing placeholder in the update
		"UPDATE posts SET title = $1, body = $2, updated_at = now() WHERE id = $3 RETURNING created_at, updated_at",
		"DELETE FROM posts WHERE id = $1",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderRepo_WithoutTimestamps(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "app",
		Module:  "example.com/app",
		Entities: []spec.Entity{{
			Name: "Account",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "name", Type: spec.TypeString},
			},
		}},
	}

	got := renderRepoSrc(t, s, "Account")
	for _, want := range []string{
		"SELECT id, name FROM accounts WHERE id = $1",
		"r.db.SelectContext(ctx, &rows, `SELECT id, name FROM accounts ORDER BY id LIMIT $1 OFFSET $2`, p.Limit, p.Offset)",
		"DELETE FROM accounts WHERE id = $1",
		// no timestamps -> Exec + RowsAffected for the not-found check
		"res.RowsAffected()",
		"if n == 0 {",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderList_FiltersAndSort(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{
			{Name: "Author", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}},
			{Name: "Post", Plural: "posts", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true, Sort: true},
				{Name: "title", Type: spec.TypeString, Required: true, Filter: true, Sort: true},
				{Name: "author", Type: spec.TypeReferences, Target: "Author", Filter: true},
				{Name: "published_on", Type: spec.TypeDate, Filter: true},
				{Name: "views", Type: spec.TypeInt32, Filter: true},
				{Name: "published", Type: spec.TypeBool, Filter: true},
			}},
			{Name: "Tag", Fields: []spec.Field{
				{Name: "slug", Type: spec.TypeString, Primary: true},
				{Name: "label", Type: spec.TypeString, Sort: true},
			}},
		},
	}

	cases := []struct {
		name   string
		render func(t *testing.T, s *spec.Spec, entity string) string
		entity string
		want   []string
		absent []string
	}{
		{
			name:   "domain params and sort constants",
			render: render,
			entity: "Post",
			want: []string{
				"type PostSort string",
				`PostSortID PostSort = "id"`,
				`PostSortIDDesc PostSort = "-id"`,
				`PostSortTitle PostSort = "title"`,
				`PostSortTitleDesc PostSort = "-title"`,
				"type PostListParams struct { Title *string Author *uuid.UUID PublishedOn *Date Views *int32 Published *bool Sort PostSort Limit int Offset int }",
				"List(ctx context.Context, p PostListParams) ([]Post, error)",
			},
		},
		{
			name:   "handler parses filters and sort",
			render: renderHandlerSrc,
			entity: "Post",
			want: []string{
				`queryPostTitle = "title"`,
				`queryPostPublishedOn = "published_on"`,
				"q := newListQuery(r, queryPostTitle, queryPostAuthor, queryPostPublishedOn, queryPostViews, queryPostPublished, querySort)",
				"Title: queryValue(q, queryPostTitle, parseString),",
				"Author: queryValue(q, queryPostAuthor, parseText[uuid.UUID]),",
				"PublishedOn: queryValue(q, queryPostPublishedOn, parseText[blog.Date]),",
				"Views: queryValue(q, queryPostViews, parseInt32),",
				"Published: queryValue(q, queryPostPublished, strconv.ParseBool),",
				"Sort: querySortValue(q, blog.PostSortID, blog.PostSortIDDesc, blog.PostSortTitle, blog.PostSortTitleDesc),",
				`"strconv"`,
			},
		},
		{
			name:   "repository assembles where and order by",
			render: renderRepoSrc,
			entity: "Post",
			want: []string{
				"if p.Author != nil { where = append(where, `author = ?`) args = append(args, *p.Author) }",
				"if p.PublishedOn != nil { where = append(where, `published_on = ?`)",
				"q := `SELECT id, title, author, published_on, views, published FROM posts`",
				"q += ` WHERE ` + strings.Join(where, ` AND `)",
				"case blog.PostSortIDDesc: q += ` ORDER BY id DESC`",
				"case blog.PostSortTitleDesc: q += ` ORDER BY title DESC, id`",
				"default: q += ` ORDER BY id`",
				"q += ` LIMIT ? OFFSET ?` args = append(args, p.Limit, p.Offset)",
				"r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...)",
				`"strings"`,
			},
		},
		{
			name:   "sort without filters",
			render: renderRepoSrc,
			entity: "Tag",
			want: []string{
				"var args []any q := `SELECT slug, label FROM tags` switch p.Sort {",
				"case blog.TagSortLabel: q += ` ORDER BY label, slug`",
			},
			absent: []string{"where = append", `"strings"`},
		},
		{
			name:   "handler without filters accepts only sort",
			render: renderHandlerSrc,
			entity: "Tag",
			want: []string{
				"q := newListQuery(r, querySort)",
				"p := blog.TagListParams{ Sort: querySortValue(q, blog.TagSortLabel, blog.TagSortLabelDesc), }",
			},
			absent: []string{"queryValue("},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.render(t, s, tc.entity)
			for _, want := range tc.want {
				wantContains(t, got, want)
			}
			for _, absent := range tc.absent {
				require.NotContains(t, got, absent)
			}
		})
	}
}

func TestRenderRepo_PrimaryKeyOnlyEntityUsesExistenceCheck(t *testing.T) {
	t.Parallel()

	// A serveable entity with only a primary key and no timestamps has nothing
	// writable: the UPDATE must not be `SET  WHERE ...` (invalid SQL). It should
	// degrade to an existence check by primary key.
	s := &spec.Spec{
		Package: "cat",
		Module:  "example.com/cat",
		Entities: []spec.Entity{{
			Name:   "Tag",
			Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}},
		}},
	}

	got := renderRepoSrc(t, s, "Tag")
	for _, want := range []string{
		"r.db.GetContext(ctx, &exists, `SELECT 1 FROM tags WHERE id = $1`, row.ID)",
		"return cat.ErrNotFound",
	} {
		wantContains(t, got, want)
	}
	// must NOT emit a malformed empty SET clause
	require.NotContains(t, strings.Join(strings.Fields(got), " "), "SET WHERE",
		"primary-key-only entity must not generate an empty UPDATE SET clause")
}

func TestRepoInfo_HasNullable(t *testing.T) {
	t.Parallel()

	byNameOf := func(s *spec.Spec) map[string]*spec.Entity {
		m := make(map[string]*spec.Entity, len(s.Entities))
		for i := range s.Entities {
			m[s.Entities[i].Name] = &s.Entities[i]
		}
		return m
	}

	cases := []struct {
		name   string
		fields []spec.Field
		want   bool
	}{
		{
			name:   "a non-required column is nullable",
			fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}, {Name: "body", Type: spec.TypeText}},
			want:   true,
		},
		{
			name:   "all columns required or primary",
			fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}, {Name: "value", Type: spec.TypeString, Required: true}},
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "app", Module: "example.com/app", Entities: []spec.Entity{{Name: "Thing", Fields: tc.fields}}}
			rd, err := repoInfo(s, &s.Entities[0], byNameOf(s))
			require.NoError(t, err)
			require.Equal(t, tc.want, rd.HasNullable)
		})
	}
}

func TestRenderNulls_GenericHelpers(t *testing.T) {
	t.Parallel()

	src, err := renderNulls(packageData{Package: pkgPostgres})
	require.NoError(t, err)
	requireParses(t, src)

	got := string(src)
	for _, want := range []string{
		"package postgres",
		`import "database/sql"`,
		"func toNull[T any](p *T) sql.Null[T]",
		"func fromNull[T any](n sql.Null[T]) *T",
		"return sql.Null[T]{V: *p, Valid: true}",
	} {
		wantContains(t, got, want)
	}
}

// renderMigrationSrc runs migrationInfo + renderMigration for the named entity.
func renderMigrationSrc(t *testing.T, s *spec.Spec, entity string) string {
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
	require.NotNilf(t, e, "entity %q not found in spec", entity)

	md, err := migrationInfo(e, byName)
	require.NoErrorf(t, err, "migrationInfo(%s)", entity)
	src, err := renderMigration(md)
	require.NoErrorf(t, err, "renderMigration(%s)", entity)
	return string(src)
}

func TestRenderDate_TextAndSQLRoundTrip(t *testing.T) {
	t.Parallel()

	src, err := renderDate(packageData{Package: "blog"})
	require.NoError(t, err)
	requireParses(t, src)

	got := string(src)
	for _, want := range []string{
		"package blog",
		"type Date time.Time",
		"func (d Date) MarshalText() ([]byte, error)",
		"func (d *Date) UnmarshalText(b []byte) error",
		"func (d Date) Value() (driver.Value, error)",
		"func (d *Date) Scan(src any) error",
		"time.DateOnly",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderMigration_ColumnsConstraintsAndOptions(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{
			{Name: "Author", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
			}},
			{Name: "Post", Plural: "posts", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true},
				{Name: "body", Type: spec.TypeText},
				{Name: "published", Type: spec.TypeBool, Default: false},
				{Name: "slug", Type: spec.TypeString, Unique: true, Index: true},
				{Name: "author", Type: spec.TypeReferences, Target: "Author"},
				{Name: "editor", Type: spec.TypeReferences, Target: "Author", OnDelete: spec.OnDeleteCascade},
			}, Options: spec.EntityOptions{Timestamps: true}},
		},
	}

	got := renderMigrationSrc(t, s, "Post")
	for _, want := range []string{
		// goose annotations frame the up/down sections and wrap each statement
		"-- +goose Up",
		"-- +goose Down",
		"-- +goose StatementBegin",
		"-- +goose StatementEnd",
		"CREATE TABLE posts (",
		"id UUID NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY", // single PK declared inline
		"title TEXT NOT NULL", // required -> NOT NULL
		"body TEXT,",          // optional column is nullable
		"published BOOLEAN NOT NULL DEFAULT FALSE", // bool default rendered as SQL literal
		"slug TEXT UNIQUE",                         // unique modifier
		"author UUID REFERENCES authors (id),",     // FK column typed from + pointing at the target PK
		"editor UUID REFERENCES authors (id) ON DELETE CASCADE,",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"CREATE INDEX idx_posts_slug ON posts (slug);",
		"DROP TABLE posts;",
	} {
		wantContains(t, got, want)
	}
	// Each statement is wrapped in its own block: CREATE TABLE + CREATE INDEX (up)
	// and DROP TABLE (down) make three.
	require.Equal(t, 3, strings.Count(got, "-- +goose StatementBegin"))
	require.Equal(t, 3, strings.Count(got, "-- +goose StatementEnd"))
	require.True(t, strings.HasPrefix(got, "-- +goose Up"), "migration starts with the goose Up annotation")
}

func TestMigrationOrder(t *testing.T) {
	t.Parallel()

	byNameOf := func(s *spec.Spec) map[string]*spec.Entity {
		m := make(map[string]*spec.Entity, len(s.Entities))
		for i := range s.Entities {
			m[s.Entities[i].Name] = &s.Entities[i]
		}
		return m
	}

	t.Run("referenced entity is ordered first", func(t *testing.T) {
		t.Parallel()

		// Post references Author but is declared first; Author must still come first
		// so its table exists when the posts foreign key is created.
		s := &spec.Spec{Package: "blog", Entities: []spec.Entity{
			{Name: "Post", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "author", Type: spec.TypeReferences, Target: "Author"},
			}},
			{Name: "Author", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}},
		}}

		order, err := migrationOrder(s.Entities, byNameOf(s))
		require.NoError(t, err)
		names := []string{order[0].Name, order[1].Name}
		require.Equal(t, []string{"Author", "Post"}, names)
	})

	t.Run("self-reference is allowed", func(t *testing.T) {
		t.Parallel()

		s := &spec.Spec{Package: "tree", Entities: []spec.Entity{
			{Name: "Node", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "parent", Type: spec.TypeReferences, Target: "Node"},
			}},
		}}

		order, err := migrationOrder(s.Entities, byNameOf(s))
		require.NoError(t, err)
		require.Len(t, order, 1)
	})

	t.Run("reference cycle errors", func(t *testing.T) {
		t.Parallel()

		s := &spec.Spec{Package: "loop", Entities: []spec.Entity{
			{Name: "A", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "b", Type: spec.TypeReferences, Target: "B"},
			}},
			{Name: "B", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "a", Type: spec.TypeReferences, Target: "A"},
			}},
		}}

		_, err := migrationOrder(s.Entities, byNameOf(s))
		require.Error(t, err)
	})
}

func TestSQLType(t *testing.T) {
	t.Parallel()

	byName := map[string]*spec.Entity{
		"Author": {Name: "Author", Fields: []spec.Field{{Name: "id", Type: spec.TypeInt64, Primary: true}}},
	}
	cases := []struct {
		name  string
		field spec.Field
		want  string
	}{
		{name: "string", field: spec.Field{Type: spec.TypeString}, want: "TEXT"},
		{name: "int32", field: spec.Field{Type: spec.TypeInt32}, want: "INTEGER"},
		{name: "int64", field: spec.Field{Type: spec.TypeInt64}, want: "BIGINT"},
		{name: "float", field: spec.Field{Type: spec.TypeFloat}, want: "DOUBLE PRECISION"},
		{name: "decimal", field: spec.Field{Type: spec.TypeDecimal}, want: "NUMERIC"},
		{name: "bool", field: spec.Field{Type: spec.TypeBool}, want: "BOOLEAN"},
		{name: "date", field: spec.Field{Type: spec.TypeDate}, want: "DATE"},
		{name: "datetime", field: spec.Field{Type: spec.TypeDatetime}, want: "TIMESTAMPTZ"},
		{name: "uuid", field: spec.Field{Type: spec.TypeUUID}, want: "UUID"},
		{name: "json", field: spec.Field{Type: spec.TypeJSON}, want: "JSONB"},
		{name: "reference takes target PK type", field: spec.Field{Type: spec.TypeReferences, Target: "Author"}, want: "BIGINT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := sqlType(tc.field, byName)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSQLDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   any
		want string
	}{
		{name: "unset", in: nil, want: ""},
		{name: "true", in: true, want: "TRUE"},
		{name: "false", in: false, want: "FALSE"},
		{name: "int", in: 42, want: "42"},
		{name: "string is quoted", in: "draft", want: "'draft'"},
		{name: "string with quote is escaped", in: "o'brien", want: "'o''brien'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := sqlDefault(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRenderDB_DriverSelection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		driver     string
		wantName   string
		wantImport string
	}{
		{name: "default is pgx", driver: "", wantName: `"pgx"`, wantImport: `_ "github.com/jackc/pgx/v5/stdlib"`},
		{name: "explicit pgx", driver: DriverPgx, wantName: `"pgx"`, wantImport: `_ "github.com/jackc/pgx/v5/stdlib"`},
		{name: "pq maps to postgres", driver: DriverPq, wantName: `"postgres"`, wantImport: `_ "github.com/lib/pq"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, imp, ok := driverInfo(tc.driver)
			require.Truef(t, ok, "driverInfo(%q) ok", tc.driver)

			s := &spec.Spec{Package: "blog", Module: "example.com/blog"}
			src, err := renderDB(dbInfo(s, name, imp))
			require.NoError(t, err)
			requireParses(t, src)

			got := string(src)
			wantContains(t, got, "driverName = "+tc.wantName)
			wantContains(t, got, tc.wantImport)
			wantContains(t, got, "func NewDB(dsn string) (*sqlx.DB, error)")
			wantContains(t, got, "db, err := sqlx.Open(driverName, dsn)")
			wantContains(t, got, "db.SetMaxOpenConns(maxOpenConns)")
			wantContains(t, got, `sqlStateUniqueViolation = "23505"`)
			wantContains(t, got, "package postgres")
			wantContains(t, got, `"example.com/blog" )`)
			wantContains(t, got, "return fmt.Errorf(\"%w: %v\", blog.ErrAlreadyExists, err)")
			wantContains(t, got, "return fmt.Errorf(\"%w: %v\", blog.ErrStillReferenced, err)")
		})
	}
}

func TestDriverInfo_RejectsUnknown(t *testing.T) {
	t.Parallel()

	_, _, ok := driverInfo("mysql")
	require.False(t, ok, "unknown driver should be rejected")
}

func TestRenderRouter_WiresEntities(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{Package: "blog", Module: "example.com/blog"}
	src, err := renderRouter(routerInfo(s, []routerEntity{
		{Struct: "Post", Repo: "blog.PostRepository", DepsField: "Posts"},
	}))
	require.NoError(t, err)
	requireParses(t, src)

	got := string(src)
	for _, want := range []string{
		"package restapi",
		`"example.com/blog"`,
		"blog.ErrNotFound: {http.StatusNotFound, codeNotFound},",
		"blog.ErrAlreadyExists: {http.StatusConflict, codeAlreadyExists},",
		"blog.ErrReferenceNotFound: {http.StatusUnprocessableEntity, codeReferenceNotFound},",
		"blog.ErrStillReferenced: {http.StatusConflict, codeStillReferenced},",
		`codeValidationFailed = "validation_failed"`,
		"type bodyResponse[T any] struct { Body T `json:\"body\"` }",
		"type errorResponse struct { Error apiError `json:\"error\"` }",
		"type apiError struct { Code string `json:\"code\"` Message string `json:\"message\"` Details []errorDetail `json:\"details\"` }",
		"type page[T any] struct { Items []T `json:\"items\"` Limit int `json:\"limit\"` Offset int `json:\"offset\"` }",
		"writeError(w, http.StatusUnprocessableEntity, codeValidationFailed,",
		"writeError(w, http.StatusInternalServerError, codeInternal, http.StatusText(http.StatusInternalServerError), nil)",
		"func valueOr[T any](p *T, def T) T",
		"json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxBodyBytes), v, json.RejectUnknownMembers(true))",
		"func NewRouter(deps Deps) http.Handler {",
		"return withRouteErrors(mux)",
		"Posts blog.PostRepository",
		"RegisterPostRoutes(mux, NewPostHandler(deps.Posts))",
	} {
		wantContains(t, got, want)
	}
}

func TestPluralize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		override string
		want     string
	}{
		{name: "regular plural", in: "Post", want: "posts"},
		{name: "another regular", in: "Author", want: "authors"},
		{name: "consonant y to ies", in: "Category", want: "categories"},
		{name: "x to es", in: "Box", want: "boxes"},
		{name: "vowel y stays", in: "Day", want: "days"},
		{name: "explicit override", in: "Person", override: "people", want: "people"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, plural(tc.in, tc.override))
		})
	}
}

func TestPascalCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "single word", in: "title", want: "Title"},
		{name: "snake case", in: "created_at", want: "CreatedAt"},
		{name: "initialism id", in: "id", want: "ID"},
		{name: "trailing initialism", in: "user_id", want: "UserID"},
		{name: "two initialisms", in: "api_url", want: "APIURL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, pascalCase(tc.in))
		})
	}
}

func TestSnakeCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "single word", in: "Post", want: "post"},
		{name: "another word", in: "Author", want: "author"},
		{name: "camel case", in: "BlogPost", want: "blog_post"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, snakeCase(tc.in))
		})
	}
}

func TestGroupImports(t *testing.T) {
	t.Parallel()

	const testModule = "example.com/blog"

	cases := []struct {
		name   string
		module string
		in     []string
		want   []string
	}{
		{name: "empty", in: nil, want: nil},
		{name: "stdlib only", in: []string{importTime, importContext}, want: []string{importContext, importTime}},
		{name: "third-party only", in: []string{importUUID, importDecimal}, want: []string{importUUID, importDecimal}},
		{
			name: "mixed split by blank entry",
			in:   []string{importSQLx, importTime, importUUID, importContext},
			want: []string{importContext, importTime, "", importUUID, importSQLx},
		},
		{
			name: "own module in its own group",
			in:   []string{importNetHTTP, testModule, importUUID},
			want: []string{importNetHTTP, "", importUUID, "", testModule},
		},
		{
			name: "own subpackage in its own group",
			in:   []string{testModule + "/restapi", importContext},
			want: []string{importContext, "", testModule + "/restapi"},
		},
		{
			name:   "dotless module is not grouped with stdlib",
			module: "blogservice/internal/blog",
			in:     []string{importNetHTTP, "blogservice/internal/blog", importUUID},
			want:   []string{importNetHTTP, "", importUUID, "", "blogservice/internal/blog"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			set := make(map[string]struct{}, len(tc.in))
			for _, imp := range tc.in {
				set[imp] = struct{}{}
			}
			module := tc.module
			if module == "" {
				module = testModule
			}
			require.Equal(t, tc.want, groupImports(set, module))
		})
	}
}

func TestCheckColumns(t *testing.T) {
	t.Parallel()

	id := spec.Field{Name: "id", Type: spec.TypeUUID, Primary: true}
	cases := []struct {
		name    string
		fields  []spec.Field
		options spec.EntityOptions
		wantErr bool
	}{
		{"distinct", []spec.Field{id, {Name: "title", Type: spec.TypeString}}, spec.EntityOptions{Timestamps: true}, false},
		{"duplicate name", []spec.Field{id, {Name: "title", Type: spec.TypeString}, {Name: "title", Type: spec.TypeText}}, spec.EntityOptions{}, true},
		{"same column", []spec.Field{id, {Name: "authorName", Type: spec.TypeString}, {Name: "author_name", Type: spec.TypeString}}, spec.EntityOptions{}, true},
		{"same go field", []spec.Field{id, {Name: "Id", Type: spec.TypeString}}, spec.EntityOptions{}, true},
		{"timestamps collision", []spec.Field{id, {Name: "created_at", Type: spec.TypeDatetime}}, spec.EntityOptions{Timestamps: true}, true},
		{"option column without option", []spec.Field{id, {Name: "created_at", Type: spec.TypeDatetime}}, spec.EntityOptions{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkColumns(&spec.Entity{Name: "Post", Fields: tc.fields, Options: tc.options})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRenderMigration_KeyGeneration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		keyType string
		want    string
	}{
		{"uuid", spec.TypeUUID, "id UUID NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY"},
		{"int32", spec.TypeInt32, "id INTEGER NOT NULL GENERATED ALWAYS AS IDENTITY PRIMARY KEY"},
		{"int64", spec.TypeInt64, "id BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY PRIMARY KEY"},
		{"string", spec.TypeString, "id TEXT NOT NULL PRIMARY KEY"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{
				Package: "app",
				Module:  "example.com/app",
				Entities: []spec.Entity{{
					Name:   "Item",
					Fields: []spec.Field{{Name: "id", Type: tc.keyType, Primary: true}},
				}},
			}
			wantContains(t, renderMigrationSrc(t, s, "Item"), tc.want)
		})
	}
}

func TestGoLiteral(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   any
		want string
	}{
		{"bool", true, "true"},
		{"int", 42, "42"},
		{"float", 1.5, "1.5"},
		{"string", `say "hi"`, `"say \"hi\""`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := goLiteral(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNowDefault(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "app",
		Module:  "example.com/app",
		Entities: []spec.Entity{{
			Name: "Event",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "occurred_at", Type: spec.TypeDatetime, Default: spec.DefaultNow},
			},
		}},
	}

	wantContains(t, renderMigrationSrc(t, s, "Event"), "occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()")
	wantContains(t, render(t, s, "Event"), "OccurredAt time.Time `json:\"occurred_at\"`")

	handler := renderHandlerSrc(t, s, "Event")
	wantContains(t, handler, `"time"`)
	wantContains(t, handler, "OccurredAt: valueOr(req.OccurredAt, time.Now()),")
}

func TestRenderRepo_KeyGeneration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		keyType string
		want    []string
		notWant string
	}{
		{"uuid in code", spec.TypeUUID, []string{
			"INSERT INTO items (id, name) VALUES ($1, $2)`",
			"m.ID = uuid.New()",
		}, "RETURNING"},
		{"int64 identity in database", spec.TypeInt64, []string{
			"INSERT INTO items (name) VALUES ($1) RETURNING id`",
			"Scan(&m.ID)",
		}, "m.ID ="},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{
				Package: "app",
				Module:  "example.com/app",
				Entities: []spec.Entity{{
					Name: "Item",
					Fields: []spec.Field{
						{Name: "id", Type: tc.keyType, Primary: true},
						{Name: "name", Type: spec.TypeString, Required: true},
					},
				}},
			}
			got := renderRepoSrc(t, s, "Item")
			for _, want := range tc.want {
				wantContains(t, got, want)
			}
			require.NotContains(t, got, tc.notWant)
		})
	}
}

func TestDomainTypesQualifiedOutsideRoot(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{{
			Name: "Author",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "born_on", Type: spec.TypeDate},
			},
		}},
	}

	wantContains(t, render(t, s, "Author"), "BornOn *Date")
	wantContains(t, renderHandlerSrc(t, s, "Author"), "BornOn *blog.Date")
	wantContains(t, renderRepoSrc(t, s, "Author"), "BornOn sql.Null[blog.Date]")
}

func TestRenderErrors_Sentinels(t *testing.T) {
	t.Parallel()

	src, err := renderErrors(packageData{Package: "blog"})
	require.NoError(t, err)
	requireParses(t, src)

	got := string(src)
	for _, want := range []string{
		"package blog",
		`ErrNotFound = errors.New("not found")`,
		`ErrAlreadyExists = errors.New("already exists")`,
		`ErrReferenceNotFound = errors.New("referenced entity not found")`,
		`ErrStillReferenced = errors.New("entity is still referenced")`,
	} {
		wantContains(t, got, want)
	}
}

func TestCheckCollisions(t *testing.T) {
	t.Parallel()

	goFile := func(path, src string) genFile {
		return genFile{Path: path, Src: []byte(src)}
	}

	cases := []struct {
		name    string
		files   []genFile
		wantErr bool
	}{
		{"distinct names", []genFile{
			goFile("a.gen.go", "package blog\ntype A struct{}"),
			goFile("b.gen.go", "package blog\ntype B struct{}"),
		}, false},
		{"same name in different packages", []genFile{
			goFile("post.gen.go", "package blog\ntype PostRepository interface{}"),
			goFile("postgres/post.gen.go", "package postgres\ntype PostRepository struct{}"),
		}, false},
		{"methods and blank identifiers are not declarations", []genFile{
			goFile("a.gen.go", "package blog\ntype A struct{}\nfunc (A) M() {}\nvar _ = 1"),
			goFile("b.gen.go", "package blog\ntype B struct{}\nfunc (B) M() {}\nvar _ = 2"),
		}, false},
		{"non-Go files are not parsed", []genFile{
			goFile("migrations/00001_create_dates.sql", "CREATE TABLE dates ();"),
		}, false},
		{"duplicate type in one package", []genFile{
			goFile("date.gen.go", "package blog\ntype Date struct{}"),
			goFile("calendar_date.gen.go", "package blog\ntype Date int"),
		}, true},
		{"duplicate func and var in one package", []genFile{
			goFile("postgres/a.gen.go", "package postgres\nfunc NewDB() {}"),
			goFile("postgres/b.gen.go", "package postgres\nvar NewDB = 1"),
		}, true},
		{"duplicate path", []genFile{
			goFile("date.gen.go", "package blog\ntype Date struct{}"),
			goFile("date.gen.go", "package blog\ntype Other struct{}"),
		}, true},
		{"paths differing only in case", []genFile{
			goFile("migrations/00001_create_Posts.sql", ""),
			goFile("migrations/00001_create_posts.sql", ""),
		}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkCollisions(tc.files)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRenderFiles_EntityNamesCollideWithGeneratedCode(t *testing.T) {
	t.Parallel()

	id := spec.Field{Name: "id", Type: spec.TypeUUID, Primary: true}
	cases := []struct {
		name     string
		entities []spec.Entity
		wantErr  bool
	}{
		{"entity Date without date fields", []spec.Entity{
			{Name: "Date", Fields: []spec.Field{id}},
		}, false},
		{"entity Date next to a date field", []spec.Entity{
			{Name: "Date", Fields: []spec.Field{id, {Name: "on", Type: spec.TypeDate}}},
		}, true},
		{"entity Errors", []spec.Entity{
			{Name: "Errors", Fields: []spec.Field{id}},
		}, true},
		{"entity Router", []spec.Entity{
			{Name: "Router", Fields: []spec.Field{id}},
		}, true},
		{"entity NewPost next to Post", []spec.Entity{
			{Name: "Post", Fields: []spec.Field{id}},
			{Name: "NewPost", Fields: []spec.Field{id}},
		}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "app", Module: "example.com/app", Entities: tc.entities}
			files, err := renderFiles(s, sqlDriverPgx, importDriverPgx)
			require.NoError(t, err)
			err = checkCollisions(files)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
