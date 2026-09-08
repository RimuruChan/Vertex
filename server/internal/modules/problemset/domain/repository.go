package domain

import "context"

// Repository is the persistence boundary for curated sets.
type Repository interface {
	List(ctx context.Context, filters Filters) ([]Set, int, error)
	Get(ctx context.Context, id, viewerID string) (*Set, error)
	Create(ctx context.Context, authorID string, input UpsertInput) (*Set, error)
	Update(ctx context.Context, id, viewerID string, input UpsertInput) (*Set, error)
	Delete(ctx context.Context, id, viewerID string) error
	SetItems(ctx context.Context, id, viewerID string, items []ItemInput) error
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
}
