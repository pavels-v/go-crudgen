package blog

import (
	"github.com/google/uuid"
	"time"
)

// Post represents a post.
type Post struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title" validate:"required,min=1,max=200"`
	Body      string    `json:"body"`
	Published bool      `json:"published"`
	Author    uuid.UUID `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
