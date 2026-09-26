package restapi

import (
	"encoding"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"

	"example.com/blogservice/internal/blog"
)

// validate is the shared validator used to check request DTOs. Field names in
// its errors are the json names, so details point at the request body.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get(tagJSON), tagOptionSep)
		return name
	})
	return v
}

const (
	codeMalformedBody     = "malformed_body"
	codeBodyTooLarge      = "body_too_large"
	codeInvalidID         = "invalid_id"
	codeInvalidQuery      = "invalid_query"
	codeValidationFailed  = "validation_failed"
	codeNotFound          = "not_found"
	codeMethodNotAllowed  = "method_not_allowed"
	codeAlreadyExists     = "already_exists"
	codeReferenceNotFound = "reference_not_found"
	codeStillReferenced   = "still_referenced"
	codeInternal          = "internal"
)

const (
	reasonUnknownField   = "unknown_field"
	reasonDuplicateField = "duplicate_field"
	reasonSyntax         = "syntax"
	reasonInvalidValue   = "invalid_value"
)

type repoError struct {
	status int
	code   string
}

var repoErrors = map[error]repoError{
	blog.ErrNotFound:          {http.StatusNotFound, codeNotFound},
	blog.ErrAlreadyExists:     {http.StatusConflict, codeAlreadyExists},
	blog.ErrReferenceNotFound: {http.StatusUnprocessableEntity, codeReferenceNotFound},
	blog.ErrStillReferenced:   {http.StatusConflict, codeStillReferenced},
}

// Deps holds the repository implementation for each entity.
type Deps struct {
	Posts    blog.PostRepository
	Authors  blog.AuthorRepository
	Comments blog.CommentRepository
	Tags     blog.TagRepository
}

// NewRouter wires every entity's routes onto a fresh ServeMux. Requests that
// match no route get the JSON error envelope instead of ServeMux's plain text.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	RegisterPostRoutes(mux, NewPostHandler(deps.Posts))
	RegisterAuthorRoutes(mux, NewAuthorHandler(deps.Authors))
	RegisterCommentRoutes(mux, NewCommentHandler(deps.Comments))
	RegisterTagRoutes(mux, NewTagHandler(deps.Tags))
	return withRouteErrors(mux)
}

func withRouteErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{header: http.Header{}}
		h.ServeHTTP(rec, r)
		if rec.status == http.StatusMethodNotAllowed {
			w.Header().Set("Allow", rec.header.Get("Allow"))
			writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "method not allowed", nil)
			return
		}
		writeError(w, http.StatusNotFound, codeNotFound, "route not found", nil)
	})
}

type statusRecorder struct {
	header http.Header
	status int
}

func (s *statusRecorder) Header() http.Header         { return s.header }
func (s *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (s *statusRecorder) WriteHeader(status int)      { s.status = status }

const (
	pathParamID     = "id"
	queryLimit      = "limit"
	queryOffset     = "offset"
	querySort       = "sort"
	contentTypeJSON = "application/json"
	tagJSON         = "json"
	tagOptionSep    = ","
	ruleParamSep    = "="
	jsonPointerRoot = "/"
	maxBodyBytes    = 1 << 20
)

type bodyResponse[T any] struct {
	Body T `json:"body"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []errorDetail `json:"details"`
}

type errorDetail struct {
	Field  string `json:"field,omitzero"`
	Reason string `json:"reason"`
}

type page[T any] struct {
	Items  []T `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxBodyBytes), v, json.RejectUnknownMembers(true))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, v)
}

func writeBody[T any](w http.ResponseWriter, status int, body T) {
	writeJSON(w, status, bodyResponse[T]{Body: body})
}

