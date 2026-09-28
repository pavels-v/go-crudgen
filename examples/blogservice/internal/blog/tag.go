package blog

import (
	"context"
)

// Tag is the API model of the tag entity.
type Tag struct {
	Slug   string  `json:"slug"`
	Label  string  `json:"label"`
	Color  string  `json:"color"`
	Weight float64 `json:"weight"`
}

type TagListParams struct {
	Dir    SortDir
	Limit  int
	Offset int
}

// TagRepository is the storage interface for Tag.
type TagRepository interface {
	Create(ctx context.Context, m *Tag) error
	Get(ctx context.Context, id string) (*Tag, error)
	List(ctx context.Context, p TagListParams) ([]Tag, error)
	Update(ctx context.Context, m *Tag) error
	Delete(ctx context.Context, id string) error
}
