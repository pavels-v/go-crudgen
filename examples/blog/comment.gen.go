package blog

import (
	"time"

	"github.com/google/uuid"
)

// Comment is the API model of the comment entity.
type Comment struct {
	ID        int64      `json:"id"`
	Post      uuid.UUID  `json:"post" validate:"required"`
	Body      string     `json:"body" validate:"required,max=2000"`
	Likes     int32      `json:"likes"`
	PostedAt  time.Time  `json:"posted_at"`
	DeletedAt *time.Time `json:"deleted_at,omitzero"`
}

// CreateCommentRequest is the request body for creating the comment entity.
type CreateCommentRequest struct {
	Post     uuid.UUID  `json:"post" validate:"required"`
	Body     string     `json:"body" validate:"required,max=2000"`
	Likes    *int32     `json:"likes"`
	PostedAt *time.Time `json:"posted_at"`
}

// UpdateCommentRequest is the request body for replacing the comment entity.
type UpdateCommentRequest struct {
	Post     uuid.UUID  `json:"post" validate:"required"`
	Body     string     `json:"body" validate:"required,max=2000"`
	Likes    *int32     `json:"likes"`
	PostedAt *time.Time `json:"posted_at"`
}
