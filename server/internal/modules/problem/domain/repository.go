package domain

import "context"

type Queries interface {
	Access(ctx context.Context, id, userID string) (Access, error)
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	List(ctx context.Context, filters Filters) ([]ProblemView, int, error)
	UserStatuses(ctx context.Context, viewerID string, problemIDs []string) (map[string]string, error)
	Tags(ctx context.Context) ([]Tag, error)
	Get(ctx context.Context, id string) (*ProblemView, error)
	GetWorkspace(ctx context.Context, id string) (*ProblemView, error)
}

type Repository interface {
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
	Create(ctx context.Context, authorID string, input *CreateInput) (*ProblemView, error)
	Delete(ctx context.Context, id string) error
}
