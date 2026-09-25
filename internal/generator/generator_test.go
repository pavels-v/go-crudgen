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
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "name", Type: spec.TypeString, Required: true, Validate: "min=1"},
				{Name: "price", Type: spec.TypeDecimal},
				{Name: "in_stock", Type: spec.TypeBool},
				{Name: "released_at", Type: spec.TypeDatetime},
				{Name: "metadata", Type: spec.TypeJSON},
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
		"ID uuid.UUID", // primary key -> non-null value type
		`json:"id"`,
		"Name string",                           // required -> non-null value type
		`json:"name" validate:"required,min=1"`, // required merged ahead of validate; no db tag on the API model
		"Price *decimal.Decimal",                // nullable -> pointer
		"InStock *bool",                         // snake_case -> PascalCase, nullable -> pointer
		`json:"in_stock,omitempty"`,
		"ReleasedAt *time.Time",
		"Metadata *json.RawMessage",
	} {
		wantContains(t, got, want)
	}
	// The API model carries no db tags; column mapping lives on the repository row.
	require.NotContains(t, got, `db:"`, "API model should not carry db struct tags")
}

func TestRenderModel_ReferenceDerivesTargetPKType(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
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
	wantContains(t, got, `json:"author,omitempty"`)
}

func TestRenderModel_OptionsTimestampsAndSoftDelete(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "app",
		Entities: []spec.Entity{{
			Name:    "Session",
			Fields:  []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}},
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

func TestRenderModel_DTOs(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
		Entities: []spec.Entity{{
			Name: "Post",
			Fields: []spec.Field{
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "title", Type: spec.TypeString, Required: true},
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
	_, create, ok := strings.Cut(got, "type CreatePostRequest")
	require.True(t, ok, "CreatePostRequest should be generated")
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
		"Get(ctx context.Context, id uuid.UUID) (*Post, error)",
		"id, err := uuid.Parse(r.PathValue(pathParamID))",
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
		Entities: []spec.Entity{{
			Name:   "Widget",
			Fields: []spec.Field{{Name: "id", Type: spec.TypeInt32, Primary: true}},
		}},
	}

	got := renderHandlerSrc(t, s, "Widget")
	for _, want := range []string{
		`idRaw, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 32)`,
		"id := int32(idRaw)",
		"Get(ctx context.Context, id int32) (*Widget, error)",
	} {
		wantContains(t, got, want)
	}
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
		"var _ PostRepository = (*PostgresPostRepository)(nil)",
		"func NewPostgresPostRepository(db *sqlx.DB) *PostgresPostRepository",
		// the primary-key type's package is imported for the Get/Delete signatures
		`"github.com/google/uuid"`,
		`"github.com/jmoiron/sqlx"`,
		// the row struct carries db tags; nullable body becomes sql.Null[T]
		"type postRow struct {",
		"Body sql.Null[string]",
		"func newPostRow(m *Post) postRow",
		"Body: toNull(m.Body)",
		"Body: fromNull(row.Body)",
		// sqlx scans into the row, which is then converted to the API model
		"r.db.GetContext(ctx, &row, getPostSQL, id)",
		"m := row.toModel()",
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
				{Name: "id", Type: spec.TypeUUID, Primary: true},
				{Name: "name", Type: spec.TypeString},
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
			Fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}},
		}},
	}

	got := renderRepoSrc(t, s, "Tag")
	for _, want := range []string{
		"updateTagSQL = `SELECT 1 FROM tags WHERE id = $1`",
		"r.db.GetContext(ctx, &exists, updateTagSQL, row.ID)",
		"return ErrNotFound",
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
		{
			name:   "soft delete adds a nullable deleted_at",
			fields: []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}, {Name: "value", Type: spec.TypeString, Required: true}},
			want:   false, // overridden below for the soft-delete sub-case
		},
	}

	for _, tc := range cases[:2] {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &spec.Spec{Package: "app", Entities: []spec.Entity{{Name: "Thing", Fields: tc.fields}}}
			rd, err := repoInfo(s, &s.Entities[0], byNameOf(s))
			require.NoError(t, err)
			require.Equal(t, tc.want, rd.HasNullable)
		})
	}

	t.Run("soft delete makes deleted_at nullable", func(t *testing.T) {
		t.Parallel()

		s := &spec.Spec{Package: "app", Entities: []spec.Entity{{
			Name:    "Thing",
			Fields:  []spec.Field{{Name: "id", Type: spec.TypeUUID, Primary: true}, {Name: "value", Type: spec.TypeString, Required: true}},
			Options: spec.EntityOptions{SoftDelete: true},
		}}}
		rd, err := repoInfo(s, &s.Entities[0], byNameOf(s))
		require.NoError(t, err)
		require.True(t, rd.HasNullable, "deleted_at is nullable, so the repo needs sql.Null helpers")
		wantContains(t, mustRenderRepo(t, rd), "DeletedAt sql.Null[time.Time]")
	})
}

// mustRenderRepo renders repo data to source, failing on error or invalid Go.
func mustRenderRepo(t *testing.T, rd repoData) string {
	t.Helper()
	src, err := renderRepo(rd)
	require.NoError(t, err)
	requireParses(t, src)
	return string(src)
}

func TestRenderNulls_GenericHelpers(t *testing.T) {
	t.Parallel()

	src, err := renderNulls(nullsData{Package: "blog"})
	require.NoError(t, err)
	requireParses(t, src)

	got := string(src)
	for _, want := range []string{
		"package blog",
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

func TestRenderMigration_ColumnsConstraintsAndOptions(t *testing.T) {
	t.Parallel()

	s := &spec.Spec{
		Package: "blog",
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
			}, Options: spec.EntityOptions{Timestamps: true, SoftDelete: true}},
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
		"id UUID NOT NULL PRIMARY KEY",        // single PK declared inline
		"title TEXT NOT NULL",                 // required -> NOT NULL
		"body TEXT,",                          // optional column is nullable
		"published BOOLEAN DEFAULT FALSE",     // bool default rendered as SQL literal
		"slug TEXT UNIQUE",                    // unique modifier
		"author UUID REFERENCES authors (id)", // FK column typed from + pointing at the target PK
		"created_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT now()",
		"deleted_at TIMESTAMPTZ", // soft-delete marker is nullable
		"CREATE INDEX idx_posts_slug ON posts (slug);",
		"DROP TABLE posts;",
	} {
		wantContains(t, got, want)
	}
	// Each statement is wrapped in its own block: CREATE TABLE + CREATE INDEX (up)
	// and DROP TABLE (down) make three.
	require.Equal(t, 3, strings.Count(got, "-- +goose StatementBegin"))
	require.Equal(t, 3, strings.Count(got, "-- +goose StatementEnd"))
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

func TestGroupImports(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty", in: nil, want: nil},
		{name: "stdlib only", in: []string{importTime, importContext}, want: []string{importContext, importTime}},
		{name: "third-party only", in: []string{importUUID, importDecimal}, want: []string{importUUID, importDecimal}},
		{
			name: "mixed split by blank entry",
			in:   []string{importSQLx, importTime, importUUID, importContext},
			want: []string{importContext, importTime, "", importUUID, importSQLx},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			set := make(map[string]struct{}, len(tc.in))
			for _, imp := range tc.in {
				set[imp] = struct{}{}
			}
			require.Equal(t, tc.want, groupImports(set))
		})
	}
}
