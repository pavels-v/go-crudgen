package restapi

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/blogservice/internal/blog"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type memStore[K comparable, M any] struct {
	mu     sync.Mutex
	data   map[K]M
	key    func(*M) *K
	nextID func() K
}

func newMemStore[K comparable, M any](key func(*M) *K, nextID func() K) *memStore[K, M] {
	return &memStore[K, M]{
		data:   map[K]M{},
		key:    key,
		nextID: nextID,
	}
}

func (s *memStore[K, M]) Create(_ context.Context, m *M) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nextID != nil {
		*s.key(m) = s.nextID()
	}
	k := *s.key(m)
	if _, ok := s.data[k]; ok {
		return blog.ErrAlreadyExists
	}
	s.data[k] = *m
	return nil
}

func (s *memStore[K, M]) Get(_ context.Context, id K) (*M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.data[id]
	if !ok {
		return nil, blog.ErrNotFound
	}
	return &m, nil
}

func (s *memStore[K, M]) List(_ context.Context, _, _ int) ([]M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]M, 0, len(s.data))
	for _, m := range s.data {
		out = append(out, m)
	}
	return out, nil
}

func (s *memStore[K, M]) Update(_ context.Context, m *M) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := *s.key(m)
	if _, ok := s.data[k]; !ok {
		return blog.ErrNotFound
	}
	s.data[k] = *m
	return nil
}

func (s *memStore[K, M]) Delete(_ context.Context, id K) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return blog.ErrNotFound
	}
	delete(s.data, id)
	return nil
}

func newServer() *httptest.Server {
	var commentSeq atomic.Int64
	return httptest.NewServer(NewRouter(Deps{
		Posts:    newMemStore(func(m *blog.Post) *uuid.UUID { return &m.ID }, uuid.New),
		Authors:  newMemStore(func(m *blog.Author) *uuid.UUID { return &m.ID }, uuid.New),
		Comments: newMemStore(func(m *blog.Comment) *int64 { return &m.ID }, func() int64 { return commentSeq.Add(1) }),
		Tags:     newMemStore[string](func(m *blog.Tag) *string { return &m.Slug }, nil),
	}))
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
	defer func() { require.NoError(t, resp.Body.Close()) }()

	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		require.Failf(t, "unexpected status",
			"%s %s: status = %d, want %d (body: %s)", method, path, resp.StatusCode, wantStatus, b)
	}
	if out != nil {
		require.NoError(t, json.UnmarshalRead(resp.Body, out), "decode response")
	}
}

func TestPostCRUD(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	authorID := uuid.New()

	var created blog.Post
	do(t, srv, http.MethodPost, "/posts",
		CreatePostRequest{Title: "Hello", Body: new("world"), Author: &authorID},
		&created, http.StatusCreated)
	require.NotEqual(t, uuid.Nil, created.ID)
	require.Equal(t, "Hello", created.Title)
	require.Equal(t, "world", *created.Body)
	require.False(t, created.Published)
	idPath := "/posts/" + created.ID.String()

	var got blog.Post
	do(t, srv, http.MethodGet, idPath, nil, &got, http.StatusOK)
	require.Equal(t, "Hello", got.Title)

	var list []blog.Post
	do(t, srv, http.MethodGet, "/posts", nil, &list, http.StatusOK)
	require.Len(t, list, 1)

	var updated blog.Post
	do(t, srv, http.MethodPut, idPath,
		UpdatePostRequest{Title: "Updated", Body: new("body2"), Author: &authorID},
		&updated, http.StatusOK)
	require.Equal(t, "Updated", updated.Title)
	require.Equal(t, created.ID, updated.ID)

	do(t, srv, http.MethodDelete, idPath, nil, nil, http.StatusNoContent)
	do(t, srv, http.MethodGet, idPath, nil, nil, http.StatusNotFound)
}

func TestPostCreateValidationFails(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	// Title is required; omitting it must fail validation with 400.
	do(t, srv, http.MethodPost, "/posts",
		CreatePostRequest{Author: new(uuid.New())},
		nil, http.StatusBadRequest)
}

