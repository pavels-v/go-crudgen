package restapi

import (
	"encoding"
	"encoding/json/v2"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

const (
	pathParamID  = "id"
	tagJSON      = "json"
	tagOptionSep = ","
	maxBodyBytes = 1 << 20
)

// validate checks request DTOs and reports fields by their json names.
var validate = newValidator() //nolint:gochecknoglobals // caches struct metadata across requests

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get(tagJSON), tagOptionSep)
		return name
	})

	return v
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxBodyBytes), v, json.RejectUnknownMembers(true))
}

// pathID parses the {id} path segment and answers 400 when it is malformed.
func pathID[T any](w http.ResponseWriter, r *http.Request, parse func(string) (T, error)) (T, bool) {
	id, err := parse(r.PathValue(pathParamID))
	if err != nil {
		writeInvalidID(w, r)
		return id, false
	}

	return id, true
}

func valueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}

	return *p
}

func parseString(s string) (string, error) {
	return s, nil
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
