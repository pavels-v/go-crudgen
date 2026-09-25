package blog

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrReferenceNotFound = errors.New("referenced entity not found")
	ErrStillReferenced   = errors.New("entity is still referenced")
)
