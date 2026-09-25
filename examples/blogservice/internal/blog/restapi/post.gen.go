package restapi

import (
	"encoding/json/jsontext"
	"net/http"

	"github.com/google/uuid"

	"example.com/blogservice/internal/blog"
)

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
		writeDecodeError(w, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}
	m := blog.Post{
		Title:     req.Title,
		Body:      req.Body,
		Published: valueOr(req.Published, false),
		Views:     valueOr(req.Views, 0),
		Metadata:  req.Metadata,
		Author:    req.Author,
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeBody(w, http.StatusCreated, m)
}

func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeInvalidID(w)
		return
	}
	m, err := h.repo.Get(r.Context(), id)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeBody(w, http.StatusOK, m)
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, details := parsePage(r)
	if details != nil {
		writeError(w, http.StatusBadRequest, codeInvalidQuery, "invalid query parameters", details)
		return
	}
	items, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeBody(w, http.StatusOK, page[blog.Post]{Items: items, Limit: limit, Offset: offset})
}

func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeInvalidID(w)
		return
	}
	var req UpdatePostRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
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
		Published: valueOr(req.Published, false),
		Views:     valueOr(req.Views, 0),
		Metadata:  req.Metadata,
		Author:    req.Author,
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeBody(w, http.StatusOK, m)
}

func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeInvalidID(w)
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeRepoError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
