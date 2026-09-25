package blog

import (
	"encoding/json/jsontext"
	"time"

	"github.com/google/uuid"
)

// Post is the API model of the post entity.
type Post struct {
	ID        uuid.UUID       `json:"id"`
	Title     string          `json:"title" validate:"required,min=1,max=200"`
	Body      *string         `json:"body,omitzero"`
	Published bool            `json:"published"`
	Views     int64           `json:"views"`
	Metadata  *jsontext.Value `json:"metadata,omitzero"`
	Author    *uuid.UUID      `json:"author,omitzero"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// CreatePostRequest is the request body for creating the post entity.
type CreatePostRequest struct {
	Title     string          `json:"title" validate:"required,min=1,max=200"`
	Body      *string         `json:"body,omitzero"`
	Published *bool           `json:"published"`
	Views     *int64          `json:"views"`
	Metadata  *jsontext.Value `json:"metadata,omitzero"`
	Author    *uuid.UUID      `json:"author,omitzero"`
}

// UpdatePostRequest is the request body for replacing the post entity.
type UpdatePostRequest struct {
	Title     string          `json:"title" validate:"required,min=1,max=200"`
	Body      *string         `json:"body,omitzero"`
	Published *bool           `json:"published"`
	Views     *int64          `json:"views"`
	Metadata  *jsontext.Value `json:"metadata,omitzero"`
	Author    *uuid.UUID      `json:"author,omitzero"`
}
