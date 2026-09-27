package blog

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Comment is the API model of the comment entity.
type Comment struct {
	ID       int64     `json:"id"`
	Post     uuid.UUID `json:"post"`
	Body     string    `json:"body"`
	Likes    int32     `json:"likes"`
	PostedAt time.Time `json:"posted_at"`
}

type CommentListParams struct {
	Post   *uuid.UUID
	Dir    SortDir
	Limit  int
	Offset int
}

// CommentRepository is the storage interface for Comment.
type CommentRepository interface {
	Create(ctx context.Context, m *Comment) error
	Get(ctx context.Context, id int64) (*Comment, error)
	List(ctx context.Context, p CommentListParams) ([]Comment, error)
	Update(ctx context.Context, m *Comment) error
	Delete(ctx context.Context, id int64) error
}