func TestGetInvalidIDIsBadRequest(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	do(t, srv, http.MethodGet, "/posts/not-a-uuid", nil, nil, http.StatusBadRequest)
}

func TestCreateRejectsMalformedBody(t *testing.T) {
	t.Parallel()

	srv := newServer()
	t.Cleanup(srv.Close)

	valid := `{"title":"Hello"}`
	cases := []struct {
		name string
		body string
	}{
		{"trailing object", valid + `{}`},
		{"trailing garbage", valid + `x`},
		{"second value", valid + valid},
		{"duplicate member", `{"title":"Hello","title":"Bye"}`},
		{"member name case mismatch", `{"Title":"Hello"}`},
		{"unknown member", `{"title":"Hello","extra":1}`},
		{"invalid unicode", `{"title":"\ud800"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp, err := http.Post(srv.URL+"/posts", contentTypeJSON, strings.NewReader(tc.body))
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestPostCreateAppliesDefault(t *testing.T) {
	t.Parallel()

	srv := newServer()
	t.Cleanup(srv.Close)

	cases := []struct {
		name      string
		published *bool
		want      bool
	}{
		{"omitted takes default", nil, false},
		{"explicit value kept", new(true), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var created blog.Post
			do(t, srv, http.MethodPost, "/posts",
				CreatePostRequest{Title: "Hello", Published: tc.published},
				&created, http.StatusCreated)
			require.Equal(t, tc.want, created.Published)
		})
	}
}

func TestCommentDefaults(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	before := time.Now()
	var created blog.Comment
	do(t, srv, http.MethodPost, "/comments",
		CreateCommentRequest{Post: uuid.New(), Body: "Nice"},
		&created, http.StatusCreated)
	require.NotZero(t, created.ID)
	require.Zero(t, created.Likes)
	require.False(t, created.PostedAt.Before(before), "posted_at defaults to now")

	var got blog.Comment
	do(t, srv, http.MethodGet, "/comments/"+strconv.FormatInt(created.ID, 10), nil, &got, http.StatusOK)
	require.Equal(t, created.Body, got.Body)
}

func TestCommentRequiresPost(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	do(t, srv, http.MethodPost, "/comments", CreateCommentRequest{Body: "Orphan"}, nil, http.StatusBadRequest)
}

func TestTagClientKey(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	const slug = "go"

	var created blog.Tag
	do(t, srv, http.MethodPost, "/tags", CreateTagRequest{Slug: slug, Label: "Go"}, &created, http.StatusCreated)
	require.Equal(t, blog.Tag{Slug: slug, Label: "Go", Color: "gray", Weight: 1}, created)

	do(t, srv, http.MethodPost, "/tags", CreateTagRequest{Slug: slug, Label: "Again"}, nil, http.StatusConflict)
	do(t, srv, http.MethodPost, "/tags", CreateTagRequest{Label: "No slug"}, nil, http.StatusBadRequest)

	var updated blog.Tag
	do(t, srv, http.MethodPut, "/tags/"+slug, UpdateTagRequest{Label: "Golang", Color: new("blue")}, &updated, http.StatusOK)
	require.Equal(t, blog.Tag{Slug: slug, Label: "Golang", Color: "blue", Weight: 1}, updated)
}

func TestAuthorDateWireFormat(t *testing.T) {
	t.Parallel()

	srv := newServer()
	t.Cleanup(srv.Close)

	const bornOn = "1815-12-10"

	cases := []struct {
		name       string
		bornOn     string
		wantStatus int
	}{
		{"date only accepted", bornOn, http.StatusCreated},
		{"timestamp rejected", bornOn + "T00:00:00Z", http.StatusBadRequest},
		{"invalid date rejected", "1815-13-10", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := `{"email":"ada@example.com","born_on":"` + tc.bornOn + `"}`
			resp, err := http.Post(srv.URL+"/authors", contentTypeJSON, strings.NewReader(body))
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, tc.wantStatus, resp.StatusCode)
			if tc.wantStatus != http.StatusCreated {
				return
			}

			var got map[string]any
			require.NoError(t, json.UnmarshalRead(resp.Body, &got))
			require.Equal(t, bornOn, got["born_on"])
		})
	}
}
