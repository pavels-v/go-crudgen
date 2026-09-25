package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	blog "example.com/blog"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegration drives the generated service against a real PostgreSQL instance
// started with testcontainers: it applies the generated goose migrations, wires
// the generated Postgres repositories into the router, and exercises the HTTP API
// end to end. A Docker daemon is required.
func TestIntegration(t *testing.T) {
	dsn := startPostgres(t)

	db, err := blog.NewDB(dsn)
	require.NoError(t, err, "NewDB")
	t.Cleanup(func() { _ = db.Close() })

	// Apply the generated migrations through goose itself, proving the generated
	// goose files parse and the DDL is valid Postgres.
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(db.DB, "../migrations"), "goose up")

	srv := httptest.NewServer(blog.NewRouter(blog.Deps{
		Posts:   blog.NewPostgresPostRepository(db),
		Authors: blog.NewPostgresAuthorRepository(db),
	}))
	t.Cleanup(srv.Close)

	// Subtests share one container and run in order (no t.Parallel): the down
	// migration must come last because it drops the tables the others rely on.

	t.Run("CRUD round-trips through Postgres", func(t *testing.T) {
		id, authorID := uuid.New(), uuid.New()
		idPath := "/posts/" + id.String()

		// The post's author column has a foreign key to authors(id), so the author
		// must exist before a post can reference it.
		do(t, srv, http.MethodPost, "/authors",
			blog.CreateAuthorRequest{ID: authorID, Email: authorID.String() + "@example.com", Name: new("Ada")},
			nil, http.StatusCreated)

		var created blog.Post
		do(t, srv, http.MethodPost, "/posts",
			blog.CreatePostRequest{ID: id, Title: "Hello", Body: new("world"), Author: &authorID},
			&created, http.StatusCreated)
		require.Equal(t, id, created.ID)
		require.Equal(t, "world", *created.Body)
		require.False(t, created.CreatedAt.IsZero(), "created_at is populated by the DB")

		var got blog.Post
		do(t, srv, http.MethodGet, idPath, nil, &got, http.StatusOK)
		require.Equal(t, "Hello", got.Title)
		require.Equal(t, "world", *got.Body)
		require.Equal(t, authorID, *got.Author)

		var updated blog.Post
		do(t, srv, http.MethodPut, idPath,
			blog.UpdatePostRequest{Title: "Updated", Body: new("body2"), Author: &authorID},
			&updated, http.StatusOK)
		require.Equal(t, "Updated", updated.Title)
		require.Equal(t, "body2", *updated.Body)

		do(t, srv, http.MethodDelete, idPath, nil, nil, http.StatusNoContent)
		do(t, srv, http.MethodGet, idPath, nil, nil, http.StatusNotFound)
	})

	t.Run("nullable fields round-trip as NULL", func(t *testing.T) {
		id := uuid.New()

		// Body and Author are omitted, so they must be stored and returned as NULL
		// (nil pointers) - the sql.Null[T] round-trip on the repository row.
		var created blog.Post
		do(t, srv, http.MethodPost, "/posts",
			blog.CreatePostRequest{ID: id, Title: "No body"},
			&created, http.StatusCreated)
		require.Nil(t, created.Body)
		require.Nil(t, created.Author)

		var got blog.Post
		do(t, srv, http.MethodGet, "/posts/"+id.String(), nil, &got, http.StatusOK)
		require.Equal(t, "No body", got.Title)
		require.Nil(t, got.Body, "a NULL column must decode back to a nil pointer")
		require.Nil(t, got.Author)
	})

	t.Run("list returns persisted posts", func(t *testing.T) {
		var list []blog.Post
		do(t, srv, http.MethodGet, "/posts?limit=10", nil, &list, http.StatusOK)
		require.GreaterOrEqual(t, len(list), 1)
	})

	t.Run("down migrations drop the tables", func(t *testing.T) {
		require.NoError(t, goose.Reset(db.DB, "../migrations"), "goose reset (down)")
		_, err := db.Exec("SELECT 1 FROM posts")
		require.Error(t, err, "posts table should not exist after the down migrations")
	})
}

// startPostgres launches a throwaway PostgreSQL container and returns a DSN for
// it, registering termination as test cleanup.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("blog"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "start postgres container")
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(ctr)) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "connection string")
	return dsn
}

// do issues a request, asserts the status code, and decodes the response into
// out when out is non-nil.
func do(t *testing.T, srv *httptest.Server, method, path string, body, out any, wantStatus int) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err, "marshal body")
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	require.NoError(t, err, "new request")

	resp, err := http.DefaultClient.Do(req)
	require.NoErrorf(t, err, "%s %s", method, path)
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		require.Failf(t, "unexpected status",
			"%s %s: status = %d, want %d (body: %s)", method, path, resp.StatusCode, wantStatus, b)
	}
	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out), "decode response")
	}
}
