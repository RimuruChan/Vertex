package console

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jackc/pgx/v5/pgconn"
)

// ConsoleStore reads across every domain for the administration surface.
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
		   (SELECT count(*) FROM problems WHERE visibility = 'public')::int,
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

// ---------- tags ----------

func (s *ConsoleStore) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT t.id, t.name,
		        (SELECT count(*) FROM problem_tags AS pt WHERE pt.tag_id = t.id)::int
		 FROM tags AS t ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Tag{}
	for rows.Next() {
		var item Tag
		if err := rows.Scan(&item.ID, &item.Name, &item.ProblemCount); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// RenameTag renames in place, or merges when the new name already exists.
// Colliding on a unique constraint is the normal way an author discovers the
// duplicate, so it is handled rather than reported as an error.
func (s *ConsoleStore) RenameTag(ctx context.Context, id int64, name string) (*Tag, error) {
	var existingID int64
	err := s.db.Pool.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = $1`, name).Scan(&existingID)
	if err == nil && existingID != id {
		return s.MergeTags(ctx, id, existingID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	var item Tag
	err = s.db.Pool.QueryRowContext(ctx,
		`UPDATE tags SET name = $2 WHERE id = $1
		 RETURNING id, name,
		   (SELECT count(*) FROM problem_tags AS pt WHERE pt.tag_id = tags.id)::int`,
		id, name).Scan(&item.ID, &item.Name, &item.ProblemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.ConstraintName == "tags_name_key" {
			return nil, invalid("a tag with that name already exists")
		}
		return nil, err
	}
	return &item, nil
}

// MergeTags repoints every problem of the source tag at the target and removes
// the source. Problems already carrying both keep a single link.
func (s *ConsoleStore) MergeTags(ctx context.Context, sourceID, targetID int64) (*Tag, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM tags WHERE id = $1)`, targetID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO problem_tags (problem_id, tag_id)
		 SELECT problem_id, $2 FROM problem_tags WHERE tag_id = $1
		 ON CONFLICT (problem_id, tag_id) DO NOTHING`, sourceID, targetID); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = $1`, sourceID)
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

	var item Tag
	if err := tx.QueryRowContext(ctx,
		`SELECT id, name, (SELECT count(*) FROM problem_tags AS pt WHERE pt.tag_id = tags.id)::int
		 FROM tags WHERE id = $1`, targetID).Scan(&item.ID, &item.Name, &item.ProblemCount); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *ConsoleStore) DeleteTag(ctx context.Context, id int64) error {
	result, err := s.db.Pool.ExecContext(ctx, `DELETE FROM tags WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- announcements ----------

const announcementColumns = `a.id, a.title, a.content_md, a.pinned, a.published,
	COALESCE(u.username, ''), a.created_at, a.updated_at`

func scanAnnouncement(scanner interface{ Scan(...any) error }) (Announcement, error) {
	var item Announcement
	err := scanner.Scan(&item.ID, &item.Title, &item.ContentMD, &item.Pinned,
		&item.Published, &item.AuthorName, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *ConsoleStore) ListAnnouncements(ctx context.Context, publishedOnly bool, limit int) ([]Announcement, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+announcementColumns+`
		 FROM announcements AS a LEFT JOIN users AS u ON u.id = a.created_by
		 WHERE (NOT $1 OR a.published)
		 ORDER BY a.pinned DESC, a.created_at DESC LIMIT $2`, publishedOnly, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Announcement{}
	for rows.Next() {
		item, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *ConsoleStore) CreateAnnouncement(ctx context.Context, authorID string, input AnnouncementInput) (*Announcement, error) {
	var creator *string
	if authorID != "" {
		creator = &authorID
	}
	var id string
	if err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO announcements (title, content_md, pinned, published, created_by)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		input.Title, input.ContentMD, input.Pinned, input.Published, creator).Scan(&id); err != nil {
		return nil, err
	}
	return s.announcement(ctx, id)
}

func (s *ConsoleStore) UpdateAnnouncement(ctx context.Context, id string, input AnnouncementInput) (*Announcement, error) {
	result, err := s.db.Pool.ExecContext(ctx,
		`UPDATE announcements SET title = $2, content_md = $3, pinned = $4,
		        published = $5, updated_at = now()
		 WHERE id = $1`, id, input.Title, input.ContentMD, input.Pinned, input.Published)
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
	return s.announcement(ctx, id)
}

func (s *ConsoleStore) DeleteAnnouncement(ctx context.Context, id string) error {
	result, err := s.db.Pool.ExecContext(ctx, `DELETE FROM announcements WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ConsoleStore) announcement(ctx context.Context, id string) (*Announcement, error) {
	item, err := scanAnnouncement(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+announcementColumns+`
		 FROM announcements AS a LEFT JOIN users AS u ON u.id = a.created_by
		 WHERE a.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
