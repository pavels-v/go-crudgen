package blog

// Tag represents a tag.
type Tag struct {
	Slug   string  `json:"slug"`
	Label  string  `json:"label" validate:"required"`
	Color  string  `json:"color"`
	Weight float64 `json:"weight"`
}

// CreateTagRequest is the request body for creating a tag.
type CreateTagRequest struct {
	Slug   string   `json:"slug" validate:"required"`
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
}

// UpdateTagRequest is the request body for replacing a tag.
type UpdateTagRequest struct {
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
}
