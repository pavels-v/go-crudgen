package blog

import (
	"github.com/google/uuid"
	"time"
)

// Post represents a post.
type Post struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Title     string    `json:"title" validate:"required,min=1,max=200" db:"title"`
	Body      string    `json:"body" db:"body"`
	Published bool      `json:"published" db:"published"`
	Author    uuid.UUID `json:"author" db:"author"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CreatePostRequest is the request body for creating a post.
type CreatePostRequest struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title" validate:"required,min=1,max=200"`
	Body      string    `json:"body"`
	Published bool      `json:"published"`
	Author    uuid.UUID `json:"author"`
}

// UpdatePostRequest is the request body for replacing a post.
type UpdatePostRequest struct {
	Title     string    `json:"title" validate:"required,min=1,max=200"`
	Body      string    `json:"body"`
	Published bool      `json:"published"`
	Author    uuid.UUID `json:"author"`
}
