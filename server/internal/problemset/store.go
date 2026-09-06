package problemset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jackc/pgx/v5/pgtype"
)

// SetStore persists curated problem sets.
type SetStore struct{ db *database.DB }

func NewSetStore(db *database.DB) *SetStore { return &SetStore{db: db} }

// listColumns carries the read-model counts with the row so a listing is one
// query rather than one plus N.
//
// $1 is the viewer ID as text and $2 says whether that viewer is an admin. An
// empty viewer ID means anonymous, in which case the solved count is zero
// without touching the submissions table.
const itemAccessCondition = `(p.visibility = 'public' OR $2 OR
	CASE WHEN $1 = '' THEN FALSE ELSE p.author_id = $1::uuid END)`

const listColumns = `s.id, s.title, s.description, s.author_id, COALESCE(u.username, ''),
	s.visibility, s.created_at, s.updated_at,
	(SELECT count(*) FROM problem_set_problems AS item
	 JOIN problems AS p ON p.id = item.problem_id
	 WHERE item.set_id = s.id AND ` + itemAccessCondition + `)::int,
	CASE WHEN $1 = '' THEN 0 ELSE (
	  SELECT count(*) FROM problem_set_problems AS item
	  JOIN problems AS p ON p.id = item.problem_id
	  WHERE item.set_id = s.id
	    AND ` + itemAccessCondition + `
	    AND EXISTS (
	      SELECT 1 FROM submissions AS sub
	      WHERE sub.user_id = $1::uuid AND sub.problem_id = item.problem_id
	        AND sub.contest_id IS NULL AND sub.status = 'Accepted')
	)::int END`

func scanSet(scanner interface{ Scan(...any) error }) (Set, error) {
	var item Set
	err := scanner.Scan(&item.ID, &item.Title, &item.Description, &item.AuthorID,
		&item.AuthorName, &item.Visibility, &item.CreatedAt, &item.UpdatedAt,
		&item.ProblemCount, &item.SolvedCount)
	return item, err
}

// listConditions builds the filter clause. offset is how many placeholders the
// caller already consumed, because the two queries below do not share a
// parameter list: the page needs the viewer ID for its progress columns and
// the count does not.
func listConditions(filters Filters, offset int) (string, []any) {
	clauses := []string{"1 = 1"}
	args := []any{}
	placeholder := func() string { return "$" + strconv.Itoa(offset+len(args)) }
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, strings.Replace(clause, "?", placeholder(), 1))
	}

	if !filters.Admin {
		// A viewer sees public sets plus their own drafts.
		args = append(args, filters.ViewerID)
		viewer := placeholder()
		clauses = append(clauses,
			"(s.visibility = 'public' OR ("+viewer+" <> '' AND s.author_id = "+viewer+"::uuid))")
	}
	if filters.AuthorID != "" {
		add("s.author_id = ?::uuid", filters.AuthorID)
	}
	if filters.Keyword != "" {
		args = append(args, "%"+filters.Keyword+"%")
		position := placeholder()
		clauses = append(clauses, "(s.title ILIKE "+position+" OR s.description ILIKE "+position+")")
	}
	return strings.Join(clauses, " AND "), args
}

