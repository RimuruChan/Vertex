package console

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// ConsoleStore owns site-wide account governance and domain-scoped content.
// Stats and accounts are only exposed by the site administrator routes.
type ConsoleStore struct{ db *database.DB }

func NewConsoleStore(db *database.DB) *ConsoleStore { return &ConsoleStore{db: db} }

// Stats gathers the dashboard in two round trips: one aggregate over the
// business tables and one over the judge queue.
func (s *ConsoleStore) Stats(ctx context.Context) (*Stats, error) {
	var stats Stats
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT
		   (SELECT count(*) FROM users)::int,
		   (SELECT count(*) FROM users WHERE created_at >= now() - interval '1 day')::int,
		   (SELECT count(*) FROM problems)::int,
		   (SELECT count(*) FROM problems WHERE visibility = 'public' AND published_version IS NOT NULL)::int,
		   (SELECT count(*) FROM submissions)::int,
		   (SELECT count(*) FROM submissions WHERE submitted_at >= now() - interval '1 day')::int,
		   (SELECT count(*) FROM contests)::int,
		   (SELECT count(*) FROM contests WHERE now() BETWEEN begin_at AND end_at)::int,
		   (SELECT count(*) FROM editorials WHERE status = 'published')::int,
		   (SELECT count(*) FROM problem_sets)::int`,
	).Scan(&stats.Users, &stats.UsersToday, &stats.Problems, &stats.PublicProblems,
		&stats.Submissions, &stats.SubmissionsToday, &stats.Contests, &stats.RunningContests,
		&stats.Editorials, &stats.ProblemSets); err != nil {
		return nil, err
	}

	var oldest sql.NullTime
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT
		   count(*) FILTER (WHERE state = 'queued')::int,
		   count(*) FILTER (WHERE state = 'running')::int,
		   count(*) FILTER (WHERE state = 'dead')::int,
		   min(created_at) FILTER (WHERE state = 'queued'),
		   count(DISTINCT worker_id) FILTER (
		     WHERE state = 'running' AND lease_expires_at >= now())::int
		 FROM judge_jobs`,
	).Scan(&stats.QueuedJobs, &stats.RunningJobs, &stats.DeadJobs, &oldest, &stats.ActiveWorkers); err != nil {
		return nil, err
	}
	if oldest.Valid {
		at := oldest.Time
		stats.OldestQueued = &at
	}

	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT status, count(*)::int FROM submissions
		 WHERE submitted_at >= now() - interval '1 day'
		 GROUP BY status ORDER BY count(*) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats.VerdictBreakdown = []VerdictCount{}
	for rows.Next() {
		var item VerdictCount
		if err := rows.Scan(&item.Verdict, &item.Count); err != nil {
			return nil, err
		}
		stats.VerdictBreakdown = append(stats.VerdictBreakdown, item)
	}
	return &stats, rows.Err()
}

// ---------- accounts ----------

const accountColumns = `u.id, u.username, u.email, u.role, u.rating, u.created_at,
	u.disabled_at, u.disabled_reason,
	(SELECT count(*) FROM submissions AS s WHERE s.user_id = u.id)::int,
	(SELECT count(DISTINCT s.problem_id) FROM submissions AS s
	   WHERE s.user_id = u.id AND s.status = 'Accepted')::int`

func scanAccount(scanner interface{ Scan(...any) error }) (AccountSummary, error) {
	var item AccountSummary
	err := scanner.Scan(&item.ID, &item.Username, &item.Email, &item.Role, &item.Rating,
		&item.CreatedAt, &item.DisabledAt, &item.DisabledReason,
		&item.SubmissionCount, &item.SolvedCount)
	return item, err
}

func (s *ConsoleStore) ListAccounts(ctx context.Context, filters AccountFilters) ([]AccountSummary, int, error) {
	clauses := []string{"1 = 1"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, strings.Replace(clause, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if filters.Keyword != "" {
		args = append(args, "%"+filters.Keyword+"%")
		position := strconv.Itoa(len(args))
		clauses = append(clauses, "(u.username ILIKE $"+position+" OR u.email ILIKE $"+position+")")
	}
	if filters.Role != "" {
		add("u.role = ?", filters.Role)
	}
	if filters.OnlyDisabled {
		clauses = append(clauses, "u.disabled_at IS NOT NULL")
	}
	where := strings.Join(clauses, " AND ")

	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT count(*)::int FROM users AS u WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM users AS u WHERE `+where+`
		 ORDER BY u.created_at DESC
		 LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []AccountSummary{}
	for rows.Next() {
		item, err := scanAccount(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}

// UpdateAccount applies only the fields the caller set. Disabling an account
// also revokes its sessions, so a blocked user is signed out immediately
// instead of at the end of their refresh window.
func (s *ConsoleStore) UpdateAccount(ctx context.Context, userID string, update AccountUpdate) (*AccountSummary, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	assignments := []string{}
	args := []any{userID}
	add := func(assignment string, value any) {
		args = append(args, value)
		assignments = append(assignments, strings.Replace(assignment, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if update.Role != nil {
		add("role = ?", *update.Role)
	}
	if update.Rating != nil {
		add("rating = ?", *update.Rating)
	}
	if update.Disabled != nil {
		if *update.Disabled {
			add("disabled_reason = ?", update.Reason)
			assignments = append(assignments, "disabled_at = now()")
		} else {
			assignments = append(assignments, "disabled_at = NULL", "disabled_reason = ''")
		}
	}

	if len(assignments) > 0 {
		result, err := tx.ExecContext(ctx,
			`UPDATE users SET `+strings.Join(assignments, ", ")+` WHERE id = $1`, args...)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 0 {
			return nil, ErrNotFound
		}
	}
	if update.Disabled != nil && *update.Disabled {
		if _, err := tx.ExecContext(ctx,
			`UPDATE auth_sessions SET revoked_at = now()
			 WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	item, err := scanAccount(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM users AS u WHERE u.id = $1`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
