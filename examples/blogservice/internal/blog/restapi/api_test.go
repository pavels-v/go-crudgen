package restapi

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
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

type memStore[K comparable, M, P any] struct {
	mu     sync.Mutex
	data   map[K]M
	key    func(*M) *K
	nextID func() K
	limit  func(P) int
	listed []P
}

func newMemStore[K comparable, M, P any](key func(*M) *K, nextID func() K) *memStore[K, M, P] {
	return &memStore[K, M, P]{
		data:   map[K]M{},
		key:    key,
		nextID: nextID,
	}
}

func (s *memStore[K, M, P]) Create(_ context.Context, m *M) error {
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

func (s *memStore[K, M, P]) Get(_ context.Context, id K) (*M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.data[id]
	if !ok {
		return nil, blog.ErrNotFound
	}
	return &m, nil
}

func (s *memStore[K, M, P]) List(_ context.Context, p P) ([]M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listed = append(s.listed, p)
	out := make([]M, 0, len(s.data))
	for _, m := range s.data {
		out = append(out, m)
	}
	if s.limit != nil {
		out = out[:min(len(out), s.limit(p))]
	}
	return out, nil
}

func (s *memStore[K, M, P]) Update(_ context.Context, m *M) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := *s.key(m)
	if _, ok := s.data[k]; !ok {
		return blog.ErrNotFound
	}
	s.data[k] = *m
	return nil
}

func (s *memStore[K, M, P]) Delete(_ context.Context, id K) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return blog.ErrNotFound
	}
	delete(s.data, id)
	return nil
}

func newServer() *httptest.Server {
	return newServerWithPosts(newPostStore())
}

func newAuthorStore() *memStore[uuid.UUID, blog.Author, blog.AuthorListParams] {
	s := newMemStore[uuid.UUID, blog.Author, blog.AuthorListParams](func(m *blog.Author) *uuid.UUID { return &m.ID }, uuid.New)
	s.limit = func(p blog.AuthorListParams) int { return p.Limit }
	return s
}

func newPostStore() *memStore[uuid.UUID, blog.Post, blog.PostListParams] {
	s := newMemStore[uuid.UUID, blog.Post, blog.PostListParams](func(m *blog.Post) *uuid.UUID { return &m.ID }, uuid.New)
	s.limit = func(p blog.PostListParams) int { return p.Limit }
	return s
}

func newServerWithPosts(posts blog.PostRepository) *httptest.Server {
	var commentSeq atomic.Int64
	return httptest.NewServer(NewRouter(Deps{
		Posts:    posts,
		Authors:  newAuthorStore(),
		Comments: newMemStore[int64, blog.Comment, blog.CommentListParams](func(m *blog.Comment) *int64 { return &m.ID }, func() int64 { return commentSeq.Add(1) }),
		Tags:     newMemStore[string, blog.Tag, blog.TagListParams](func(m *blog.Tag) *string { return &m.Slug }, nil),
	}))
}

// do issues a request, asserts the status code, and decodes the envelope body
// into out when out is non-nil.
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
		var env bodyResponse[jsontext.Value]
		require.NoError(t, json.UnmarshalRead(resp.Body, &env), "decode response")
		require.NoError(t, json.Unmarshal(env.Body, out), "decode body")
	}
}

// doError sends a raw body, asserts the status code and returns the decoded
// error envelope after checking it carries exactly the error object.
func doError(t *testing.T, srv *httptest.Server, method, path, body string, wantStatus int) apiError {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err, "new request")

	resp, err := http.DefaultClient.Do(req)
	require.NoErrorf(t, err, "%s %s", method, path)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, wantStatus, resp.StatusCode)
	require.Equal(t, contentTypeJSON, resp.Header.Get("Content-Type")) //nolint:testifylint // compares a header, not JSON

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var shape map[string]map[string]jsontext.Value
	require.NoError(t, json.Unmarshal(raw, &shape), "decode envelope")
	require.Len(t, shape, 1, "envelope carries only error")
	require.ElementsMatch(t, []string{"code", "message", "details"}, keys(shape["error"]))

	var env errorResponse
	require.NoError(t, json.Unmarshal(raw, &env))
	return env.Error
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
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

	var list cursorPage[blog.Post]
	do(t, srv, http.MethodGet, "/posts", nil, &list, http.StatusOK)
	require.Len(t, list.Items, 1)
	require.Empty(t, list.NextCursor)

	var updated blog.Post
	do(t, srv, http.MethodPut, idPath,
		UpdatePostRequest{Title: "Updated", Body: new("body2"), Author: &authorID},
		&updated, http.StatusOK)
	require.Equal(t, "Updated", updated.Title)
	require.Equal(t, created.ID, updated.ID)

	do(t, srv, http.MethodDelete, idPath, nil, nil, http.StatusNoContent)
	do(t, srv, http.MethodGet, idPath, nil, nil, http.StatusNotFound)
}

