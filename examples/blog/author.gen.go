package blog

import (
	"github.com/google/uuid"
)

// Author is the API model of the author entity.
type Author struct {
	ID     uuid.UUID `json:"id"`
	Email  string    `json:"email" validate:"required,email"`
	Name   *string   `json:"name,omitzero"`
	BornOn *Date     `json:"born_on,omitzero"`
}

// CreateAuthorRequest is the request body for creating the author entity.
type CreateAuthorRequest struct {
	Email  string  `json:"email" validate:"required,email"`
	Name   *string `json:"name,omitzero"`
	BornOn *Date   `json:"born_on,omitzero"`
}

// UpdateAuthorRequest is the request body for replacing the author entity.
type UpdateAuthorRequest struct {
	Email  string  `json:"email" validate:"required,email"`
	Name   *string `json:"name,omitzero"`
	BornOn *Date   `json:"born_on,omitzero"`
}
