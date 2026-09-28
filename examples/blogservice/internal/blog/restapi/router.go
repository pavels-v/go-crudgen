package restapi

import (
	"net/http"
)

// Deps holds the repository implementation for each entity.
type Deps struct {
	Posts    PostRepository
	Authors  AuthorRepository
	Comments CommentRepository
	Tags     TagRepository
}

// NewRouter serves every entity's routes wrapped in WithRouteErrors.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	NewPostHandler(deps.Posts).RegisterRoutes(mux)
	NewAuthorHandler(deps.Authors).RegisterRoutes(mux)
	NewCommentHandler(deps.Comments).RegisterRoutes(mux)
	NewTagHandler(deps.Tags).RegisterRoutes(mux)

	return WithRouteErrors(mux)
}
