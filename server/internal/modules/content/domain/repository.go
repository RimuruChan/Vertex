package domain

import "context"

type EditorialRepository interface {
	List(ctx context.Context, filters EditorialFilters) ([]EditorialSummary, int, error)
	Get(ctx context.Context, id, viewerID string) (*Editorial, error)
	Create(ctx context.Context, authorID string, input EditorialInput) (*Editorial, error)
	Update(ctx context.Context, id, userID string, input EditorialInput) (*Editorial, error)
	Delete(ctx context.Context, id, userID string) error
	Vote(ctx context.Context, id, userID string, up bool) (int, error)
}

type DiscussionRepository interface {
	ListByProblem(ctx context.Context, problemID, userID string) (Thread, error)
	ListByEditorial(ctx context.Context, editorialID, userID string) (Thread, error)
	CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	Update(ctx context.Context, postID int64, userID, contentMD string) (*DiscussionPost, error)
	Delete(ctx context.Context, postID int64, userID string) error
}

// ScopeAccess is the content domain's view of target visibility. Its concrete
// implementation may read other domains' tables, but content rules depend on
// this parent visibility check; persistence repeats it inside each mutation.
type ScopeAccess interface {
	CanViewProblem(ctx context.Context, problemID string, userID string) (bool, error)
}
