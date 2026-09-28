package restapi

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	domain "example.com/blogservice/internal/blog"
)

func newTagTestHandler() (http.Handler, *fakeRepository[string, domain.Tag, domain.TagListParams]) {
	repo := newFakeRepository[string, domain.Tag, domain.TagListParams](func(m *domain.Tag) *string { return &m.Slug }, nil)
	mux := http.NewServeMux()
	NewTagHandler(repo).RegisterRoutes(mux)

	return WithRouteErrors(mux), repo
}

func seedTag(t *testing.T, repo *fakeRepository[string, domain.Tag, domain.TagListParams]) domain.Tag {
	t.Helper()

	m := domain.Tag{Slug: "sample"}
	require.NoError(t, repo.Create(t.Context(), &m))

	return m
}

func validCreateTagRequest() CreateTagRequest {
	return CreateTagRequest{
		Slug:  "sample",
		Label: "sample",
	}
}

func validUpdateTagRequest() UpdateTagRequest {
	return UpdateTagRequest{
		Label: "sample",
	}
}

func TestTagHandler_Create(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(validCreateTagRequest())
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

			h, _ := newTagTestHandler()

			rec := serve(t, h, http.MethodPost, "/tags", tc.body)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestTagHandler_Get(t *testing.T) {
	t.Parallel()

	h, repo := newTagTestHandler()
	seeded := seedTag(t, repo)

	cases := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "existing", id: seeded.Slug, wantStatus: http.StatusOK},
		{name: "missing", id: "missing", wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodGet, "/tags/"+tc.id, "")
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestTagHandler_List(t *testing.T) {
	t.Parallel()

	h, repo := newTagTestHandler()
	seedTag(t, repo)

	rec := serve(t, h, http.MethodGet, "/tags", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var page struct {
		Body struct {
			Items []jsontext.Value `json:"items"`
		} `json:"body"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Body.Items, 1)

	rec = serve(t, h, http.MethodGet, "/tags?limit=0", "")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestTagHandler_Update(t *testing.T) {
	t.Parallel()

	h, repo := newTagTestHandler()
	seeded := seedTag(t, repo)

	valid, err := json.Marshal(validUpdateTagRequest())
	require.NoError(t, err)

	cases := []struct {
		name       string
		id         string
		body       string
		wantStatus int
	}{
		{name: "existing", id: seeded.Slug, body: string(valid), wantStatus: http.StatusOK},
		{name: "missing", id: "missing", body: string(valid), wantStatus: http.StatusNotFound},
		{name: "malformed body", id: seeded.Slug, body: "{", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodPut, "/tags/"+tc.id, tc.body)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestTagHandler_Delete(t *testing.T) {
	t.Parallel()

	h, repo := newTagTestHandler()
	seeded := seedTag(t, repo)

	cases := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "existing", id: seeded.Slug, wantStatus: http.StatusNoContent},
		{name: "missing", id: "missing", wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodDelete, "/tags/"+tc.id, "")
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
