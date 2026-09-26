package blog

import (
	"context"
	"encoding/json/jsontext"
	"time"

	"github.com/google/uuid"
)

// Post is the API model of the post entity.
type Post struct {
	ID        uuid.UUID       `json:"id"`
	Title     string          `json:"title"`
	Body      *string         `json:"body,omitzero"`
	Published bool            `json:"published"`
	Views     int64           `json:"views"`
	Metadata  *jsontext.Value `json:"metadata,omitzero"`
	Author    *uuid.UUID      `json:"author,omitzero"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type PostSort string

const (
	PostSortTitle     PostSort = "title"
	PostSortViews     PostSort = "views"
	PostSortCreatedAt PostSort = "created_at"
)

type PostCursor struct {
	Title     string    `json:"title"`
	Views     int64     `json:"views"`
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type PostListParams struct {
	Published *bool
	Author    *uuid.UUID
	Sort      PostSort
	Dir       SortDir
	After     *PostCursor
	Limit     int
}

// PostRepository is the storage interface for Post.
type PostRepository interface {
	Create(ctx context.Context, m *Post) error
	Get(ctx context.Context, id uuid.UUID) (*Post, error)
	List(ctx context.Context, p PostListParams) ([]Post, error)
	Update(ctx context.Context, m *Post) error
	Delete(ctx context.Context, id uuid.UUID) error
}
