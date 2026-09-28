package domain

import (
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

type PostCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type PostListParams struct {
	Published *bool
	Author    *uuid.UUID
	Dir       SortDir
	After     *PostCursor
	Limit     int
}
