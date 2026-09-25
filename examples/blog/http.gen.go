package blog

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
)

// validate is the shared validator used to check request DTOs.
var validate = validator.New()

// ErrNotFound is returned by a repository when an entity does not exist; the
// HTTP layer maps it to a 404 response.
var ErrNotFound = errors.New("not found")

var (
	ErrAlreadyExists     = errors.New("already exists")
	ErrReferenceNotFound = errors.New("referenced entity not found")
	ErrStillReferenced   = errors.New("entity is still referenced")
)

var (
	errTrailingJSON = errors.New("request body must contain a single JSON value")
	errInternal     = errors.New(http.StatusText(http.StatusInternalServerError))
)

var repoErrorStatus = map[error]int{
	ErrNotFound:          http.StatusNotFound,
	ErrAlreadyExists:     http.StatusConflict,
	ErrReferenceNotFound: http.StatusUnprocessableEntity,
	ErrStillReferenced:   http.StatusConflict,
}

// Deps holds the repository implementation for each entity.
type Deps struct {
	Posts    PostRepository
	Authors  AuthorRepository
	Comments CommentRepository
	Tags     TagRepository
}

// NewRouter wires every entity's routes onto a fresh ServeMux.
func NewRouter(deps Deps) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterPostRoutes(mux, NewPostHandler(deps.Posts))
	RegisterAuthorRoutes(mux, NewAuthorHandler(deps.Authors))
	RegisterCommentRoutes(mux, NewCommentHandler(deps.Comments))
	RegisterTagRoutes(mux, NewTagHandler(deps.Tags))
	return mux
}

const (
	pathParamID     = "id"
	queryLimit      = "limit"
	queryOffset     = "offset"
	contentTypeJSON = "application/json"
)

type errorResponse struct {
	Error string `json:"error"`
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&json.RawMessage{}); !errors.Is(err, io.EOF) {
		return errTrailingJSON
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeRepoError(w http.ResponseWriter, r *http.Request, err error) {
	for sentinel, status := range repoErrorStatus {
		if errors.Is(err, sentinel) {
			writeError(w, status, sentinel)
			return
		}
	}
	slog.ErrorContext(r.Context(), "failed to handle request", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, errInternal)
}

func valueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

// parsePage reads optional ?limit= and ?offset= query parameters, clamping them
// to sane defaults. Filtering and sorting are a later milestone.
func parsePage(r *http.Request) (limit, offset int) {
	limit, offset = defaultLimit, 0
	if v := r.URL.Query().Get(queryLimit); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v := r.URL.Query().Get(queryOffset); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
