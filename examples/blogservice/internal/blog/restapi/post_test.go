package restapi

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "example.com/blogservice/internal/blog"
)

func newPostTestHandler() (http.Handler, *fakeRepository[uuid.UUID, domain.Post, domain.PostListParams]) {
	repo := newFakeRepository[uuid.UUID, domain.Post, domain.PostListParams](func(m *domain.Post) *uuid.UUID { return &m.ID }, uuid.New)
	mux := http.NewServeMux()
	NewPostHandler(repo).RegisterRoutes(mux)

	return WithRouteErrors(mux), repo
}

func seedPost(t *testing.T, repo *fakeRepository[uuid.UUID, domain.Post, domain.PostListParams]) domain.Post {
	t.Helper()

	m := domain.Post{}
	require.NoError(t, repo.Create(t.Context(), &m))

	return m
}

func validCreatePostRequest() CreatePostRequest {
	return CreatePostRequest{
		Title: "sample",
	}
}

func validUpdatePostRequest() UpdatePostRequest {
	return UpdatePostRequest{
		Title: "sample",
	}
}

func TestPostHandler_Create(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(validCreatePostRequest())
	require.NoError(t, err)

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "valid", body: string(valid), wantStatus: http.StatusCreated},
		{name: "malformed body", body: "{", wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"unknown_field":1}`, wantStatus: http.StatusBadRequest},
		{name: "missing required fields", body: "{}", wantStatus: http.StatusUnprocessableEntity},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newPostTestHandler()

			rec := serve(t, h, http.MethodPost, "/posts", tc.body)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestPostHandler_Get(t *testing.T) {
	t.Parallel()

	h, repo := newPostTestHandler()
	seeded := seedPost(t, repo)

	cases := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "existing", id: seeded.ID.String(), wantStatus: http.StatusOK},
		{name: "missing", id: uuid.NewString(), wantStatus: http.StatusNotFound},
		{name: "invalid id", id: "not-an-id", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodGet, "/posts/"+tc.id, "")
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestPostHandler_List(t *testing.T) {
	t.Parallel()

	h, repo := newPostTestHandler()
	seedPost(t, repo)

	rec := serve(t, h, http.MethodGet, "/posts", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var page struct {
		Body struct {
			Items []jsontext.Value `json:"items"`
		} `json:"body"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Body.Items, 1)

	rec = serve(t, h, http.MethodGet, "/posts?limit=0", "")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestPostHandler_Update(t *testing.T) {
	t.Parallel()

	h, repo := newPostTestHandler()
	seeded := seedPost(t, repo)

	valid, err := json.Marshal(validUpdatePostRequest())
	require.NoError(t, err)

	cases := []struct {
		name       string
		id         string
		body       string
		wantStatus int
	}{
		{name: "existing", id: seeded.ID.String(), body: string(valid), wantStatus: http.StatusOK},
		{name: "missing", id: uuid.NewString(), body: string(valid), wantStatus: http.StatusNotFound},
		{name: "malformed body", id: seeded.ID.String(), body: "{", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodPut, "/posts/"+tc.id, tc.body)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestPostHandler_Delete(t *testing.T) {
	t.Parallel()

	h, repo := newPostTestHandler()
	seeded := seedPost(t, repo)

	cases := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "existing", id: seeded.ID.String(), wantStatus: http.StatusNoContent},
		{name: "missing", id: uuid.NewString(), wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodDelete, "/posts/"+tc.id, "")
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
