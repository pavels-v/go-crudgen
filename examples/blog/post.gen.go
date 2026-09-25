package blog

import (
	"time"

	"github.com/google/uuid"
)

// Post represents a post.
type Post struct {
	ID        uuid.UUID  `json:"id"`
	Title     string     `json:"title" validate:"required,min=1,max=200"`
	Body      *string    `json:"body,omitempty"`
	Published *bool      `json:"published,omitempty"`
	Author    *uuid.UUID `json:"author,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreatePostRequest is the request body for creating a post.
type CreatePostRequest struct {
	ID        uuid.UUID  `json:"id"`
	Title     string     `json:"title" validate:"required,min=1,max=200"`
	Body      *string    `json:"body,omitempty"`
	Published *bool      `json:"published,omitempty"`
	Author    *uuid.UUID `json:"author,omitempty"`
}

// UpdatePostRequest is the request body for replacing a post.
type UpdatePostRequest struct {
	Title     string     `json:"title" validate:"required,min=1,max=200"`
	Body      *string    `json:"body,omitempty"`
	Published *bool      `json:"published,omitempty"`
	Author    *uuid.UUID `json:"author,omitempty"`
}
