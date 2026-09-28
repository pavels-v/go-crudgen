package generator

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"

	"github.com/pavels-v/go-crudgen/internal/spec"
)

func findEntity(t *testing.T, s *spec.Spec, name string) *spec.Entity {
	t.Helper()

	e, ok := entitiesByName(s)[name]
	require.Truef(t, ok, "entity %q not found in spec", name)

	return e
}

// render runs renderModel for the named entity and returns the generated
// source, failing the test on error or if the output is not valid Go.
func render(t *testing.T, s *spec.Spec, entity string) string {
	t.Helper()

	byName := entitiesByName(s)
	e := findEntity(t, s, entity)

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
		"package domain",
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
	} {
		wantContains(t, got, want)
	}

	require.NotContains(t, got, "interface {", "domain model should not declare the repository interface")

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

func TestRenderModel_GeneratedTimestamps(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "app",
		Module:  "example.com/app",
		Entities: []spec.Entity{{
			Name: "Session",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate},
				{Name: "updated_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnWrite},
			},
		}},
	}

	got := render(t, s, "Session")
	wantContains(t, got, "CreatedAt time.Time `json:\"created_at\"`")
	wantContains(t, got, "UpdatedAt time.Time `json:\"updated_at\"`")
}

// renderHandlerSrc runs handlerInfo + renderHandler for the named entity and
// returns the generated source, failing if generation errors or the output is
// not valid Go.
func renderHandlerSrc(t *testing.T, s *spec.Spec, entity string) string {
	t.Helper()

	byName := entitiesByName(s)
	e := findEntity(t, s, entity)

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
				{Name: "slug", Type: spec.TypeString, Required: true, Validate: "min=3"},
				{Name: "bio", Type: spec.TypeText, Validate: "max=500"},
				{Name: "views", Type: spec.TypeInt64, Default: 0, Validate: "gte=0"},
				{Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate},
			},
		}},
	}

	got := renderHandlerSrc(t, s, "Post")
	for _, want := range []string{
		"type CreatePostRequest struct {",
		"type UpdatePostRequest struct {",
		"Title string",
		`json:"title" validate:"required"`,
		`json:"slug" validate:"required,min=3"`,
		"Bio *string `json:\"bio,omitzero\" validate:\"omitnil,max=500\"`",
		"Views *int64 `json:\"views\" validate:\"omitnil,gte=0\"`",
	} {
		wantContains(t, got, want)
	}
	// The update body omits the primary key (it comes from the path)...
	require.NotContains(t, strings.Join(strings.Fields(got), " "),
		"type UpdatePostRequest struct { ID uuid.UUID",
		"UpdatePostRequest should not contain the primary key field")
	// ...and DTOs never carry generated fields.
	_, create, ok := strings.Cut(got, "type CreatePostRequest")
	require.True(t, ok, "CreatePostRequest should be generated")
	require.NotContains(t, create, "CreatedAt", "DTOs should not contain generated fields")
	require.NotContains(t, create, "ID uuid.UUID", "a generated key is not part of CreatePostRequest")
}

func TestRenderHandler_RequiredFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field spec.Field
		want  []string
	}{
		{
			name:  "bool accepts false",
			field: spec.Field{Name: "active", Type: spec.TypeBool, Required: true},
			want:  []string{"Active *bool `json:\"active\" validate:\"required\"`", "Active: *req.Active,"},
		},
		{
			name:  "int keeps value rules",
			field: spec.Field{Name: "qty", Type: spec.TypeInt32, Required: true, Validate: "gte=0"},
			want:  []string{"Qty *int32 `json:\"qty\" validate:\"required,gte=0\"`", "Qty: *req.Qty,"},
		},
		{
			name:  "decimal",
			field: spec.Field{Name: "price", Type: spec.TypeDecimal, Required: true},
			want:  []string{"Price *decimal.Decimal `json:\"price\" validate:\"required\"`", "Price: *req.Price,"},
		},
		{
			name:  "string rejects empty",
			field: spec.Field{Name: "code", Type: spec.TypeString, Required: true},
			want:  []string{"Code string `json:\"code\" validate:\"required\"`", "Code: req.Code,"},
		},
		{
			name:  "required_with does not replace required",
			field: spec.Field{Name: "code", Type: spec.TypeString, Required: true, Validate: "required_with=Qty"},
			want:  []string{"Code string `json:\"code\" validate:\"required,required_with=Qty\"`"},
		},
		{
			name:  "explicit required is not repeated",
			field: spec.Field{Name: "code", Type: spec.TypeString, Required: true, Validate: "required,min=3"},
			want:  []string{"Code string `json:\"code\" validate:\"required,min=3\"`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{
				Package: "shop",
				Module:  "example.com/shop",
				Entities: []spec.Entity{{
					Name: "Item",
					Fields: []spec.Field{
						{Name: "id", Type: spec.TypeInt64, Primary: true},
						tc.field,
					},
				}},
			}

			got := renderHandlerSrc(t, s, "Item")
			for _, want := range tc.want {
				wantContains(t, got, want)
			}
		})
	}
}

func TestRenderHandler_ClientKeyReferencingIntKey(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "shop",
		Module:  "example.com/shop",
		Entities: []spec.Entity{
			{
				Name:   "Item",
				Fields: []spec.Field{{Name: "id", Type: spec.TypeInt64, Primary: true}},
			},
			{
				Name:   "Stock",
				Fields: []spec.Field{{Name: "item", Type: spec.TypeReferences, Target: "Item", Primary: true}},
			},
		},
	}
	got := renderHandlerSrc(t, s, "Stock")
	wantContains(t, got, "Item *int64 `json:\"item\" validate:\"required\"`")
	wantContains(t, got, "Item: *req.Item,")
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
		"defaultTagPublished = false",
		"Published: valueOr(req.Published, defaultTagPublished),",
	} {
		wantContains(t, handler, want)
	}

	repo := renderRepoSrc(t, s, "Tag")
	wantContains(t, repo, "r.db.ExecContext(ctx, `INSERT INTO tags (slug, published) VALUES ($1, $2)`, row.Slug, row.Published, )")
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
		"func (h *PostHandler) RegisterRoutes(mux *http.ServeMux) {",
		`mux.HandleFunc("POST /posts", h.Create)`,
		`mux.HandleFunc("GET /posts", h.List)`,
		`mux.HandleFunc("GET /posts/{id}", h.Get)`,
		`mux.HandleFunc("PUT /posts/{id}", h.Update)`,
		`mux.HandleFunc("DELETE /posts/{id}", h.Delete)`,
		`"example.com/blog/domain"`,
		"type PostRepository interface { Create(ctx context.Context, m *domain.Post) error Get(ctx context.Context, id uuid.UUID) (*domain.Post, error) List(ctx context.Context, p domain.PostListParams) ([]domain.Post, error)",
		"repo PostRepository",
		"m := domain.Post{",
		"id, ok := pathID(w, r, parseText[uuid.UUID]) if !ok { return }",
		"writeDecodeError(w, r, err)",
		"writeValidationError(w, r, err)",
		"writeBody(w, r, http.StatusCreated, m)",
		"q := newListQuery(r, queryOffset) limit := q.limit() p := domain.PostListParams{ Dir: q.dir(), Limit: limit + 1, Offset: q.offset(), }",
		"items, err := h.repo.List(r.Context(), p)",
		"items, more := trimPage(items, limit)",
		"writeBody(w, r, http.StatusOK, offsetPage[domain.Post]{Items: items, Limit: limit, Offset: p.Offset, HasMore: more})",
		"writeError(w, r, http.StatusBadRequest, codeInvalidQuery, \"invalid query parameters\", q.details...)",
		"w.WriteHeader(http.StatusNoContent)",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderHandler_StringPKUsesParseString(t *testing.T) {
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
	wantContains(t, got, "id, ok := pathID(w, r, parseString)")
}

