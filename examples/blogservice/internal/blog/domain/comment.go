package domain

import (
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
