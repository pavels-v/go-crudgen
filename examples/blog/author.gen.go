package blog

import (
	"github.com/google/uuid"
)

// Author represents a author.
type Author struct {
	ID    uuid.UUID `json:"id" db:"id"`
	Email string    `json:"email" validate:"required,email" db:"email"`
	Name  string    `json:"name" db:"name"`
}

// CreateAuthorRequest is the request body for creating a author.
type CreateAuthorRequest struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email" validate:"required,email"`
	Name  string    `json:"name"`
}

// UpdateAuthorRequest is the request body for replacing a author.
type UpdateAuthorRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name"`
}
