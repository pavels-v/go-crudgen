package restapi

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "example.com/blogservice/internal/blog"
)

func newCommentTestHandler() (http.Handler, *fakeRepository[int64, domain.Comment, domain.CommentListParams]) {
	var seq int64

	repo := newFakeRepository[int64, domain.Comment, domain.CommentListParams](func(m *domain.Comment) *int64 { return &m.ID }, func() int64 {
		seq++

		return seq
	})
	mux := http.NewServeMux()
	NewCommentHandler(repo).RegisterRoutes(mux)

	return WithRouteErrors(mux), repo
}

func seedComment(t *testing.T, repo *fakeRepository[int64, domain.Comment, domain.CommentListParams]) domain.Comment {
	t.Helper()

	m := domain.Comment{}
	require.NoError(t, repo.Create(t.Context(), &m))

	return m
}

func validCreateCommentRequest() CreateCommentRequest {
	return CreateCommentRequest{
		Post: new(uuid.New()),
		Body: "sample",
	}
}

func validUpdateCommentRequest() UpdateCommentRequest {
	return UpdateCommentRequest{
		Post: new(uuid.New()),
		Body: "sample",
	}
}

func TestCommentHandler_Create(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(validCreateCommentRequest())
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

			h, _ := newCommentTestHandler()

			rec := serve(t, h, http.MethodPost, "/comments", tc.body)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestCommentHandler_Get(t *testing.T) {
	t.Parallel()

	h, repo := newCommentTestHandler()
	seeded := seedComment(t, repo)

	cases := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "existing", id: strconv.FormatInt(seeded.ID, 10), wantStatus: http.StatusOK},
		{name: "missing", id: strconv.FormatInt(seeded.ID+1, 10), wantStatus: http.StatusNotFound},
		{name: "invalid id", id: "not-an-id", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodGet, "/comments/"+tc.id, "")
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestCommentHandler_List(t *testing.T) {
	t.Parallel()

	h, repo := newCommentTestHandler()
	seedComment(t, repo)

	rec := serve(t, h, http.MethodGet, "/comments", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var page struct {
		Body struct {
			Items []jsontext.Value `json:"items"`
		} `json:"body"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Body.Items, 1)

	rec = serve(t, h, http.MethodGet, "/comments?limit=0", "")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestCommentHandler_Update(t *testing.T) {
	t.Parallel()

	h, repo := newCommentTestHandler()
	seeded := seedComment(t, repo)

	valid, err := json.Marshal(validUpdateCommentRequest())
	require.NoError(t, err)

	cases := []struct {
		name       string
		id         string
		body       string
		wantStatus int
	}{
		{name: "existing", id: strconv.FormatInt(seeded.ID, 10), body: string(valid), wantStatus: http.StatusOK},
		{name: "missing", id: strconv.FormatInt(seeded.ID+1, 10), body: string(valid), wantStatus: http.StatusNotFound},
		{name: "malformed body", id: strconv.FormatInt(seeded.ID, 10), body: "{", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodPut, "/comments/"+tc.id, tc.body)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

func TestCommentHandler_Delete(t *testing.T) {
	t.Parallel()

	h, repo := newCommentTestHandler()
	seeded := seedComment(t, repo)

	cases := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "existing", id: strconv.FormatInt(seeded.ID, 10), wantStatus: http.StatusNoContent},
		{name: "missing", id: strconv.FormatInt(seeded.ID+1, 10), wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serve(t, h, http.MethodDelete, "/comments/"+tc.id, "")
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
