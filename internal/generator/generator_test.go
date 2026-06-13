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
		`json:"name" validate:"required,min=1" db:"name"`, // required merged ahead of validate; db tag for sqlx
		"Price decimal.Decimal",
		"InStock bool", // snake_case -> PascalCase
		`json:"in_stock" db:"in_stock"`,
		"ReleasedAt time.Time",
		"Metadata json.RawMessage",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderModel_ReferenceDerivesTargetPKType(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

// renderHandlerSrc runs handlerInfo + renderHandler for the named entity and
// returns the generated source, failing if the entity is not serveable or the
// output is not valid Go.
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

	hd, ok, err := handlerInfo(s, e, byName)
	require.NoErrorf(t, err, "handlerInfo(%s)", entity)
	require.Truef(t, ok, "entity %q unexpectedly not serveable", entity)

	src, err := renderHandler(hd)
	require.NoErrorf(t, err, "renderHandler(%s)", entity)
	requireParses(t, src)
	return string(src)
}

func TestRenderModel_DTOs(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Entities: []spec.Entity{{
			Name: "Post",
			Fields: []spec.Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "title", Type: "string", Required: true},
			},
			Options: spec.EntityOptions{Timestamps: true},
		}},
	}

	got := render(t, s, "Post")
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
	create := got[strings.Index(got, "type CreatePostRequest"):]
	require.NotContains(t, create, "CreatedAt", "DTOs should not contain timestamp fields")
}

func TestRenderHandler_RoutesAndStatusCodes(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Entities: []spec.Entity{{
			Name:   "Post",
			Plural: "posts",
			Fields: []spec.Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "title", Type: "string", Required: true},
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
		"Get(ctx context.Context, id uuid.UUID) (*Post, error)",
		"id, err := uuid.Parse(r.PathValue(\"id\"))",
		"writeJSON(w, http.StatusCreated, m)",
		"w.WriteHeader(http.StatusNoContent)",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderHandler_StringPKNeedsNoParse(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "cat",
		Entities: []spec.Entity{{
			Name:   "Tag",
			Fields: []spec.Field{{Name: "slug", Type: "string", Primary: true}},
		}},
	}

	got := renderHandlerSrc(t, s, "Tag")
	wantContains(t, got, `id := r.PathValue("id")`)
}

func TestHandlerInfo_SkipsCompositePK(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "rel",
		Entities: []spec.Entity{{
			Name: "Membership",
			Fields: []spec.Field{
				{Name: "user_id", Type: "uuid", Primary: true},
				{Name: "group_id", Type: "uuid", Primary: true},
			},
		}},
	}
	byName := map[string]*spec.Entity{"Membership": &s.Entities[0]}

	_, ok, err := handlerInfo(s, &s.Entities[0], byName)
	require.NoError(t, err)
	require.False(t, ok, "composite-primary-key entity should not be serveable")
}

// renderRepoSrc runs repoInfo + renderRepo for the named entity and returns the
// generated source, failing if the entity is not serveable or the output is not
// valid Go.
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

	rd, ok, err := repoInfo(s, e, byName)
	require.NoErrorf(t, err, "repoInfo(%s)", entity)
	require.Truef(t, ok, "entity %q unexpectedly not serveable", entity)

	src, err := renderRepo(rd)
	require.NoErrorf(t, err, "renderRepo(%s)", entity)
	requireParses(t, src)
	return string(src)
}

