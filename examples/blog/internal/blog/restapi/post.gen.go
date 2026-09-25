package restapi

import (
	"encoding/json/jsontext"
	"net/http"

	"example.com/blog/internal/blog"
	"github.com/google/uuid"
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	writeJSON(w, http.StatusCreated, m)
}

func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m, err := h.repo.Get(r.Context(), id)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req UpdatePostRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	writeJSON(w, http.StatusOK, m)
}

func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeRepoError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
