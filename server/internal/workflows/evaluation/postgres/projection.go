// Package postgres composes evaluation's transactional effects across owners.
// Callers pass the existing transaction; this workflow never commits separately.
package postgres

import (
	"context"
	"database/sql"

	contest "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	problem "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
)

func Rebuild(ctx context.Context, tx *sql.Tx, contestID *string, userID, problemID string) error {
	if contestID != nil && *contestID != "" {
		return contest.RebuildCell(ctx, tx, *contestID, userID, problemID)
	}
	return problem.RebuildPracticeCounters(ctx, tx, problemID)
}
