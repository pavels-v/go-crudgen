package restapi

import (
	"net/http"

	domain "example.com/blogservice/internal/blog"
)

const (
	queryTagGroup = "group"
)

const (
	defaultTagColor  = "gray"
	defaultTagWeight = 1
	defaultTagGroup  = "general"
)

// CreateTagRequest is the request body for creating the tag entity.
type CreateTagRequest struct {
	Slug   string   `json:"slug" validate:"required"`
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
	Group  *string  `json:"group"`
}

// UpdateTagRequest is the request body for replacing the tag entity.
type UpdateTagRequest struct {
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
	Group  *string  `json:"group"`
}

// TagHandler serves the CRUD endpoints for Tag.
type TagHandler struct {
	repo domain.TagRepository
}

// NewTagHandler returns a handler backed by repo.
func NewTagHandler(repo domain.TagRepository) *TagHandler {
	return &TagHandler{repo: repo}
}

// RegisterRoutes registers the Tag REST routes on mux.
func (h *TagHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /tags", h.Create)
	mux.HandleFunc("GET /tags", h.List)
	mux.HandleFunc("GET /tags/{id}", h.Get)
	mux.HandleFunc("PUT /tags/{id}", h.Update)
	mux.HandleFunc("DELETE /tags/{id}", h.Delete)
}

func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err)
		return
	}

	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}

	m := domain.Tag{
		Slug:   req.Slug,
		Label:  req.Label,
		Color:  valueOr(req.Color, defaultTagColor),
		Weight: valueOr(req.Weight, defaultTagWeight),
		Group:  valueOr(req.Group, defaultTagGroup),
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}

	writeBody(w, r, http.StatusCreated, m)
}

func (h *TagHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, parseString)
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

func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	q := newListQuery(r, queryOffset, queryTagGroup)
	limit := q.limit()
	p := domain.TagListParams{
		Group:  queryValue(q, queryTagGroup, parseString),
		Dir:    q.dir(),
		Limit:  limit + 1,
		Offset: q.offset(),
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
	writeBody(w, r, http.StatusOK, offsetPage[domain.Tag]{Items: items, Limit: limit, Offset: p.Offset, HasMore: more})
}

func (h *TagHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, parseString)
	if !ok {
		return
	}

	var req UpdateTagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, r, err)
		return
	}

	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}

	m := domain.Tag{
		Slug:   id,
		Label:  req.Label,
		Color:  valueOr(req.Color, defaultTagColor),
		Weight: valueOr(req.Weight, defaultTagWeight),
		Group:  valueOr(req.Group, defaultTagGroup),
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}

	writeBody(w, r, http.StatusOK, m)
}

func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, parseString)
	if !ok {
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeRepoError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
