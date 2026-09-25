package blog

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// CommentRepository is the storage interface the Comment handlers depend on.
// The concrete implementation is provided by the caller.
type CommentRepository interface {
	Create(ctx context.Context, m *Comment) error
	Get(ctx context.Context, id int64) (*Comment, error)
	List(ctx context.Context, limit, offset int) ([]Comment, error)
	Update(ctx context.Context, m *Comment) error
	Delete(ctx context.Context, id int64) error
}

// CommentHandler serves the CRUD endpoints for Comment.
type CommentHandler struct {
	repo CommentRepository
}

// NewCommentHandler returns a handler backed by repo.
func NewCommentHandler(repo CommentRepository) *CommentHandler {
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m := Comment{
		Post:     req.Post,
		Body:     req.Body,
		Likes:    valueOr(req.Likes, 0),
		PostedAt: valueOr(req.PostedAt, time.Now()),
	}
	if err := h.repo.Create(r.Context(), &m); err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *CommentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 64)
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

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeRepoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req UpdateCommentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	m := Comment{
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
	writeJSON(w, http.StatusOK, m)
}

func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue(pathParamID), 10, 64)
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
