package contest

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Execer is the subset of a transaction the scoreboard rebuild needs. The
// judge result transaction hands its own *sqlx.Tx in so a verdict and the
// standings it produces commit together.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// RebuildCell recomputes one scoreboard square from the submissions that
// currently exist, then writes both the jury and the public view of it.
//
// Recomputing from facts rather than incrementing counters is what makes
// rejudging safe: replaying a verdict, changing one, or deleting a submission
// all converge on the same cell, and a worker retry cannot double-count.
func RebuildCell(ctx context.Context, tx Execer, contestID, userID, problemID string) error {
	// Serialize concurrent rebuilds of the same square. Two verdicts for one
	// contestant and problem can otherwise interleave read and write.
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"contest-cell:"+contestID+":"+userID+":"+problemID); err != nil {
		return err
	}

	rules, err := scoringRules(ctx, tx, contestID, problemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The contest or its problem link is gone; nothing to score.
			return nil
		}
		return err
	}

	submissions, err := loadScoredSubmissions(ctx, tx, contestID, userID, problemID)
	if err != nil {
		return err
	}

	cell := ScoreCell(*rules, submissions)
	if cell.Attempts == 0 && cell.PublicAttempts == 0 && cell.PendingCount == 0 {
		_, err := tx.ExecContext(ctx,
			`DELETE FROM contest_submission_cells
			 WHERE contest_id = $1 AND user_id = $2 AND problem_id = $3`,
			contestID, userID, problemID)
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO contest_submission_cells
		   (contest_id, user_id, problem_id, attempts, penalty_sec, score, solved_at,
		    public_attempts, public_penalty_sec, public_score, public_solved_at,
		    pending_count, last_submit_at, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
		         (SELECT domain_id FROM contests WHERE id = $1))
		 ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
		   attempts = EXCLUDED.attempts,
		   penalty_sec = EXCLUDED.penalty_sec,
		   score = EXCLUDED.score,
		   solved_at = EXCLUDED.solved_at,
		   public_attempts = EXCLUDED.public_attempts,
		   public_penalty_sec = EXCLUDED.public_penalty_sec,
		   public_score = EXCLUDED.public_score,
		   public_solved_at = EXCLUDED.public_solved_at,
		   pending_count = EXCLUDED.pending_count,
		   last_submit_at = EXCLUDED.last_submit_at`,
		contestID, userID, problemID,
		cell.Attempts, cell.PenaltySec, cell.Score, cell.SolvedAt,
		cell.PublicAttempts, cell.PublicPenaltySec, cell.PublicScore, cell.PublicSolvedAt,
		cell.PendingCount, cell.LastSubmitAt)
	return err
}

// scoringRules reads the contest configuration together with the problem's
// point value, so a rebuild sees exactly one consistent rule set.
func scoringRules(ctx context.Context, tx Execer, contestID, problemID string) (*ScoringRules, error) {
	var rules ScoringRules
	var rule string
	err := tx.QueryRowContext(ctx,
		`SELECT contest.rule, contest.begin_at, contest.end_at, contest.freeze_at,
		        contest.penalty_minutes, contest.penalize_compile_error,
		        COALESCE(problem.points, 100)
		 FROM contests AS contest
		 LEFT JOIN contest_problems AS problem
		   ON problem.contest_id = contest.id AND problem.problem_id = $2
		 WHERE contest.id = $1`, contestID, problemID).Scan(
		&rule, &rules.BeginAt, &rules.EndAt, &rules.FreezeAt,
		&rules.PenaltyMinutes, &rules.PenalizeCompileError, &rules.MaxPoints)
	if err != nil {
		return nil, err
	}
	rules.Format = NormalizeFormat(rule)
	return &rules, nil
}

func loadScoredSubmissions(ctx context.Context, tx Execer, contestID, userID, problemID string) ([]ScoredSubmission, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT submitted_at, status, score
		 FROM submissions
		 WHERE contest_id = $1 AND user_id = $2 AND problem_id = $3
		 ORDER BY submitted_at`, contestID, userID, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ScoredSubmission, 0, 8)
	for rows.Next() {
		var item ScoredSubmission
		if err := rows.Scan(&item.SubmittedAt, &item.Status, &item.Score); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// RebuildContest recomputes every cell of a contest. It is the repair path for
// changes that invalidate the whole board at once, such as editing the penalty
// value or the problem point scale after submissions already exist.
func RebuildContest(ctx context.Context, tx Execer, contestID string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT DISTINCT user_id, problem_id FROM submissions WHERE contest_id = $1`, contestID)
	if err != nil {
		return err
	}
	type pair struct{ userID, problemID string }
	pairs := make([]pair, 0, 64)
	for rows.Next() {
		var item pair
		if err := rows.Scan(&item.userID, &item.problemID); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range pairs {
		if err := RebuildCell(ctx, tx, contestID, item.userID, item.problemID); err != nil {
			return err
		}
	}
	return nil
}

// scanTime is a small helper for the nullable timestamps the scoreboard reads.
func scanTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time
	return &at
}
