package restapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"example.com/blogservice/internal/blog"
)

const (
	queryCommentPost = "post"
)

// CreateCommentRequest is the request body for creating the comment entity.
type CreateCommentRequest struct {
	Post     uuid.UUID  `json:"post" validate:"required"`
	Body     string     `json:"body" validate:"required,max=2000"`
	Likes    *int32     `json:"likes"`
	PostedAt *time.Time `json:"posted_at"`
}

// UpdateCommentRequest is the request body for replacing the comment entity.
type UpdateCommentRequest struct {
	Post     uuid.UUID  `json:"post" validate:"required"`
	Body     string     `json:"body" validate:"required,max=2000"`
	Likes    *int32     `json:"likes"`
	PostedAt *time.Time `json:"posted_at"`
}

// CommentHandler serves the CRUD endpoints for Comment.
type CommentHandler struct {
	repo blog.CommentRepository
}

// NewCommentHandler returns a handler backed by repo.
func NewCommentHandler(repo blog.CommentRepository) *CommentHandler {
	return &CommentHandler{repo: repo}
}

// RegisterCommentRoutes registers the Comment REST routes on mux.
func RegisterCommentRoutes(mux *http.ServeMux, h *CommentHandler) {
	mux.HandleFunc("POST /comments", h.Create)
	mux.HandleFunc("GET /comments", h.List)
	mux.HandleFunc("GET /comments/{id}", h.Get)
	mux.HandleFunc("PUT /comments/{id}", h.Update)
	mux.HandleFunc("DELETE /comments/{id}", h.Delete)
}

func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateCommentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}
	m := blog.Comment{
		Post:     req.Post,
		Body:     req.Body,
		Likes:    valueOr(req.Likes, 0),
		PostedAt: valueOr(req.PostedAt, time.Now()),
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeBody(w, http.StatusCreated, m)
}

func (h *CommentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 64)
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

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	q := newListQuery(r, queryOffset, queryCommentPost, querySort)
	limit := q.limit()
	p := blog.CommentListParams{
		Post:   queryValue(q, queryCommentPost, parseText[uuid.UUID]),
		Sort:   querySortValue(q, blog.CommentSortPostedAt),
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
	writeBody(w, http.StatusOK, offsetPage[blog.Comment]{Items: items, Limit: limit, Offset: p.Offset, HasMore: more})
}

func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 64)
	if err != nil {
		writeInvalidID(w)
		return
	}
	var req UpdateCommentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeValidationError(w, r, err)
		return
	}
	m := blog.Comment{
		ID:       id,
		Post:     req.Post,
		Body:     req.Body,
		Likes:    valueOr(req.Likes, 0),
		PostedAt: valueOr(req.PostedAt, time.Now()),
	}
	if err := h.repo.Update(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeBody(w, http.StatusOK, m)
}

func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 64)
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
