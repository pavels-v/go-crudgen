package restapi

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"

	"example.com/blogservice/internal/blog/domain"
)

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

const (
	contentTypeJSON = "application/json"
	ruleParamSep    = "="
	jsonPointerRoot = "/"
)

var repoErrors = map[error]repoError{ //nolint:gochecknoglobals // read-only lookup table
	domain.ErrNotFound:          {status: http.StatusNotFound, code: codeNotFound},
	domain.ErrAlreadyExists:     {status: http.StatusConflict, code: codeAlreadyExists},
	domain.ErrReferenceNotFound: {status: http.StatusUnprocessableEntity, code: codeReferenceNotFound},
	domain.ErrStillReferenced:   {status: http.StatusConflict, code: codeStillReferenced},
}

type repoError struct {
	status int
	code   string
}

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

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)

	if err := json.MarshalWrite(w, v); err != nil {
		slog.ErrorContext(r.Context(), "failed to write response", "method", r.Method, "path", r.URL.Path, "error", err)
	}
}

func writeBody[T any](w http.ResponseWriter, r *http.Request, status int, body T) {
	writeJSON(w, r, status, bodyResponse[T]{Body: body})
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, details ...errorDetail) {
	writeJSON(w, r, status, errorResponse{Error: apiError{Code: code, Message: message, Details: details}})
}

func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, codeBodyTooLarge, "request body too large")
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

	writeError(w, r, http.StatusBadRequest, codeMalformedBody, "malformed request body", detail)
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

	writeError(w, r, http.StatusUnprocessableEntity, codeValidationFailed, "request body failed validation", details...)
}

func writeInvalidID(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusBadRequest, codeInvalidID, "invalid id")
}

func writeRepoError(w http.ResponseWriter, r *http.Request, err error) {
	for sentinel, re := range repoErrors {
		if errors.Is(err, sentinel) {
			writeError(w, r, re.status, re.code, sentinel.Error())
			return
		}
	}

	writeInternalError(w, r, err)
}

func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "failed to handle request", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, r, http.StatusInternalServerError, codeInternal, http.StatusText(http.StatusInternalServerError))
}
