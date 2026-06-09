package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// postStore and authorStore are minimal in-memory repositories used to exercise
// the generated handlers end to end. They are test fixtures, not generated code;
// the real implementations arrive with the storage milestone.
type postStore struct {
	mu   sync.Mutex
	data map[uuid.UUID]Post
}

func newPostStore() *postStore { return &postStore{data: map[uuid.UUID]Post{}} }

func (s *postStore) Create(_ context.Context, m *Post) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[m.ID] = *m
	return nil
}

func (s *postStore) Get(_ context.Context, id uuid.UUID) (*Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &m, nil
}

func (s *postStore) List(_ context.Context, limit, offset int) ([]Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Post, 0, len(s.data))
	for _, m := range s.data {
		out = append(out, m)
	}
	return out, nil
}

func (s *postStore) Update(_ context.Context, m *Post) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[m.ID]; !ok {
		return ErrNotFound
	}
	s.data[m.ID] = *m
	return nil
}

func (s *postStore) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

// authorStore exists so NewRouter has a repository for every entity; the tests
// below drive the Post endpoints.
type authorStore struct {
	mu   sync.Mutex
	data map[uuid.UUID]Author
}

func newAuthorStore() *authorStore { return &authorStore{data: map[uuid.UUID]Author{}} }

func (s *authorStore) Create(_ context.Context, m *Author) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[m.ID] = *m
	return nil
}

func (s *authorStore) Get(_ context.Context, id uuid.UUID) (*Author, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &m, nil
}

func (s *authorStore) List(_ context.Context, limit, offset int) ([]Author, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Author, 0, len(s.data))
	for _, m := range s.data {
		out = append(out, m)
	}
	return out, nil
}

func (s *authorStore) Update(_ context.Context, m *Author) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[m.ID]; !ok {
		return ErrNotFound
	}
	s.data[m.ID] = *m
	return nil
}

func (s *authorStore) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

func newServer() *httptest.Server {
	return httptest.NewServer(NewRouter(Deps{Posts: newPostStore(), Authors: newAuthorStore()}))
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

func TestPostCRUD(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	id, authorID := uuid.New(), uuid.New()
	idPath := "/posts/" + id.String()

	var created Post
	do(t, srv, http.MethodPost, "/posts",
		CreatePostRequest{ID: id, Title: "Hello", Body: "world", Author: authorID},
		&created, http.StatusCreated)
	require.Equal(t, id, created.ID)
	require.Equal(t, "Hello", created.Title)

	var got Post
	do(t, srv, http.MethodGet, idPath, nil, &got, http.StatusOK)
	require.Equal(t, "Hello", got.Title)

	var list []Post
	do(t, srv, http.MethodGet, "/posts", nil, &list, http.StatusOK)
	require.Len(t, list, 1)

	var updated Post
	do(t, srv, http.MethodPut, idPath,
		UpdatePostRequest{Title: "Updated", Body: "body2", Author: authorID},
		&updated, http.StatusOK)
	require.Equal(t, "Updated", updated.Title)
	require.Equal(t, id, updated.ID)

	do(t, srv, http.MethodDelete, idPath, nil, nil, http.StatusNoContent)
	do(t, srv, http.MethodGet, idPath, nil, nil, http.StatusNotFound)
}

func TestPostCreateValidationFails(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	// Title is required; omitting it must fail validation with 400.
	do(t, srv, http.MethodPost, "/posts",
		CreatePostRequest{ID: uuid.New(), Author: uuid.New()},
		nil, http.StatusBadRequest)
}

func TestGetInvalidIDIsBadRequest(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	do(t, srv, http.MethodGet, "/posts/not-a-uuid", nil, nil, http.StatusBadRequest)
}