func TestRenderHandler_Int32PKUsesParseInt32(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "shop",
		Module:  "example.com/shop",
		Entities: []spec.Entity{{
			Name:   "Widget",
			Fields: []spec.Field{{Name: "id", Type: spec.TypeInt32, Primary: true}},
		}},
	}

	wantContains(t, renderHandlerSrc(t, s, "Widget"), "id, ok := pathID(w, r, parseInt32)")

	wantContains(t, renderHandlerSrc(t, s, "Widget"), "Get(ctx context.Context, id int32) (*domain.Widget, error)")
}

// renderRepoSrc runs repoInfo + renderRepo for the named entity and returns the
// generated source, failing if generation errors or the output is not valid Go.
func renderRepoSrc(t *testing.T, s *spec.Spec, entity string) string {
	t.Helper()

	byName := entitiesByName(s)
	e := findEntity(t, s, entity)

	rd, err := repoInfo(s, e, byName)
	require.NoErrorf(t, err, "repoInfo(%s)", entity)

	src, err := renderRepo(rd)
	require.NoErrorf(t, err, "renderRepo(%s)", entity)
	requireParses(t, src)

	return string(src)
}

func TestRenderRepo_SQL(t *testing.T) {
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
				{Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate},
				{Name: "updated_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnWrite},
			},
		}},
	}

	got := renderRepoSrc(t, s, "Post")
	for _, want := range []string{
		"package postgres",
		`"example.com/blog/domain"`,
		"func NewPostRepository(db *sqlx.DB) *PostRepository",
		"func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Post, error)",
		"return nil, domain.ErrNotFound",
		// the primary-key type's package is imported for the Get/Delete signatures
		`"github.com/google/uuid"`,
		`"github.com/jmoiron/sqlx"`,
		// the row struct carries db tags; nullable body becomes sql.Null[T]
		"type postRow struct {",
		"Body sql.Null[string]",
		"func newPostRow(m *domain.Post) postRow",
		"Body: toNull(m.Body)",
		"Body: fromNull(row.Body)",
		// sqlx scans into the row, which is then converted to the API model
		"r.db.GetContext(ctx, &row, `SELECT id, title, body, created_at, updated_at FROM posts WHERE id = $1`, id, )",
		"m := row.toModel()",
		"errors.Is(err, sql.ErrNoRows)",
		// generated columns take now() on insert and are returned into the struct
		"INSERT INTO posts (id, title, body, created_at, updated_at) VALUES ($1, $2, $3, now(), now()) RETURNING created_at, updated_at",
		"m.ID = uuid.New() row := newPostRow(m)",
		"r.db.QueryRowContext(ctx, `INSERT INTO posts (id, title, body, created_at, updated_at) VALUES ($1, $2, $3, now(), now()) RETURNING created_at, updated_at`, row.ID, row.Title, row.Body, ).Scan(&m.CreatedAt, &m.UpdatedAt)",
		"SELECT id, title, body, created_at, updated_at FROM posts WHERE id = $1",
		"q := `SELECT id, title, body, created_at, updated_at FROM posts ORDER BY ` + order + ` LIMIT $1 OFFSET $2`",
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
		"order := `id` if p.Dir == domain.SortDesc { order = `id DESC` }",
		"q := `SELECT id, name FROM accounts ORDER BY ` + order + ` LIMIT $1 OFFSET $2` var rows []accountRow err := r.db.SelectContext(ctx, &rows, q, p.Limit, p.Offset, ) if err != nil {",
		"DELETE FROM accounts WHERE id = $1",
		// no timestamps -> Exec + RowsAffected for the not-found check
		"res.RowsAffected()",
		"if n == 0 {",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderList_FiltersAndOrder(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{
			{Name: "Author", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}},
			{Name: "Post", Plural: "posts", Order: "title", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true, Filter: true},
				{Name: "author", Type: spec.TypeReferences, Target: "Author", Filter: true},
				{Name: "published_on", Type: spec.TypeDate, Filter: true},
				{Name: "views", Type: spec.TypeInt32, Filter: true},
				{Name: "published", Type: spec.TypeBool, Filter: true},
			}},
			{Name: "Tag", Order: "label", Fields: []spec.Field{
				{Name: "slug", Type: spec.TypeString, Primary: true},
				{Name: "label", Type: spec.TypeString, Required: true},
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
			name:   "domain params",
			render: render,
			entity: "Post",
			want: []string{
				"type PostListParams struct { Title *string Author *uuid.UUID PublishedOn *Date Views *int32 Published *bool Dir SortDir Limit int Offset int }",
			},
		},
		{
			name:   "handler parses filters and direction",
			render: renderHandlerSrc,
			entity: "Post",
			want: []string{
				`queryPostTitle = "title"`,
				`queryPostPublishedOn = "published_on"`,
				"q := newListQuery(r, queryOffset, queryPostTitle, queryPostAuthor, queryPostPublishedOn, queryPostViews, queryPostPublished)",
				"Title: queryValue(q, queryPostTitle, parseString),",
				"Author: queryValue(q, queryPostAuthor, parseText[uuid.UUID]),",
				"PublishedOn: queryValue(q, queryPostPublishedOn, parseText[domain.Date]),",
				"Views: queryValue(q, queryPostViews, parseInt32),",
				"Published: queryValue(q, queryPostPublished, strconv.ParseBool),",
				"Dir: q.dir(),",
				`"strconv"`,
			},
		},
		{
			name:   "repository assembles where and orders by the entity field",
			render: renderRepoSrc,
			entity: "Post",
			want: []string{
				"if p.Author != nil { where = append(where, `author = ?`) args = append(args, *p.Author) }",
				"if p.PublishedOn != nil { where = append(where, `published_on = ?`)",
				"q := `SELECT id, title, author, published_on, views, published FROM posts`",
				"q += ` WHERE ` + strings.Join(where, ` AND `)",
				"order := `title, id` if p.Dir == domain.SortDesc { order = `title DESC, id DESC` }",
				"args = append(args, p.Limit, p.Offset) q += ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`",
				"r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args..., )",
				`"strings"`,
			},
		},
		{
			name:   "no filters keeps one inline query",
			render: renderRepoSrc,
			entity: "Tag",
			want: []string{
				"order := `label, slug` if p.Dir == domain.SortDesc { order = `label DESC, slug DESC` }",
				"q := `SELECT slug, label FROM tags ORDER BY ` + order + ` LIMIT $1 OFFSET $2`",
			},
			absent: []string{"where = append", "Rebind", `"strings"`},
		},
		{
			name:   "handler without filters accepts only paging and direction",
			render: renderHandlerSrc,
			entity: "Tag",
			want: []string{
				"q := newListQuery(r, queryOffset)",
				"p := domain.TagListParams{ Dir: q.dir(),",
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

func TestRenderList_CursorPagination(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{
			{Name: "Post", Plural: "posts", Pagination: spec.PaginationCursor, Order: "created_at", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true, Filter: true},
				{Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate},
			}},
			{Name: "Tag", Pagination: spec.PaginationCursor, Fields: []spec.Field{
				{Name: "slug", Type: spec.TypeString, Primary: true},
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
			name:   "domain cursor replaces offset",
			render: render,
			entity: "Post",
			want: []string{
				"type PostCursor struct { CreatedAt time.Time `json:\"created_at\"` ID uuid.UUID `json:\"id\"` }",
				"type PostListParams struct { Title *string Dir SortDir After *PostCursor Limit int }",
			},
			absent: []string{"Offset"},
		},
		{
			name:   "repository seeks past the cursor",
			render: renderRepoSrc,
			entity: "Post",
			want: []string{
				"order, after := `created_at, id`, `(created_at, id) > (?, ?)` if p.Dir == domain.SortDesc { order, after = `created_at DESC, id DESC`, `(created_at, id) < (?, ?)` }",
				"if p.After != nil { where = append(where, after) args = append(args, p.After.CreatedAt, p.After.ID) }",
				"args = append(args, p.Limit) q += ` ORDER BY ` + order + ` LIMIT ?`",
			},
			absent: []string{"OFFSET"},
		},
		{
			name:   "key-ordered repository compares the key alone",
			render: renderRepoSrc,
			entity: "Tag",
			want: []string{
				"order, after := `slug`, `slug > ?` if p.Dir == domain.SortDesc { order, after = `slug DESC`, `slug < ?` }",
				"args = append(args, p.After.Slug)",
			},
		},
		{
			name:   "handler decodes and encodes the cursor",
			render: renderHandlerSrc,
			entity: "Post",
			want: []string{
				"type postCursor struct { Dir domain.SortDir `json:\"dir\"` After domain.PostCursor `json:\"after\"` }",
				"q := newListQuery(r, queryCursor, queryPostTitle)",
				"if c := queryCursorValue[postCursor](q); c != nil { if c.Dir != p.Dir { q.invalid(queryCursor) } p.After = &c.After }",
				"page := cursorPage[domain.Post]{Items: items}",
				"c := postCursor{ Dir: p.Dir, After: domain.PostCursor{ CreatedAt: last.CreatedAt, ID: last.ID, }, }",
				"if page.NextCursor, err = encodeCursor(c); err != nil {",
			},
			absent: []string{"q.offset()", "offsetPage"},
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

func TestRenderRouter_PaginationHelpers(t *testing.T) {
	t.Parallel()

	offset := []string{`queryOffset = "offset"`, "type offsetPage[T any]", "func (q *listQuery) offset() int"}
	cursor := []string{`queryCursor = "cursor"`, "type cursorPage[T any]", "func queryCursorValue[C any]", "func encodeCursor(", `"encoding/base64"`}
	cases := []struct {
		name   string
		shared sharedFiles
		want   []string
		absent []string
	}{
		{name: "offset only", shared: sharedFiles{Offset: true}, want: offset, absent: cursor},
		{name: "cursor only", shared: sharedFiles{Cursor: true}, want: cursor, absent: offset},
		{name: "both", shared: sharedFiles{Offset: true, Cursor: true}, want: slices.Concat(offset, cursor)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := renderRESTSrc(t, &spec.Spec{Package: "blog", Module: "example.com/blog"}, tc.shared)
			for _, want := range tc.want {
				wantContains(t, got, want)
			}

			for _, absent := range tc.absent {
				require.NotContains(t, got, absent)
			}
		})
	}
}

func TestRenderRouter_ParseHelpers(t *testing.T) {
	t.Parallel()

	helpers := map[string]string{
		parseString: "func parseString(",
		parseInt32:  "func parseInt32(",
		parseInt64:  "func parseInt64(",
		parseText:   "func parseText[",
	}
	cases := []struct {
		name    string
		parsers map[string]bool
	}{
		{name: "none", parsers: map[string]bool{}},
		{name: "string only", parsers: map[string]bool{parseString: true}},
		{name: "ints", parsers: map[string]bool{parseInt32: true, parseInt64: true}},
		{name: "text", parsers: map[string]bool{parseText: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := renderRESTSrc(t, &spec.Spec{Package: "blog", Module: "example.com/blog"}, sharedFiles{Offset: true, Parsers: tc.parsers})
			for helper, decl := range helpers {
				if tc.parsers[helper] {
					wantContains(t, got, decl)
				} else {
					require.NotContains(t, got, decl)
				}
			}

			if tc.parsers[parseText] {
				wantContains(t, got, `"encoding"`)
			} else {
				require.NotContains(t, got, `"encoding"`)
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
		"r.db.GetContext(ctx, &exists, `SELECT 1 FROM tags WHERE id = $1`, row.ID, )",
		"return domain.ErrNotFound",
	} {
		wantContains(t, got, want)
	}
	// must NOT emit a malformed empty SET clause
	require.NotContains(t, strings.Join(strings.Fields(got), " "), "SET WHERE",
		"primary-key-only entity must not generate an empty UPDATE SET clause")
}

func TestRenderRepo_GeneratedColumns(t *testing.T) {
	t.Parallel()

	id := spec.Field{Name: "id", Type: spec.TypeInt64, Primary: true}
	name := spec.Field{Name: "name", Type: spec.TypeString, Required: true}
	cases := []struct {
		name   string
		fields []spec.Field
		want   []string
	}{
		{
			name:   "on_create is set on insert and only returned on update",
			fields: []spec.Field{id, name, {Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate}},
			want: []string{
				"INSERT INTO events (name, created_at) VALUES ($1, now()) RETURNING id, created_at",
				"UPDATE events SET name = $1 WHERE id = $2 RETURNING created_at`, row.Name, row.ID, ).Scan(&m.CreatedAt)",
			},
		},
		{
			name:   "on_write is reset on every update",
			fields: []spec.Field{id, name, {Name: "updated_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnWrite}},
			want: []string{
				"UPDATE events SET name = $1, updated_at = now() WHERE id = $2 RETURNING updated_at",
			},
		},
		{
			name:   "nothing writable still reads back on_create",
			fields: []spec.Field{id, {Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate}},
			want: []string{
				"r.db.QueryRowContext(ctx, `SELECT created_at FROM events WHERE id = $1`, row.ID, ).Scan(&m.CreatedAt)",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "app", Module: "example.com/app", Entities: []spec.Entity{{Name: "Event", Fields: tc.fields}}}

			got := renderRepoSrc(t, s, "Event")
			for _, want := range tc.want {
				wantContains(t, got, want)
			}

			require.NotContains(t, got, "CreatedAt: m.CreatedAt", "generated columns are not written from the model")
		})
	}
}

func TestRepoInfo_HasNullable(t *testing.T) {
	t.Parallel()

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
			rd, err := repoInfo(s, &s.Entities[0], entitiesByName(s))
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

	byName := entitiesByName(s)
	e := findEntity(t, s, entity)

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
				{Name: "created_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnCreate},
				{Name: "updated_at", Type: spec.TypeDatetime, Generate: spec.GenerateOnWrite},
			}},
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

func TestRenderFiles_MigrationVersions(t *testing.T) {
	t.Parallel()

	uuidKey := spec.Field{Name: "id", Type: spec.TypeUUID, Primary: true}
	s := &spec.Spec{Package: "blog", Module: "example.com/blog", Entities: []spec.Entity{
		{Name: "Post", Fields: []spec.Field{uuidKey, {Name: "author", Type: spec.TypeReferences, Target: "Author"}}},
		{Name: "Author", Fields: []spec.Field{uuidKey}},
	}}

	files, err := renderFiles(s, entitiesByName(s), sqlDriverPgx, importDriverPgx, Options{MigrationTime: time.Date(2026, 1, 2, 3, 4, 59, 0, time.UTC)})
	require.NoError(t, err)

	var got []string
	for _, f := range files {
		if strings.HasPrefix(f.Path, dirMigrations+"/") {
			got = append(got, f.Path)
		}
	}

	slices.Sort(got)
	require.Equal(t, []string{
		"migrations/20260102030459_create_authors.sql",
		"migrations/20260102030500_create_posts.sql",
	}, got)
}

func TestCheckOutDir(t *testing.T) {
	t.Parallel()

	targets := []genFile{{Path: "post.go"}, {Path: "restapi/post.go"}, {Path: "postgres/post.go"}}
	cases := []struct {
		name    string
		files   []string
		wantErr bool
	}{
		{name: "empty"},
		{name: "hand-written files only", files: []string{"go.mod", "service.go", "cmd/main.go", "restapi/api_test.go", "postgres/post_test.go"}},
		{name: "target in the root", files: []string{"post.go"}, wantErr: true},
		{name: "target in a subpackage", files: []string{"restapi/post.go"}, wantErr: true},
		{name: "other restapi code", files: []string{"restapi/middleware.go"}, wantErr: true},
		{name: "other postgres code", files: []string{"postgres/tx.go"}, wantErr: true},
		{name: "any migration", files: []string{"migrations/20260101000000_add_index.sql"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, f := range tc.files {
				p := filepath.Join(dir, filepath.FromSlash(f))
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, nil, 0o600))
			}

			err := checkOutDir(dir, targets)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}

	t.Run("missing directory", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, checkOutDir(filepath.Join(t.TempDir(), "absent"), targets))
	})
}

func TestMigrationOrder(t *testing.T) {
	t.Parallel()

	uuidKey := spec.Field{Name: "id", Type: spec.TypeUUID, Primary: true}
	cases := []struct {
		name     string
		entities []spec.Entity
		want     []string
		wantErr  bool
	}{
		{
			// Post references Author but is declared first; Author must still come first
			// so its table exists when the posts foreign key is created.
			name: "referenced entity is ordered first",
			entities: []spec.Entity{
				{Name: "Post", Fields: []spec.Field{uuidKey, {Name: "author", Type: spec.TypeReferences, Target: "Author"}}},
				{Name: "Author", Fields: []spec.Field{uuidKey}},
			},
			want: []string{"Author", "Post"},
		},
		{
			name: "self-reference is allowed",
			entities: []spec.Entity{
				{Name: "Node", Fields: []spec.Field{uuidKey, {Name: "parent", Type: spec.TypeReferences, Target: "Node"}}},
			},
			want: []string{"Node"},
		},
		{
			name: "reference cycle errors",
			entities: []spec.Entity{
				{Name: "A", Fields: []spec.Field{uuidKey, {Name: "b", Type: spec.TypeReferences, Target: "B"}}},
				{Name: "B", Fields: []spec.Field{uuidKey, {Name: "a", Type: spec.TypeReferences, Target: "A"}}},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Entities: tc.entities}

			order, err := migrationOrder(s.Entities, entitiesByName(s))
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			names := make([]string, len(order))
			for i, e := range order {
				names[i] = e.Name
			}

			require.Equal(t, tc.want, names)
		})
	}
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
		{name: "string", field: spec.Field{Type: spec.TypeString}, want: sqlText},
		{name: "int32", field: spec.Field{Type: spec.TypeInt32}, want: sqlInteger},
		{name: "int64", field: spec.Field{Type: spec.TypeInt64}, want: sqlBigint},
		{name: "float", field: spec.Field{Type: spec.TypeFloat}, want: sqlDouble},
		{name: "decimal", field: spec.Field{Type: spec.TypeDecimal}, want: sqlNumeric},
		{name: "bool", field: spec.Field{Type: spec.TypeBool}, want: sqlBoolean},
		{name: "date", field: spec.Field{Type: spec.TypeDate}, want: sqlDate},
		{name: "datetime", field: spec.Field{Type: spec.TypeDatetime}, want: sqlTimestamptz},
		{name: "uuid", field: spec.Field{Type: spec.TypeUUID}, want: sqlUUID},
		{name: "json", field: spec.Field{Type: spec.TypeJSON}, want: sqlJSONB},
		{name: "reference takes target PK type", field: spec.Field{Type: spec.TypeReferences, Target: "Author"}, want: sqlBigint},
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
		{name: "default is pgx", wantName: sqlDriverPgx, wantImport: importDriverPgx},
		{name: "explicit pgx", driver: DriverPgx, wantName: sqlDriverPgx, wantImport: importDriverPgx},
		{name: "pq maps to postgres", driver: DriverPq, wantName: sqlDriverPq, wantImport: importDriverPq},
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
			wantContains(t, got, fmt.Sprintf("driverName = %q", tc.wantName))
			wantContains(t, got, fmt.Sprintf("_ %q", tc.wantImport))
			wantContains(t, got, "func NewDB(dsn string) (*sqlx.DB, error)")
			wantContains(t, got, "db, err := sqlx.Open(driverName, dsn)")
			wantContains(t, got, "db.SetMaxOpenConns(maxOpenConns)")
			wantContains(t, got, `sqlStateUniqueViolation = "23505"`)
			wantContains(t, got, "package postgres")
			wantContains(t, got, `"example.com/blog/domain" )`)
			wantContains(t, got, `return fmt.Errorf("%w: %v", domain.ErrAlreadyExists, err)`)
			wantContains(t, got, `return fmt.Errorf("%w: %v", domain.ErrStillReferenced, err)`)
		})
	}
}

func TestDriverInfo_RejectsUnknown(t *testing.T) {
	t.Parallel()

	_, _, ok := driverInfo("mysql")
	require.False(t, ok, "unknown driver should be rejected")
}

func renderRESTSrc(t *testing.T, s *spec.Spec, shared sharedFiles) string {
	t.Helper()

	files, err := restFiles(s, shared)
	require.NoError(t, err)

	var b strings.Builder
	for _, f := range files {
		requireParses(t, f.Src)
		b.Write(f.Src)
	}

	return b.String()
}

func TestRestFiles_RouterFlag(t *testing.T) {
	t.Parallel()

	always := []string{fileRequest, fileResponse, fileQuery, fileRoutes}
	cases := []struct {
		name   string
		router bool
		want   []string
	}{
		{name: "without router", want: always},
		{name: "with router", router: true, want: append(slices.Clone(always), fileRouter)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files, err := restFiles(&spec.Spec{Package: "blog", Module: "example.com/blog"}, sharedFiles{Offset: true, Router: tc.router})
			require.NoError(t, err)

			paths := make([]string, len(files))
			for i, f := range files {
				paths[i] = f.Path
			}

			require.Equal(t, tc.want, paths)
		})
	}
}

func TestRestFiles_OptionalHelpers(t *testing.T) {
	t.Parallel()

	const (
		queryValueDecl = "func queryValue["
		valueOrDecl    = "func valueOr["
	)

	cases := []struct {
		name   string
		shared sharedFiles
		want   []string
		absent []string
	}{
		{name: "none", shared: sharedFiles{Offset: true}, absent: []string{queryValueDecl, valueOrDecl}},
		{name: "filters", shared: sharedFiles{Offset: true, Filters: true}, want: []string{queryValueDecl}, absent: []string{valueOrDecl}},
		{name: "defaults", shared: sharedFiles{Offset: true, Defaults: true}, want: []string{valueOrDecl}, absent: []string{queryValueDecl}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := renderRESTSrc(t, &spec.Spec{Package: "blog", Module: "example.com/blog"}, tc.shared)
			for _, want := range tc.want {
				wantContains(t, got, want)
			}

			for _, absent := range tc.absent {
				require.NotContains(t, got, absent)
			}
		})
	}
}

func TestMainFilePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		outDir  string
		mainDir string
		want    string
	}{
		{name: "stdout", mainDir: "cmd/blog", want: "cmd/blog/main.go"},
		{name: "sibling of the package", outDir: "internal/blog", mainDir: "cmd/blog", want: "../../cmd/blog/main.go"},
		{name: "inside the package", outDir: "internal/blog", mainDir: "internal/blog/cmd", want: "cmd/main.go"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := mainFilePath(tc.outDir, tc.mainDir)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDefaultMainDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		outDir  string
		modRoot string
		want    string
		wantErr bool
	}{
		{name: "module root", outDir: filepath.Join("svc", "internal", "blog"), modRoot: "svc", want: filepath.Join("svc", "cmd", "blog")},
		{name: "no module", outDir: filepath.Join("svc", "internal", "blog"), wantErr: true},
		{name: "stdout", want: filepath.Join("cmd", "blog")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := defaultMainDir(tc.outDir, tc.modRoot, "blog")
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestFindModuleRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		goMod  string
		outDir string
		want   string
	}{
		{name: "module above the package", goMod: "svc/go.mod", outDir: "svc/internal/blog", want: "svc"},
		{name: "module at the package", goMod: "svc/go.mod", outDir: "svc", want: "svc"},
		{name: "no module", outDir: "svc/internal/blog"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if tc.goMod != "" {
				writeFile(t, filepath.Join(root, filepath.FromSlash(tc.goMod)), "")
			}

			got, err := findModuleRoot(filepath.Join(root, filepath.FromSlash(tc.outDir)))
			require.NoError(t, err)

			if tc.want == "" {
				require.Empty(t, got)
				return
			}

			require.Equal(t, filepath.Join(root, filepath.FromSlash(tc.want)), got)
		})
	}
}

