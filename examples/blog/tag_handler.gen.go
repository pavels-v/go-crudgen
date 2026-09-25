package blog

import (
	"context"
	"net/http"
)

// TagRepository is the storage interface the Tag handlers depend on.
// The concrete implementation is provided by the caller.
type TagRepository interface {
	Create(ctx context.Context, m *Tag) error
	Get(ctx context.Context, id string) (*Tag, error)
	List(ctx context.Context, limit, offset int) ([]Tag, error)
	Update(ctx context.Context, m *Tag) error
	Delete(ctx context.Context, id string) error
}

// TagHandler serves the CRUD endpoints for Tag.
type TagHandler struct {
	repo TagRepository
}

// NewTagHandler returns a handler backed by repo.
func NewTagHandler(repo TagRepository) *TagHandler {
	return &TagHandler{repo: repo}
}

// RegisterTagRoutes registers the Tag REST routes on mux.
func RegisterTagRoutes(mux *http.ServeMux, h *TagHandler) {
	mux.HandleFunc("POST /tags", h.Create)
	mux.HandleFunc("GET /tags", h.List)
	mux.HandleFunc("GET /tags/{id}", h.Get)
	mux.HandleFunc("PUT /tags/{id}", h.Update)
	mux.HandleFunc("DELETE /tags/{id}", h.Delete)
}

func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m := Tag{
		Slug:   req.Slug,
		Label:  req.Label,
		Color:  valueOr(req.Color, "gray"),
		Weight: valueOr(req.Weight, 1),
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *TagHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue(pathParamID)
	m, err := h.repo.Get(r.Context(), id)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *TagHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue(pathParamID)
	var req UpdateTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m := Tag{
		Slug:   id,
		Label:  req.Label,
		Color:  valueOr(req.Color, "gray"),
		Weight: valueOr(req.Weight, 1),
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue(pathParamID)
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeRepoError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