func TestRenderRepo_SQLAndInterfaceSatisfaction(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Entities: []spec.Entity{{
			Name:   "Post",
			Plural: "posts",
			Fields: []spec.Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "title", Type: "string", Required: true},
				{Name: "body", Type: "text"},
			},
			Options: spec.EntityOptions{Timestamps: true},
		}},
	}

	got := renderRepoSrc(t, s, "Post")
	for _, want := range []string{
		// compile-time check that the concrete type implements the interface
		"var _ PostRepository = (*PostgresPostRepository)(nil)",
		"func NewPostgresPostRepository(db *sqlx.DB) *PostgresPostRepository",
		// the primary-key type's package is imported for the Get/Delete signatures
		`"github.com/google/uuid"`,
		`"github.com/jmoiron/sqlx"`,
		// sqlx struct scanning + database/sql not-found sentinel
		"r.db.GetContext(ctx, &m, getPostSQL, id)",
		"errors.Is(err, sql.ErrNoRows)",
		// timestamps default to now() on insert and are returned into the struct
		"INSERT INTO posts (id, title, body, created_at, updated_at) VALUES ($1, $2, $3, now(), now()) RETURNING created_at, updated_at",
		"SELECT id, title, body, created_at, updated_at FROM posts WHERE id = $1",
		"ORDER BY id LIMIT $1 OFFSET $2",
		// the primary key is the trailing placeholder in the update
		"UPDATE posts SET title = $1, body = $2, updated_at = now() WHERE id = $3 RETURNING updated_at",
		"DELETE FROM posts WHERE id = $1",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderRepo_SoftDeleteFiltersAndUpdates(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "app",
		Entities: []spec.Entity{{
			Name: "Account",
			Fields: []spec.Field{
				{Name: "id", Type: "uuid", Primary: true},
				{Name: "name", Type: "string"},
			},
			Options: spec.EntityOptions{SoftDelete: true},
		}},
	}

	got := renderRepoSrc(t, s, "Account")
	for _, want := range []string{
		// reads exclude soft-deleted rows
		"WHERE id = $1 AND deleted_at IS NULL",
		"FROM accounts WHERE deleted_at IS NULL ORDER BY id",
		// delete is a soft update, not a row removal
		"UPDATE accounts SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL",
		// no timestamps -> Exec + RowsAffected for the not-found check
		"res.RowsAffected()",
		"if n == 0 {",
	} {
		wantContains(t, got, want)
	}
}

func TestRenderRepo_PrimaryKeyOnlyEntityUsesExistenceCheck(t *testing.T) {
	t.Parallel()

	// A serveable entity with only a primary key and no timestamps has nothing
	// writable: the UPDATE must not be `SET  WHERE ...` (invalid SQL). It should
	// degrade to an existence check by primary key.
	s := &spec.Spec{
		Package: "cat",
		Entities: []spec.Entity{{
			Name:   "Tag",
			Fields: []spec.Field{{Name: "id", Type: "uuid", Primary: true}},
		}},
	}

	got := renderRepoSrc(t, s, "Tag")
	for _, want := range []string{
		"updateTagSQL = `SELECT 1 FROM tags WHERE id = $1`",
		"r.db.GetContext(ctx, &exists, updateTagSQL, m.ID)",
		"return ErrNotFound",
	} {
		wantContains(t, got, want)
	}
	// must NOT emit a malformed empty SET clause
	require.NotContains(t, strings.Join(strings.Fields(got), " "), "SET WHERE",
		"primary-key-only entity must not generate an empty UPDATE SET clause")
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
		{name: "explicit pgx", driver: "pgx", wantName: `"pgx"`, wantImport: `_ "github.com/jackc/pgx/v5/stdlib"`},
		{name: "pq maps to postgres", driver: "pq", wantName: `"postgres"`, wantImport: `_ "github.com/lib/pq"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, imp, ok := driverInfo(tc.driver)
			require.Truef(t, ok, "driverInfo(%q) ok", tc.driver)

			src, err := renderDB(dbData{Package: "blog", DriverName: name, DriverImport: imp})
			require.NoError(t, err)
			requireParses(t, src)

			got := string(src)
			wantContains(t, got, "driverName = "+tc.wantName)
			wantContains(t, got, tc.wantImport)
			wantContains(t, got, "func NewDB(dsn string) (*sqlx.DB, error)")
			wantContains(t, got, "db, err := sqlx.Open(driverName, dsn)")
			wantContains(t, got, "db.SetMaxOpenConns(maxOpenConns)")
		})
	}
}

func TestDriverInfo_RejectsUnknown(t *testing.T) {
	t.Parallel()

	_, _, ok := driverInfo("mysql")
	require.False(t, ok, "unknown driver should be rejected")
}

func TestRenderShared_WiresEntities(t *testing.T) {
	t.Parallel()

	src, err := renderShared(sharedData{
		Package: "blog",
		Entities: []sharedEntity{
			{Struct: "Post", Repo: "PostRepository", DepsField: "Posts"},
		},
	})
	require.NoError(t, err)
	requireParses(t, src)

	got := string(src)
	for _, want := range []string{
		"var ErrNotFound = errors.New(\"not found\")",
		"Posts PostRepository",
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
