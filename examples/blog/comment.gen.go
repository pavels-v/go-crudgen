package blog

import (
	"time"

	"github.com/google/uuid"
)

// Comment represents a comment.
type Comment struct {
	ID        int64      `json:"id"`
	Post      uuid.UUID  `json:"post" validate:"required"`
	Body      string     `json:"body" validate:"required,max=2000"`
	Likes     int32      `json:"likes"`
	PostedAt  time.Time  `json:"posted_at"`
	DeletedAt *time.Time `json:"deleted_at,omitzero"`
}

// CreateCommentRequest is the request body for creating a comment.
type CreateCommentRequest struct {
	Post     uuid.UUID  `json:"post" validate:"required"`
	Body     string     `json:"body" validate:"required,max=2000"`
	Likes    *int32     `json:"likes"`
	PostedAt *time.Time `json:"posted_at"`
}

// UpdateCommentRequest is the request body for replacing a comment.
type UpdateCommentRequest struct {
	Post     uuid.UUID  `json:"post" validate:"required"`
	Body     string     `json:"body" validate:"required,max=2000"`
	Likes    *int32     `json:"likes"`
	PostedAt *time.Time `json:"posted_at"`
}
