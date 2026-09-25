package blog

// Tag is the API model of the tag entity.
type Tag struct {
	Slug   string  `json:"slug"`
	Label  string  `json:"label" validate:"required"`
	Color  string  `json:"color"`
	Weight float64 `json:"weight"`
}

// CreateTagRequest is the request body for creating the tag entity.
type CreateTagRequest struct {
	Slug   string   `json:"slug" validate:"required"`
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
}

// UpdateTagRequest is the request body for replacing the tag entity.
type UpdateTagRequest struct {
	Label  string   `json:"label" validate:"required"`
	Color  *string  `json:"color"`
	Weight *float64 `json:"weight"`
}