func (s *SetStore) List(ctx context.Context, filters Filters) ([]Set, int, error) {
	// The count query carries no viewer parameter: passing one that the SQL
	// never references leaves PostgreSQL unable to infer its type.
	countWhere, countArgs := listConditions(filters, 0)
	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT count(*)::int FROM problem_sets AS s WHERE `+countWhere, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// The page query puts the viewer first because listColumns reads $1.
	pageWhere, pageArgs := listConditions(filters, 2)
	args := append([]any{filters.ViewerID, filters.Admin}, pageArgs...)
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+listColumns+`
		 FROM problem_sets AS s
		 LEFT JOIN users AS u ON u.id = s.author_id
		 WHERE `+pageWhere+`
		 ORDER BY s.created_at DESC
		 LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Set{}
	for rows.Next() {
		item, err := scanSet(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}

// Get loads one set with its ordered items and the viewer's per-problem status.
func (s *SetStore) Get(ctx context.Context, id, viewerID string, admin bool) (*Set, error) {
	item, err := scanSet(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+listColumns+`
		 FROM problem_sets AS s
		 LEFT JOIN users AS u ON u.id = s.author_id
		 WHERE s.id = $3`, viewerID, admin, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT item.problem_id, p.author_id, item.sort_order, item.note,
		        p.title, p.difficulty, p.visibility,
		        p.submission_count, p.accepted_count,
		        COALESCE(jsonb_agg(t.name ORDER BY t.name)
		          FILTER (WHERE t.name IS NOT NULL), '[]'::jsonb),
		        CASE
		          WHEN $1 = '' THEN 'none'
		          WHEN EXISTS (SELECT 1 FROM submissions AS sub
		                       WHERE sub.user_id = $1::uuid AND sub.problem_id = item.problem_id
		                         AND sub.contest_id IS NULL AND sub.status = 'Accepted') THEN 'solved'
		          WHEN EXISTS (SELECT 1 FROM submissions AS sub
		                       WHERE sub.user_id = $1::uuid AND sub.problem_id = item.problem_id
		                         AND sub.contest_id IS NULL) THEN 'attempted'
		          ELSE 'none'
		        END
		 FROM problem_set_problems AS item
		 JOIN problems AS p ON p.id = item.problem_id
		 LEFT JOIN problem_tags AS pt ON pt.problem_id = p.id
		 LEFT JOIN tags AS t ON t.id = pt.tag_id
		 WHERE item.set_id = $3 AND `+itemAccessCondition+`
		 GROUP BY item.problem_id, p.author_id, item.sort_order, item.note, p.title, p.difficulty,
		          p.visibility, p.submission_count, p.accepted_count
		 ORDER BY item.sort_order`, viewerID, admin, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	item.Items = []Item{}
	for rows.Next() {
		var entry Item
		var tagsJSON []byte
		if err := rows.Scan(&entry.ProblemID, &entry.AuthorID, &entry.SortOrder, &entry.Note,
			&entry.Title, &entry.Difficulty, &entry.Visibility,
			&entry.SubmitCount, &entry.AcceptCount, &tagsJSON, &entry.UserStatus); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &entry.Tags); err != nil {
			return nil, err
		}
		item.Items = append(item.Items, entry)
	}
	return &item, rows.Err()
}

func (s *SetStore) Create(ctx context.Context, authorID string, input UpsertInput) (*Set, error) {
	input = withDefaults(input)
	var id string
	if err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO problem_sets (title, description, author_id, visibility)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		input.Title, input.Description, authorID, input.Visibility).Scan(&id); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, authorID, false)
}

func (s *SetStore) Update(ctx context.Context, id, viewerID string, admin bool, input UpsertInput) (*Set, error) {
	input = withDefaults(input)
	var authorID sql.NullString
	err := s.db.Pool.QueryRowContext(ctx,
		`UPDATE problem_sets SET title = $2, description = $3, visibility = $4, updated_at = now()
		 WHERE id = $1 RETURNING author_id`,
		id, input.Title, input.Description, input.Visibility).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id, viewerID, admin)
}

func (s *SetStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.Pool.ExecContext(ctx, `DELETE FROM problem_sets WHERE id = $1`, id)
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

// SetItems locks the set and every permitted problem before replacement. The
// authorization check and rewrite therefore cannot be separated by a
// visibility change or a concurrent ownership loss.
func (s *SetStore) SetItems(ctx context.Context, id, viewerID string, admin bool, items []ItemInput) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var authorID *string
	if err := tx.QueryRowContext(ctx,
		`SELECT author_id FROM problem_sets WHERE id = $1 FOR UPDATE`, id).Scan(&authorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !admin && (authorID == nil || *authorID != viewerID) {
		return ErrForbidden
	}

	problemIDs := make([]string, 0, len(items))
	for index := range items {
		var parsed pgtype.UUID
		if err := parsed.Scan(items[index].ProblemID); err != nil || !parsed.Valid {
			return invalid("one or more problems are unavailable")
		}
		items[index].ProblemID = parsed.String()
		problemIDs = append(problemIDs, items[index].ProblemID)
	}
	if len(problemIDs) > 0 {
		var viewer any
		if viewerID != "" {
			viewer = viewerID
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM problems
			 WHERE id = ANY($1::uuid[])
			   AND (visibility = 'public' OR $3 OR author_id = $2::uuid)
			 FOR SHARE`, problemIDs, viewer, admin)
		if err != nil {
			return err
		}
		available := 0
		for rows.Next() {
			var problemID string
			if err := rows.Scan(&problemID); err != nil {
				rows.Close()
				return err
			}
			available++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if available != len(problemIDs) {
			return invalid("one or more problems are unavailable")
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM problem_set_problems WHERE set_id = $1`, id); err != nil {
		return err
	}
	for order, entry := range items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO problem_set_problems (set_id, problem_id, sort_order, note)
			 VALUES ($1, $2, $3, $4)`, id, entry.ProblemID, order, entry.Note); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE problem_sets SET updated_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// withDefaults fills the values the visibility CHECK constraint requires. The
// service normally supplies them; this keeps a direct store call from turning a
// missing field into a constraint violation.
func withDefaults(input UpsertInput) UpsertInput {
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}
	return input
}