func TestResolveModule(t *testing.T) {
	t.Parallel()

	const goMod = "module example.com/svc\n\ngo 1.27\n"

	cases := []struct {
		name    string
		goMod   string
		outDir  string
		module  string
		want    string
		wantErr string
	}{
		{name: "derived below the module root", goMod: goMod, outDir: "internal/blog", want: "example.com/svc/internal/blog"},
		{name: "derived at the module root", goMod: goMod, outDir: ".", want: "example.com/svc"},
		{name: "matching module", goMod: goMod, outDir: "internal/blog", module: "example.com/svc/internal/blog", want: "example.com/svc/internal/blog"},
		{name: "mismatched module", goMod: goMod, outDir: "internal/blog", module: "example.com/other", wantErr: "does not match"},
		{name: "no module directive", goMod: "go 1.27\n", outDir: "internal/blog", wantErr: "no module directive"},
		{name: "no go.mod keeps the spec module", outDir: "internal/blog", module: "example.com/blog", want: "example.com/blog"},
		{name: "no go.mod and no module", outDir: "internal/blog", wantErr: "set module in the spec"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var modRoot string

			root := t.TempDir()
			if tc.goMod != "" {
				writeFile(t, filepath.Join(root, fileGoMod), tc.goMod)
				modRoot = root
			}

			s := &spec.Spec{Module: tc.module}

			err := resolveModule(s, filepath.Join(root, filepath.FromSlash(tc.outDir)), modRoot)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, s.Module)
		})
	}

	t.Run("stdout needs a module", func(t *testing.T) {
		t.Parallel()

		require.ErrorContains(t, resolveModule(&spec.Spec{}, "", ""), "module is not set")
	})
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
}

