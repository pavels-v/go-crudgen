package blog

import (
	"time"

	"github.com/google/uuid"
)

// Author represents a author.
type Author struct {
	ID     uuid.UUID  `json:"id"`
	Email  string     `json:"email" validate:"required,email"`
	Name   *string    `json:"name,omitempty"`
	BornOn *time.Time `json:"born_on,omitempty"`
}

// CreateAuthorRequest is the request body for creating a author.
type CreateAuthorRequest struct {
	Email  string     `json:"email" validate:"required,email"`
	Name   *string    `json:"name,omitempty"`
	BornOn *time.Time `json:"born_on,omitempty"`
}

// UpdateAuthorRequest is the request body for replacing a author.
type UpdateAuthorRequest struct {
	Email  string     `json:"email" validate:"required,email"`
	Name   *string    `json:"name,omitempty"`
	BornOn *time.Time `json:"born_on,omitempty"`
}
