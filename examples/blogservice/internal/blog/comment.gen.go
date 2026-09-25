package blog

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Comment is the API model of the comment entity.
type Comment struct {
	ID        int64      `json:"id"`
	Post      uuid.UUID  `json:"post"`
	Body      string     `json:"body"`
	Likes     int32      `json:"likes"`
	PostedAt  time.Time  `json:"posted_at"`
	DeletedAt *time.Time `json:"deleted_at,omitzero"`
}

// CommentRepository is the storage interface for Comment.
type CommentRepository interface {
	Create(ctx context.Context, m *Comment) error
	Get(ctx context.Context, id int64) (*Comment, error)
	List(ctx context.Context, limit, offset int) ([]Comment, error)
	Update(ctx context.Context, m *Comment) error
	Delete(ctx context.Context, id int64) error
}
