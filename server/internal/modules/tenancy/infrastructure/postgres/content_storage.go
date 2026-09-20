package postgres

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Storage producers use a transaction shared lock; the collector uses the
// matching session exclusive lock across its database commit and file removal.
func ContentStorageKey(problemID string) string {
	return "resource-authorization:problem-content:" + problemID
}
func LockContentStorage(ctx context.Context, tx *sqlx.Tx, problemID string) error {
	return ResourceGuard(ctx, tx, "problem-content", problemID, false)
}
