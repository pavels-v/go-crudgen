package restapi

import (
	"net/http"

	"example.com/blogservice/internal/blog"
	"github.com/google/uuid"
)

// CreateAuthorRequest is the request body for creating the author entity.
type CreateAuthorRequest struct {
	Email  string     `json:"email" validate:"required,email"`
	Name   *string    `json:"name,omitzero"`
	BornOn *blog.Date `json:"born_on,omitzero"`
}

// UpdateAuthorRequest is the request body for replacing the author entity.
type UpdateAuthorRequest struct {
	Email  string     `json:"email" validate:"required,email"`
	Name   *string    `json:"name,omitzero"`
	BornOn *blog.Date `json:"born_on,omitzero"`
}

// AuthorHandler serves the CRUD endpoints for Author.
type AuthorHandler struct {
	repo blog.AuthorRepository
}

// NewAuthorHandler returns a handler backed by repo.
func NewAuthorHandler(repo blog.AuthorRepository) *AuthorHandler {
	return &AuthorHandler{repo: repo}
}

// RegisterAuthorRoutes registers the Author REST routes on mux.
func RegisterAuthorRoutes(mux *http.ServeMux, h *AuthorHandler) {
	mux.HandleFunc("POST /authors", h.Create)
	mux.HandleFunc("GET /authors", h.List)
	mux.HandleFunc("GET /authors/{id}", h.Get)
	mux.HandleFunc("PUT /authors/{id}", h.Update)
	mux.HandleFunc("DELETE /authors/{id}", h.Delete)
}

func (h *AuthorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateAuthorRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m := blog.Author{
		Email:  req.Email,
		Name:   req.Name,
		BornOn: req.BornOn,
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *AuthorHandler) Get(w http.ResponseWriter, r *http.Request) {
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

func (h *AuthorHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AuthorHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req UpdateAuthorRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m := blog.Author{
		ID:     id,
		Email:  req.Email,
		Name:   req.Name,
		BornOn: req.BornOn,
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *AuthorHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
