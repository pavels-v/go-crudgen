package restapi

import (
	"net/http"

	"github.com/google/uuid"

	"example.com/blogservice/internal/blog"
)

const (
	queryAuthorEmail = "email"
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
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
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
	writeBody(w, http.StatusCreated, m)
}

func (h *AuthorHandler) Get(w http.ResponseWriter, r *http.Request) {
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

func (h *AuthorHandler) List(w http.ResponseWriter, r *http.Request) {
	q := newListQuery(r, queryOffset, queryAuthorEmail)
	limit := q.limit()
	p := blog.AuthorListParams{
		Email:  queryValue(q, queryAuthorEmail, parseString),
		Dir:    q.dir(),
		Limit:  limit + 1,
		Offset: q.offset(),
	}
	if q.details != nil {
		writeError(w, http.StatusBadRequest, codeInvalidQuery, "invalid query parameters", q.details)
		return
	}
	items, err := h.repo.List(r.Context(), p)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	items, more := trimPage(items, limit)
	writeBody(w, http.StatusOK, offsetPage[blog.Author]{Items: items, Limit: limit, Offset: p.Offset, HasMore: more})
}

func (h *AuthorHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(pathParamID))
	if err != nil {
		writeInvalidID(w)
		return
	}
	var req UpdateAuthorRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
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
	writeBody(w, http.StatusOK, m)
}

func (h *AuthorHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
