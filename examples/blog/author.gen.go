package blog

import (
	"github.com/google/uuid"
)

// Author represents a author.
type Author struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email" validate:"required,email"`
	Name  string    `json:"name"`
}