func TestErrorResponses(t *testing.T) {
	t.Parallel()

	srv := newServer()
	t.Cleanup(srv.Close)

	const (
		tagPath      = "/tags"
		unknownParam = "editor"
	)
	do(t, srv, http.MethodPost, tagPath, CreateTagRequest{Slug: "taken", Label: "Taken"}, nil, http.StatusCreated)

	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		wantStatus  int
		wantCode    string
		wantDetails []errorDetail
	}{
		{
			"missing required field", http.MethodPost, "/posts", `{}`,
			http.StatusUnprocessableEntity, codeValidationFailed,
			[]errorDetail{{Field: "/title", Reason: "required"}},
		},
		{
			"rule with parameter", http.MethodPost, "/posts", `{"title":"` + strings.Repeat("x", 201) + `"}`,
			http.StatusUnprocessableEntity, codeValidationFailed,
			[]errorDetail{{Field: "/title", Reason: "max=200"}},
		},
		{
			"rule on an optional field", http.MethodPost, "/authors", `{"email":"long@example.com","name":"` + strings.Repeat("x", 101) + `"}`,
			http.StatusUnprocessableEntity, codeValidationFailed,
			[]errorDetail{{Field: "/name", Reason: "max=100"}},
		},
		{
			"missing reference key", http.MethodPost, "/comments", `{"body":"Orphan"}`,
			http.StatusUnprocessableEntity, codeValidationFailed,
			[]errorDetail{{Field: "/post", Reason: "required"}},
		},
		{
			"unknown member", http.MethodPost, "/posts", `{"title":"Hello","extra":1}`,
			http.StatusBadRequest, codeMalformedBody,
			[]errorDetail{{Field: "/extra", Reason: reasonUnknownField}},
		},
		{
			"duplicate member", http.MethodPost, "/posts", `{"title":"Hello","title":"Bye"}`,
			http.StatusBadRequest, codeMalformedBody,
			[]errorDetail{{Field: "/title", Reason: reasonDuplicateField}},
		},
		{
			"wrong value type", http.MethodPost, "/posts", `{"title":1}`,
			http.StatusBadRequest, codeMalformedBody,
			[]errorDetail{{Field: "/title", Reason: reasonInvalidValue}},
		},
		{
			"syntax error", http.MethodPost, "/posts", `{"title":`,
			http.StatusBadRequest, codeMalformedBody,
			[]errorDetail{{Field: "/title", Reason: reasonSyntax}},
		},
		{
			"body too large", http.MethodPost, "/posts", `{"title":"` + strings.Repeat("x", maxBodyBytes) + `"}`,
			http.StatusRequestEntityTooLarge, codeBodyTooLarge, nil,
		},
		{
			"invalid id", http.MethodGet, "/posts/not-a-uuid", "",
			http.StatusBadRequest, codeInvalidID, nil,
		},
		{
			"invalid page", http.MethodGet, "/authors?limit=abc&offset=-1", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{{Field: queryLimit, Reason: reasonInvalidValue}, {Field: queryOffset, Reason: reasonInvalidValue}},
		},
		{
			"unknown query parameter", http.MethodGet, "/posts?" + unknownParam + "=x&limit=5", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{{Field: unknownParam, Reason: reasonUnknownField}},
		},
		{
			"offset on a cursor-paged entity", http.MethodGet, "/posts?offset=10", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{{Field: queryOffset, Reason: reasonUnknownField}},
		},
		{
			"malformed cursor", http.MethodGet, "/posts?cursor=not-a-cursor", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{{Field: queryCursor, Reason: reasonInvalidValue}},
		},
		{
			"repeated query parameter", http.MethodGet, "/posts?published=true&published=false", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{{Field: queryPostPublished, Reason: reasonDuplicateField}},
		},
		{
			"invalid filter and direction", http.MethodGet, "/posts?author=nope&published=maybe&dir=up", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{
				{Field: queryPostPublished, Reason: reasonInvalidValue},
				{Field: queryPostAuthor, Reason: reasonInvalidValue},
				{Field: queryDir, Reason: reasonInvalidValue},
			},
		},
		{
			"filter on entity without filters", http.MethodGet, "/tags?label=go", "",
			http.StatusBadRequest, codeInvalidQuery,
			[]errorDetail{{Field: "label", Reason: reasonUnknownField}},
		},
		{
			"entity not found", http.MethodGet, "/posts/" + uuid.NewString(), "",
			http.StatusNotFound, codeNotFound, nil,
		},
		{
			"unknown route", http.MethodGet, "/nope", "",
			http.StatusNotFound, codeNotFound, nil,
		},
		{
			"method not allowed", http.MethodPatch, "/posts", "",
			http.StatusMethodNotAllowed, codeMethodNotAllowed, nil,
		},
		{
			"duplicate key", http.MethodPost, tagPath, `{"slug":"taken","label":"Again"}`,
			http.StatusConflict, codeAlreadyExists, nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := doError(t, srv, tc.method, tc.path, tc.body, tc.wantStatus)
			require.Equal(t, tc.wantCode, got.Code)
			require.NotEmpty(t, got.Message)
			if tc.wantDetails == nil {
				require.Empty(t, got.Details)
				return
			}
			require.Equal(t, tc.wantDetails, got.Details)
		})
	}
}

