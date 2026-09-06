package problem

import (
	"context"
	"database/sql"
)

type counterExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// RebuildPracticeCounters refreshes the public problem aggregates from
// standalone practice submissions only. Contest results have their own
// scoreboard and must not leak through public counters while feedback is
// withheld or the board is frozen.
func RebuildPracticeCounters(ctx context.Context, tx counterExecer, problemID string) error {
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "problem:"+problemID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE problems SET
		   submission_count = (SELECT count(*) FROM submissions
		     WHERE problem_id = $1 AND contest_id IS NULL AND judged_at IS NOT NULL),
		   accepted_count = (SELECT count(*) FROM submissions
		     WHERE problem_id = $1 AND contest_id IS NULL AND status = 'Accepted'),
		   solved_user_count = (SELECT count(DISTINCT user_id) FROM submissions
		     WHERE problem_id = $1 AND contest_id IS NULL AND status = 'Accepted')
		 WHERE id = $1`, problemID)
	return err
}
