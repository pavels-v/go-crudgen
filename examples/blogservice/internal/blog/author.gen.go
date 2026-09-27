package blog

import (
	"context"

	"github.com/google/uuid"
)

// Author is the API model of the author entity.
type Author struct {
	ID     uuid.UUID `json:"id"`
	Email  string    `json:"email"`
	Name   *string   `json:"name,omitzero"`
	BornOn *Date     `json:"born_on,omitzero"`
}

type AuthorListParams struct {
	Email  *string
	Dir    SortDir
	Limit  int
	Offset int
}

// AuthorRepository is the storage interface for Author.
type AuthorRepository interface {
	Create(ctx context.Context, m *Author) error
	Get(ctx context.Context, id uuid.UUID) (*Author, error)
	List(ctx context.Context, p AuthorListParams) ([]Author, error)
	Update(ctx context.Context, m *Author) error
	Delete(ctx context.Context, id uuid.UUID) error
}
