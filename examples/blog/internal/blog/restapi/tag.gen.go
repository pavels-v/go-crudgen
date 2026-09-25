package restapi

import (
	"net/http"

	"example.com/blog/internal/blog"
)

// CreateTagRequest is the request body for creating the tag entity.
type CreateTagRequest struct {
	Slug   string   `json:"slug" validate:"required"`
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
}

// UpdateTagRequest is the request body for replacing the tag entity.
type UpdateTagRequest struct {
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
}

// TagHandler serves the CRUD endpoints for Tag.
type TagHandler struct {
	repo blog.TagRepository
}

// NewTagHandler returns a handler backed by repo.
func NewTagHandler(repo blog.TagRepository) *TagHandler {
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
	m := blog.Tag{
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
	m := blog.Tag{
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
