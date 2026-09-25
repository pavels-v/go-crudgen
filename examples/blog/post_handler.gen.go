package blog

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// PostRepository is the storage interface the Post handlers depend on.
// The concrete implementation is provided by the caller.
type PostRepository interface {
	Create(ctx context.Context, m *Post) error
	Get(ctx context.Context, id uuid.UUID) (*Post, error)
	List(ctx context.Context, limit, offset int) ([]Post, error)
	Update(ctx context.Context, m *Post) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// PostHandler serves the CRUD endpoints for Post.
type PostHandler struct {
	repo PostRepository
}

// NewPostHandler returns a handler backed by repo.
func NewPostHandler(repo PostRepository) *PostHandler {
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
	m := Post{
		ID:        req.ID,
		Title:     req.Title,
		Body:      req.Body,
		Published: req.Published,
		Author:    req.Author,
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
		writeRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	m := Post{
		ID:        id,
		Title:     req.Title,
		Body:      req.Body,
		Published: req.Published,
		Author:    req.Author,
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, err)
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
		writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