func TestListParsesFiltersAndDirection(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	cases := []struct {
		name  string
		query string
		want  blog.PostListParams
	}{
		{"defaults", "", blog.PostListParams{Dir: blog.SortAsc, Limit: defaultLimit + 1}},
		{
			"filters, direction and limit", "?author=" + author.String() + "&published=true&dir=desc&limit=5",
			blog.PostListParams{Author: &author, Published: new(true), Dir: blog.SortDesc, Limit: 6},
		},
		{"explicit ascending", "?dir=asc", blog.PostListParams{Dir: blog.SortAsc, Limit: defaultLimit + 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			posts := newPostStore()
			srv := newServerWithPosts(posts)
			t.Cleanup(srv.Close)

			do(t, srv, http.MethodGet, "/posts"+tc.query, nil, nil, http.StatusOK)
			require.Equal(t, []blog.PostListParams{tc.want}, posts.listed)
		})
	}
}

func TestOffsetListReportsHasMore(t *testing.T) {
	t.Parallel()

	const authors = 3
	cases := []struct {
		name      string
		limit     int
		wantLimit int
		wantLen   int
		wantMore  bool
	}{
		{"more rows than limit", authors - 1, authors - 1, authors - 1, true},
		{"exactly limit", authors, authors, authors, false},
		{"fewer rows than limit", authors + 1, authors + 1, authors, false},
		{"limit is clamped", maxLimit + 1, maxLimit, authors, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newServer()
			t.Cleanup(srv.Close)
			for i := range authors {
				req := CreateAuthorRequest{Email: "author" + strconv.Itoa(i) + "@example.com"}
				do(t, srv, http.MethodPost, "/authors", req, nil, http.StatusCreated)
			}

			var list offsetPage[blog.Author]
			do(t, srv, http.MethodGet, "/authors?limit="+strconv.Itoa(tc.limit)+"&offset=0", nil, &list, http.StatusOK)
			require.Len(t, list.Items, tc.wantLen)
			require.Equal(t, tc.wantLimit, list.Limit)
			require.Zero(t, list.Offset)
			require.Equal(t, tc.wantMore, list.HasMore)
		})
	}
}

func TestCursorListRoundTripsNextCursor(t *testing.T) {
	t.Parallel()

	posts := newPostStore()
	srv := newServerWithPosts(posts)
	t.Cleanup(srv.Close)
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		do(t, srv, http.MethodPost, "/posts", CreatePostRequest{Title: title}, nil, http.StatusCreated)
	}

	const firstPage = "/posts?dir=desc&limit=2"
	var first cursorPage[blog.Post]
	do(t, srv, http.MethodGet, firstPage, nil, &first, http.StatusOK)
	require.Len(t, first.Items, 2)
	require.NotEmpty(t, first.NextCursor)

	last := first.Items[1]
	var second cursorPage[blog.Post]
	do(t, srv, http.MethodGet, firstPage+"&cursor="+first.NextCursor, nil, &second, http.StatusOK)
	require.Equal(t, &blog.PostCursor{CreatedAt: last.CreatedAt, ID: last.ID}, posts.listed[1].After)

	got := doError(t, srv, http.MethodGet, "/posts?dir=asc&cursor="+first.NextCursor, "", http.StatusBadRequest)
	require.Equal(t, []errorDetail{{Field: queryCursor, Reason: reasonInvalidValue}}, got.Details, "a cursor keeps its direction")
}

func TestMethodNotAllowedListsAllowedMethods(t *testing.T) {
	t.Parallel()

	srv := newServer()
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/posts", http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Allow"), http.MethodPost)
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

			got := doError(t, srv, http.MethodPost, "/posts", tc.body, http.StatusBadRequest)
			require.Equal(t, codeMalformedBody, got.Code)
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

func TestTagClientKey(t *testing.T) {
	t.Parallel()

	srv := newServer()
	defer srv.Close()

	const slug = "go"

	var created blog.Tag
	do(t, srv, http.MethodPost, "/tags", CreateTagRequest{Slug: slug, Label: "Go"}, &created, http.StatusCreated)
	require.Equal(t, blog.Tag{Slug: slug, Label: "Go", Color: "gray", Weight: 1}, created)

	do(t, srv, http.MethodPost, "/tags", CreateTagRequest{Slug: slug, Label: "Again"}, nil, http.StatusConflict)
	do(t, srv, http.MethodPost, "/tags", CreateTagRequest{Label: "No slug"}, nil, http.StatusUnprocessableEntity)

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
			if tc.wantStatus != http.StatusCreated {
				got := doError(t, srv, http.MethodPost, "/authors", body, tc.wantStatus)
				require.Equal(t, []errorDetail{{Field: "/born_on", Reason: reasonInvalidValue}}, got.Details)
				return
			}

			resp, err := http.Post(srv.URL+"/authors", contentTypeJSON, strings.NewReader(body))
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, tc.wantStatus, resp.StatusCode)

			var got bodyResponse[map[string]any]
			require.NoError(t, json.UnmarshalRead(resp.Body, &got))
			require.Equal(t, bornOn, got.Body["born_on"])
		})
	}
}