func TestMainFiles(t *testing.T) {
	t.Parallel()

	routes := []routerEntity{{Struct: "Post", RepoCtor: "NewPostRepository", DepsField: "Posts"}}
	cases := []struct {
		name   string
		router bool
		want   []string
	}{
		{
			name: "without router",
			want: []string{
				"restapi.NewPostHandler(postgres.NewPostRepository(db)).RegisterRoutes(mux)",
				"return restapi.WithRouteErrors(mux)",
			},
		},
		{
			name:   "with router",
			router: true,
			want:   []string{"return restapi.NewRouter(restapi.Deps{ Posts: postgres.NewPostRepository(db), })"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "blog", Module: "example.com/blog"}
			files, err := mainFiles(s, sharedFiles{Router: tc.router, Routes: routes}, Options{OutDir: "internal/blog", MainDir: "cmd/blog"})
			require.NoError(t, err)
			require.Len(t, files, 2)
			require.Equal(t, fileEmbed, files[0].Path)
			require.Equal(t, "../../cmd/blog/main.go", files[1].Path)

			for _, f := range files {
				requireParses(t, f.Src)
			}

			wantContains(t, string(files[0].Src), "//go:embed *.sql var FS embed.FS")

			got := string(files[1].Src)
			for _, want := range append([]string{
				`"example.com/blog/migrations"`,
				`"example.com/blog/postgres"`,
				`"example.com/blog/restapi"`,
				"goose.SetBaseFS(migrations.FS)",
				"signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)",
				"srv.Shutdown(shutdownCtx)",
			}, tc.want...) {
				wantContains(t, got, want)
			}
		})
	}
}

