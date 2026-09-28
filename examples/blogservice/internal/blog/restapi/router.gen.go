package restapi

import (
	"net/http"

	domain "example.com/blogservice/internal/blog"
)

// Deps holds the repository implementation for each entity.
type Deps struct {
	Posts    domain.PostRepository
	Authors  domain.AuthorRepository
	Comments domain.CommentRepository
	Tags     domain.TagRepository
}

// NewRouter registers every entity's routes on a fresh ServeMux and answers
// unmatched requests with the JSON error envelope.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	NewPostHandler(deps.Posts).RegisterRoutes(mux)
	NewAuthorHandler(deps.Authors).RegisterRoutes(mux)
	NewCommentHandler(deps.Comments).RegisterRoutes(mux)
	NewTagHandler(deps.Tags).RegisterRoutes(mux)

	return WithRouteErrors(mux)
}
