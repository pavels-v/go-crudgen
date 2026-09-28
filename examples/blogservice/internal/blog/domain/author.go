package domain

import (
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