func TestRenderHandlerTest_Keys(t *testing.T) {
	t.Parallel()

	category := spec.Entity{Name: "Category", Fields: []spec.Field{{Name: "id", Type: spec.TypeInt32, Primary: true}}}
	cases := []struct {
		name   string
		entity spec.Entity
		want   []string
		absent []string
	}{
		{
			name:   "generated uuid key",
			entity: spec.Entity{Name: "Post", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}},
			want:   []string{"return &m.ID }, uuid.New)", "id: seeded.ID.String()", "id: uuid.NewString()", `id: "not-an-id"`},
		},
		{
			name:   "generated int32 key",
			entity: category,
			want:   []string{"var seq int32", "id: strconv.FormatInt(int64(seeded.ID), 10)", "id: strconv.FormatInt(int64(seeded.ID)+1, 10)"},
		},
		{
			name:   "client string key",
			entity: spec.Entity{Name: "Tag", Fields: []spec.Field{{Name: "slug", Type: spec.TypeString, Primary: true}}},
			want:   []string{"return &m.Slug }, nil)", `m := domain.Tag{Slug: "sample"}`, "id: seeded.Slug", `id: "missing"`, `Slug: "sample",`},
			absent: []string{"not-an-id"},
		},
		{
			name:   "client key referencing an int32 key",
			entity: spec.Entity{Name: "Stock", Fields: []spec.Field{{Name: "category", Type: spec.TypeReferences, Target: "Category", Primary: true}}},
			want:   []string{"m := domain.Stock{Category: int32(1)}", "Category: new(int32(1)),"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "blog", Module: "example.com/blog", Entities: []spec.Entity{category, tc.entity}}
			src, err := renderHandlerTest(s, &s.Entities[1], entitiesByName(s), validator.New())
			require.NoError(t, err)
			requireParses(t, src)

			got := string(src)
			for _, want := range tc.want {
				wantContains(t, got, want)
			}

			for _, absent := range tc.absent {
				require.NotContains(t, got, absent)
			}
		})
	}
}

func TestPickSample(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		fieldType string
		validate  string
		want      string
		wantOK    bool
	}{
		{name: "no rules", fieldType: spec.TypeString, want: `"sample"`, wantOK: true},
		{name: "email", fieldType: spec.TypeString, validate: "required,email", want: `"user@example.com"`, wantOK: true},
		{name: "url", fieldType: spec.TypeText, validate: "url", want: `"https://example.com"`, wantOK: true},
		{name: "e164", fieldType: spec.TypeString, validate: "e164", want: `"+14155552671"`, wantOK: true},
		{name: "exact length", fieldType: spec.TypeString, validate: "len=3", want: `"aaa"`, wantOK: true},
		{name: "short maximum", fieldType: spec.TypeString, validate: "max=2", want: `"a"`, wantOK: true},
		{name: "string enum", fieldType: spec.TypeString, validate: "oneof=red green", want: `"red"`, wantOK: true},
		{name: "number range", fieldType: spec.TypeInt64, validate: "gte=5,lte=200", want: "int64(100)", wantOK: true},
		{name: "range from params", fieldType: spec.TypeInt32, validate: "min=18,max=65", want: "int32(18)", wantOK: true},
		{name: "exclusive bound", fieldType: spec.TypeInt32, validate: "gt=1000", want: "int32(1001)", wantOK: true},
		{name: "number enum", fieldType: spec.TypeInt64, validate: "oneof=7 9", want: "int64(7)", wantOK: true},
		{name: "float above one", fieldType: spec.TypeFloat, validate: "gt=1", want: "1.5", wantOK: true},
		{name: "float below param", fieldType: spec.TypeFloat, validate: "lt=-3", want: "-3.5", wantOK: true},
		{name: "nothing fits", fieldType: spec.TypeString, validate: "hexcolor"},
	}

	v := validator.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := pickSample(v, samples(tc.fieldType, tc.validate), tc.validate)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got.expr)
		})
	}
}

func TestRenderFiles_Tests(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{Package: "blog", Module: "example.com/blog", Entities: []spec.Entity{
		{Name: "Post", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}},
	}}

	cases := []struct {
		name  string
		tests bool
	}{
		{name: "without tests"},
		{name: "with tests", tests: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files, err := renderFiles(s, entitiesByName(s), sqlDriverPgx, importDriverPgx, Options{Tests: tc.tests})
			require.NoError(t, err)

			paths := make([]string, len(files))
			for i, f := range files {
				paths[i] = f.Path
			}

			require.Equal(t, tc.tests, slices.Contains(paths, "restapi/post_test.go"))
			require.Equal(t, tc.tests, slices.Contains(paths, fileFakeTest))
		})
	}
}

