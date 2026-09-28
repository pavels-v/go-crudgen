package restapi

import (
	"encoding/json/jsontext"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"example.com/blogservice/internal/blog"
)

const (
	queryPostPublished = "published"
	queryPostAuthor    = "author"
)

const (
	defaultPostPublished = false
	defaultPostViews     = 0
)

type postCursor struct {
	Dir   blog.SortDir    `json:"dir"`
	After blog.PostCursor `json:"after"`
}

// CreatePostRequest is the request body for creating the post entity.
type CreatePostRequest struct {
	Title     string          `json:"title" validate:"required,min=1,max=200"`
	Body      *string         `json:"body,omitzero"`
	Published *bool           `json:"published"`
	Views     *int64          `json:"views"`
	Metadata  *jsontext.Value `json:"metadata,omitzero"`
	Author    *uuid.UUID      `json:"author,omitzero"`
}

// UpdatePostRequest is the request body for replacing the post entity.
type UpdatePostRequest struct {
	Title     string          `json:"title" validate:"required,min=1,max=200"`
	Body      *string         `json:"body,omitzero"`
	Published *bool           `json:"published"`
	Views     *int64          `json:"views"`
	Metadata  *jsontext.Value `json:"metadata,omitzero"`
	Author    *uuid.UUID      `json:"author,omitzero"`
}

// PostHandler serves the CRUD endpoints for Post.
type PostHandler struct {
	repo blog.PostRepository
}

// NewPostHandler returns a handler backed by repo.
func NewPostHandler(repo blog.PostRepository) *PostHandler {
	return &PostHandler{repo: repo}
}

// RegisterPostRoutes registers the Post REST routes on mux.
func RegisterPostRoutes(mux *http.ServeMux, h *PostHandler) {
	mux.HandleFunc("POST /posts", h.Create)
	mux.HandleFunc("GET /posts", h.List)
	mux.HandleFunc("GET /posts/{id}", h.Get)
	mux.HandleFunc("PUT /posts/{id}", h.Update)
	mux.HandleFunc("DELETE /posts/{id}", h.Delete)
}

func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreatePostRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err)
		return
	}

	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}

	m := blog.Post{
		Title:     req.Title,
		Body:      req.Body,
		Published: valueOr(req.Published, defaultPostPublished),
		Views:     valueOr(req.Views, defaultPostViews),
		Metadata:  req.Metadata,
		Author:    req.Author,
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}

	writeBody(w, r, http.StatusCreated, m)
}

func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, parseText[uuid.UUID])
	if !ok {
		return
	}

	m, err := h.repo.Get(r.Context(), id)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}

	writeBody(w, r, http.StatusOK, m)
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	q := newListQuery(r, queryCursor, queryPostPublished, queryPostAuthor)
	limit := q.limit()
	p := blog.PostListParams{
		Published: queryValue(q, queryPostPublished, strconv.ParseBool),
		Author:    queryValue(q, queryPostAuthor, parseText[uuid.UUID]),
		Dir:       q.dir(),
		Limit:     limit + 1,
	}

	if c := queryCursorValue[postCursor](q); c != nil {
		if c.Dir != p.Dir {
			q.invalid(queryCursor)
		}

		p.After = &c.After
	}

	if q.details != nil {
		writeError(w, r, http.StatusBadRequest, codeInvalidQuery, "invalid query parameters", q.details...)
		return
	}

	items, err := h.repo.List(r.Context(), p)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}

	items, more := trimPage(items, limit)
	page := cursorPage[blog.Post]{Items: items}

	if more {
		last := items[len(items)-1]
		c := postCursor{
			Dir: p.Dir,
			After: blog.PostCursor{
				CreatedAt: last.CreatedAt,
				ID:        last.ID,
			},
		}

		if page.NextCursor, err = encodeCursor(c); err != nil {
			writeInternalError(w, r, err)
			return
		}
	}

	writeBody(w, r, http.StatusOK, page)
}

func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, parseText[uuid.UUID])
	if !ok {
		return
	}

	var req UpdatePostRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err)
		return
	}

	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}

	m := blog.Post{
		ID:        id,
		Title:     req.Title,
		Body:      req.Body,
		Published: valueOr(req.Published, defaultPostPublished),
		Views:     valueOr(req.Views, defaultPostViews),
		Metadata:  req.Metadata,
		Author:    req.Author,
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}

	writeBody(w, r, http.StatusOK, m)
}

func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, parseText[uuid.UUID])
	if !ok {
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeRepoError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
