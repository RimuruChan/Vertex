package domain

import "context"

type Queries interface {
	ByUsername(ctx context.Context, username string) (*Profile, error)
}