func TestRenderRouter_WiresEntities(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{Package: "blog", Module: "example.com/blog"}
	got := renderRESTSrc(t, s, sharedFiles{Offset: true, Router: true, Defaults: true, Routes: []routerEntity{
		{Struct: "Post", Repo: "PostRepository", DepsField: "Posts"},
	}})

	for _, want := range []string{
		"package restapi",
		`"example.com/blog/domain"`,
		"domain.ErrNotFound: {status: http.StatusNotFound, code: codeNotFound},",
		"domain.ErrAlreadyExists: {status: http.StatusConflict, code: codeAlreadyExists},",
		"domain.ErrReferenceNotFound: {status: http.StatusUnprocessableEntity, code: codeReferenceNotFound},",
		"domain.ErrStillReferenced: {status: http.StatusConflict, code: codeStillReferenced},",
		`codeValidationFailed = "validation_failed"`,
		"type bodyResponse[T any] struct { Body T `json:\"body\"` }",
		"type errorResponse struct { Error apiError `json:\"error\"` }",
		"type apiError struct { Code string `json:\"code\"` Message string `json:\"message\"` Details []errorDetail `json:\"details\"` }",
		"type offsetPage[T any] struct { Items []T `json:\"items\"` Limit int `json:\"limit\"` Offset int `json:\"offset\"` HasMore bool `json:\"has_more\"` }",
		"func (q *listQuery) dir() domain.SortDir { d, ok := domain.ParseSortDir(q.values.Get(queryDir)) if !ok { q.invalid(queryDir) } return d }",
		"func trimPage[T any](items []T, limit int) ([]T, bool) {",
		"writeError(w, r, http.StatusUnprocessableEntity, codeValidationFailed, \"request body failed validation\", details...)",
		"writeError(w, r, http.StatusInternalServerError, codeInternal, http.StatusText(http.StatusInternalServerError))",
		"if err := json.MarshalWrite(w, v); err != nil {",
		"func valueOr[T any](p *T, def T) T",
		"json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxBodyBytes), v, json.RejectUnknownMembers(true))",
		"func NewRouter(deps Deps) http.Handler {",
		"func WithRouteErrors(mux *http.ServeMux) http.Handler {",
		"return WithRouteErrors(mux)",
		"Posts PostRepository",
		"NewPostHandler(deps.Posts).RegisterRoutes(mux)",
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

			want := make([]string, len(tc.want))
			for i, w := range tc.want {
				if w != "" && !strings.Contains(w, `"`) {
					w = strconv.Quote(w)
				}

				want[i] = w
			}

			if tc.want == nil {
				want = nil
			}

			require.Equal(t, want, groupImports(set, module))
		})
	}
}

func TestCheckTables(t *testing.T) {
	t.Parallel()

	id := spec.Field{Name: "id", Type: spec.TypeUUID, Primary: true}
	cases := []struct {
		name     string
		entities []spec.Entity
		wantErr  bool
	}{
		{name: "distinct", entities: []spec.Entity{{Name: "Person", Fields: []spec.Field{id}}, {Name: "Member", Fields: []spec.Field{id}}}},
		{
			name:     "same plural",
			entities: []spec.Entity{{Name: "Person", Plural: "people", Fields: []spec.Field{id}}, {Name: "Member", Plural: "people", Fields: []spec.Field{id}}},
			wantErr:  true,
		},
		{
			name:     "same table after snake case",
			entities: []spec.Entity{{Name: "BlogPost", Fields: []spec.Field{id}}, {Name: "Entry", Plural: "blog-posts", Fields: []spec.Field{id}}},
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkTables(&spec.Spec{Entities: tc.entities})
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestRenderReservedIdentifiers(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Module:  "example.com/blog",
		Entities: []spec.Entity{
			{Name: "User", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}},
			{Name: "Order", Plural: "order", Order: "group", Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "user", Type: spec.TypeReferences, Target: "User", Filter: true, Index: true},
				{Name: "group", Type: spec.TypeString, Required: true},
			}},
		},
	}

	migration := renderMigrationSrc(t, s, "Order")
	for _, want := range []string{
		`CREATE TABLE "order" (`,
		`"user" UUID REFERENCES users (id)`,
		`"group" TEXT NOT NULL`,
		`CREATE INDEX idx_order_user ON "order" ("user");`,
		`DROP TABLE "order";`,
	} {
		wantContains(t, migration, want)
	}

	repo := renderRepoSrc(t, s, "Order")
	for _, want := range []string{
		`INSERT INTO "order" (id, "user", "group") VALUES ($1, $2, $3)`,
		`where = append(where, ` + "`" + `"user" = ?` + "`" + `)`,
		`order := ` + "`" + `"group", id` + "`",
		`UPDATE "order" SET "user" = $1, "group" = $2 WHERE id = $3`,
		`DELETE FROM "order" WHERE id = $1`,
		"`db:\"user\"`",
	} {
		wantContains(t, repo, want)
	}
}

func TestRenderRepo_KeyOnlyEntityInsertsDefaults(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{Package: "app", Module: "example.com/app", Entities: []spec.Entity{{
		Name:   "Counter",
		Fields: []spec.Field{{Name: "myId", Type: spec.TypeInt64, Primary: true}},
	}}}

	got := renderRepoSrc(t, s, "Counter")
	wantContains(t, got, "`INSERT INTO counters DEFAULT VALUES RETURNING my_id`, ).Scan(&m.MyId)")

	_, create, ok := strings.Cut(got, "func (r *CounterRepository) Create(")
	require.True(t, ok)

	create, _, _ = strings.Cut(create, "func (r *CounterRepository) Get(")
	require.NotContains(t, create, "row :=")
}

