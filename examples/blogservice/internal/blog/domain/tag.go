package domain

// Tag is the API model of the tag entity.
type Tag struct {
	Slug   string  `json:"slug"`
	Label  string  `json:"label"`
	Color  string  `json:"color"`
	Weight float64 `json:"weight"`
	Group  string  `json:"group"`
}

type TagListParams struct {
	Group  *string
	Dir    SortDir
	Limit  int
	Offset int
}