func writeError(w http.ResponseWriter, status int, code, message string, details []errorDetail) {
	writeJSON(w, status, errorResponse{Error: apiError{Code: code, Message: message, Details: details}})
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, codeBodyTooLarge, "request body too large", nil)
		return
	}

	detail := errorDetail{Reason: reasonInvalidValue}
	var semantic *json.SemanticError
	var syntactic *jsontext.SyntacticError
	switch {
	case errors.As(err, &semantic):
		detail.Field = string(semantic.JSONPointer)
	case errors.As(err, &syntactic):
		detail.Field = string(syntactic.JSONPointer)
		detail.Reason = reasonSyntax
	}
	switch {
	case errors.Is(err, json.ErrUnknownName):
		detail.Reason = reasonUnknownField
	case errors.Is(err, jsontext.ErrDuplicateName):
		detail.Reason = reasonDuplicateField
	}
	writeError(w, http.StatusBadRequest, codeMalformedBody, "malformed request body", []errorDetail{detail})
}

func writeValidationError(w http.ResponseWriter, r *http.Request, err error) {
	var fields validator.ValidationErrors
	if !errors.As(err, &fields) {
		writeInternalError(w, r, err)
		return
	}
	details := make([]errorDetail, len(fields))
	for i, fe := range fields {
		reason := fe.Tag()
		if fe.Param() != "" {
			reason += ruleParamSep + fe.Param()
		}
		details[i] = errorDetail{Field: jsonPointerRoot + fe.Field(), Reason: reason}
	}
	writeError(w, http.StatusUnprocessableEntity, codeValidationFailed, "request body failed validation", details)
}

func writeInvalidID(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, codeInvalidID, "invalid id", nil)
}

func writeRepoError(w http.ResponseWriter, r *http.Request, err error) {
	for sentinel, re := range repoErrors {
		if errors.Is(err, sentinel) {
			writeError(w, re.status, re.code, sentinel.Error(), nil)
			return
		}
	}
	writeInternalError(w, r, err)
}

func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "failed to handle request", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, codeInternal, http.StatusText(http.StatusInternalServerError), nil)
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

type listQuery struct {
	values  url.Values
	details []errorDetail
}

// newListQuery accepts the paging parameters plus keys; any other parameter,
// and any parameter given more than once, is an error detail.
func newListQuery(r *http.Request, keys ...string) *listQuery {
	q := &listQuery{values: r.URL.Query()}
	for _, key := range slices.Sorted(maps.Keys(q.values)) {
		switch {
		case key != queryLimit && key != queryOffset && !slices.Contains(keys, key):
			q.details = append(q.details, errorDetail{Field: key, Reason: reasonUnknownField})
		case len(q.values[key]) > 1:
			q.details = append(q.details, errorDetail{Field: key, Reason: reasonDuplicateField})
		}
	}
	return q
}

func (q *listQuery) invalid(key string) {
	q.details = append(q.details, errorDetail{Field: key, Reason: reasonInvalidValue})
}

// page reads the optional ?limit= and ?offset= query parameters. A limit above
// maxLimit is clamped.
func (q *listQuery) page() (limit, offset int) {
	limit = defaultLimit
	if v := q.values.Get(queryLimit); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			q.invalid(queryLimit)
		}
		limit = min(n, maxLimit)
	}
	if v := q.values.Get(queryOffset); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			q.invalid(queryOffset)
		}
		offset = n
	}
	return limit, offset
}

// queryValue returns nil when key is absent, so the filter is not applied.
func queryValue[T any](q *listQuery, key string, parse func(string) (T, error)) *T {
	if !q.values.Has(key) {
		return nil
	}
	v, err := parse(q.values.Get(key))
	if err != nil {
		q.invalid(key)
		return nil
	}
	return &v
}

func querySortValue[S ~string](q *listQuery, allowed ...S) S {
	v := S(q.values.Get(querySort))
	if v == "" || slices.Contains(allowed, v) {
		return v
	}
	q.invalid(querySort)
	return ""
}

func parseString(s string) (string, error) {
	return s, nil
}

func parseInt32(s string) (int32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	return int32(n), err
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseText[T any, P interface {
	*T
	encoding.TextUnmarshaler
}](s string) (T, error) {
	var v T
	err := P(&v).UnmarshalText([]byte(s))
	return v, err
}