func TestCheckColumns(t *testing.T) {
	t.Parallel()

	id := spec.Field{Name: "id", Type: spec.TypeUUID, Primary: true}
	cases := []struct {
		name    string
		fields  []spec.Field
		wantErr bool
	}{
		{"distinct", []spec.Field{id, {Name: "title", Type: spec.TypeString}}, false},
		{"duplicate name", []spec.Field{id, {Name: "title", Type: spec.TypeString}, {Name: "title", Type: spec.TypeText}}, true},
		{"same column", []spec.Field{id, {Name: "authorName", Type: spec.TypeString}, {Name: "author_name", Type: spec.TypeString}}, true},
		{"same go field", []spec.Field{id, {Name: "Id", Type: spec.TypeString}}, true},
		{"not an identifier", []spec.Field{id, {Name: "2fa", Type: spec.TypeBool}}, true},
		{"filter named like the cursor field", []spec.Field{id, {Name: "after", Type: spec.TypeString, Filter: true}}, true},
		{"filter named like the limit field", []spec.Field{id, {Name: "Limit", Type: spec.TypeInt32, Filter: true}}, true},
		{"non-filter named after", []spec.Field{id, {Name: "after", Type: spec.TypeString}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkColumns(&spec.Entity{Name: "Post", Fields: tc.fields})
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCheckRules(t *testing.T) {
	t.Parallel()

	author := spec.Entity{Name: "Author", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}}}
	cases := []struct {
		name    string
		field   spec.Field
		wantErr bool
	}{
		{"string length", spec.Field{Name: "title", Type: spec.TypeString, Validate: "min=1,max=200"}, false},
		{"alternatives", spec.Field{Name: "contact", Type: spec.TypeString, Validate: "omitempty,email|url"}, false},
		{"number range", spec.Field{Name: "likes", Type: spec.TypeInt32, Validate: "gte=0,lte=10"}, false},
		{"json length", spec.Field{Name: "meta", Type: spec.TypeJSON, Validate: "max=1024"}, false},
		{"reference key", spec.Field{Name: "author", Type: spec.TypeReferences, Target: "Author", Validate: "required"}, false},
		{"unknown rule", spec.Field{Name: "email", Type: spec.TypeString, Validate: "emial"}, true},
		{"unknown alternative", spec.Field{Name: "contact", Type: spec.TypeString, Validate: "email|urll"}, true},
		{"bad param after required", spec.Field{Name: "title", Type: spec.TypeString, Validate: "required,min=abc"}, true},
		{"bad param after omitempty", spec.Field{Name: "title", Type: spec.TypeString, Validate: "omitempty,max=abc"}, true},
		{"empty rule", spec.Field{Name: "title", Type: spec.TypeString, Validate: "min=1,,max=2"}, true},
		{"length on bool", spec.Field{Name: "published", Type: spec.TypeBool, Validate: "min=1"}, true},
		{"dive on string", spec.Field{Name: "title", Type: spec.TypeString, Validate: "dive"}, true},
		{"presence on decimal", spec.Field{Name: "price", Type: spec.TypeDecimal, Validate: "required"}, false},
		{"range on decimal", spec.Field{Name: "price", Type: spec.TypeDecimal, Validate: "gt=0"}, true},
		{"presence on date", spec.Field{Name: "born_on", Type: spec.TypeDate, Validate: "omitnil,required_with=Title"}, false},
		{"range on date", spec.Field{Name: "born_on", Type: spec.TypeDate, Validate: "min=2000-01-01"}, true},
		{"range on datetime", spec.Field{Name: "at", Type: spec.TypeDatetime, Validate: "omitempty,lte=1"}, true},
		{"uuid version", spec.Field{Name: "ext", Type: spec.TypeUUID, Validate: "uuid4|uuid5"}, false},
		{"length on uuid", spec.Field{Name: "ext", Type: spec.TypeUUID, Validate: "len=16"}, true},
		{"length on a uuid reference", spec.Field{Name: "author", Type: spec.TypeReferences, Target: "Author", Validate: "min=1"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := spec.Entity{Name: "Post", Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}, tc.field}}

			err := checkRules(&e, map[string]*spec.Entity{author.Name: &author, e.Name: &e})
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
		{
			name:    "uuid in code",
			keyType: spec.TypeUUID,
			want: []string{
				"INSERT INTO items (id, name) VALUES ($1, $2)`",
				"m.ID = uuid.New()",
			},
			notWant: "RETURNING",
		},
		{
			name:    "int64 identity in database",
			keyType: spec.TypeInt64,
			want: []string{
				"INSERT INTO items (name) VALUES ($1) RETURNING id`",
				"Scan(&m.ID)",
			},
			notWant: "m.ID =",
		},
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
	wantContains(t, renderHandlerSrc(t, s, "Author"), "BornOn *domain.Date")
	wantContains(t, renderRepoSrc(t, s, "Author"), "BornOn sql.Null[domain.Date]")
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

func TestRenderSort_Directions(t *testing.T) {
	t.Parallel()

	src, err := renderSort(packageData{Package: "blog"})
	require.NoError(t, err)
	requireParses(t, src)
	wantContains(t, string(src), `type SortDir string const ( SortAsc SortDir = "asc" SortDesc SortDir = "desc" )`)
	wantContains(t, string(src), `func ParseSortDir(s string) (SortDir, bool) { switch d := SortDir(s); d { case SortAsc, SortDesc: return d, true } return SortAsc, s == "" }`)
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
			goFile("a.go", "package blog\ntype A struct{}"),
			goFile("b.go", "package blog\ntype B struct{}"),
		}, false},
		{"same name in different packages", []genFile{
			goFile("post.go", "package blog\ntype PostRepository interface{}"),
			goFile("postgres/post.go", "package postgres\ntype PostRepository struct{}"),
		}, false},
		{"methods and blank identifiers are not declarations", []genFile{
			goFile("a.go", "package blog\ntype A struct{}\nfunc (A) M() {}\nvar _ = 1"),
			goFile("b.go", "package blog\ntype B struct{}\nfunc (B) M() {}\nvar _ = 2"),
		}, false},
		{"non-Go files are not parsed", []genFile{
			goFile("migrations/00001_create_dates.sql", "CREATE TABLE dates ();"),
		}, false},
		{"duplicate type in one package", []genFile{
			goFile("date.go", "package blog\ntype Date struct{}"),
			goFile("calendar_date.go", "package blog\ntype Date int"),
		}, true},
		{"duplicate func and var in one package", []genFile{
			goFile("postgres/a.go", "package postgres\nfunc NewDB() {}"),
			goFile("postgres/b.go", "package postgres\nvar NewDB = 1"),
		}, true},
		{"duplicate path", []genFile{
			goFile("date.go", "package blog\ntype Date struct{}"),
			goFile("date.go", "package blog\ntype Other struct{}"),
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
		router   bool
		wantErr  bool
	}{
		{name: "entity Date without date fields", entities: []spec.Entity{{Name: "Date", Fields: []spec.Field{id}}}},
		{
			name:     "entity Date next to a date field",
			entities: []spec.Entity{{Name: "Date", Fields: []spec.Field{id, {Name: "on", Type: spec.TypeDate}}}},
			wantErr:  true,
		},
		{name: "entity Errors", entities: []spec.Entity{{Name: "Errors", Fields: []spec.Field{id}}}, wantErr: true},
		{name: "entity Request", entities: []spec.Entity{{Name: "Request", Fields: []spec.Field{id}}}, wantErr: true},
		{name: "entity Router without router", entities: []spec.Entity{{Name: "Router", Fields: []spec.Field{id}}}},
		{name: "entity Router with router", entities: []spec.Entity{{Name: "Router", Fields: []spec.Field{id}}}, router: true, wantErr: true},
		{
			name:     "entity NewPost next to Post",
			entities: []spec.Entity{{Name: "Post", Fields: []spec.Field{id}}, {Name: "NewPost", Fields: []spec.Field{id}}},
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "app", Module: "example.com/app", Entities: tc.entities}
			files, err := renderFiles(s, entitiesByName(s), sqlDriverPgx, importDriverPgx, Options{Router: tc.router})
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
