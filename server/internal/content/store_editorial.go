package content

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

type EditorialStore struct{ db *database.DB }

func NewEditorialStore(db *database.DB) *EditorialStore { return &EditorialStore{db: db} }

const editorialVoteColumn = `CASE WHEN $1 = '' THEN FALSE ELSE EXISTS (
	  SELECT 1 FROM editorial_votes AS v
	  WHERE v.editorial_id = e.id AND v.user_id = $1::uuid) END`

// The list projection intentionally excludes content_md. The detail endpoint
// below is the only public read path that loads an editorial body.
const editorialSummaryColumns = `e.id, e.public_id, e.problem_id, COALESCE(p.public_id::text, ''), COALESCE(p.title, ''), e.author_id,
	COALESCE(u.username, ''), e.title, e.visibility, e.status,
	e.solved_only, e.vote_count, ` + editorialVoteColumn + `,
	e.created_at, e.updated_at`

const editorialColumns = `e.id, e.public_id, e.problem_id, COALESCE(p.public_id::text, ''), COALESCE(p.title, ''), e.author_id,
	COALESCE(u.username, ''), e.title, e.content_md, e.visibility, e.status,
	e.solved_only, e.vote_count, ` + editorialVoteColumn + `,
	e.created_at, e.updated_at`

const editorialJoins = `FROM editorials AS e
	LEFT JOIN users AS u ON u.id = e.author_id
	LEFT JOIN problems AS p ON p.id = e.problem_id`

func scanEditorial(scanner interface{ Scan(...any) error }) (Editorial, error) {
	var item Editorial
	err := scanner.Scan(&item.ID, &item.PublicID, &item.ProblemID, &item.ProblemPublicID, &item.ProblemTitle, &item.AuthorID,
		&item.AuthorName, &item.Title, &item.ContentMD, &item.Visibility, &item.Status,
		&item.SolvedOnly, &item.VoteCount, &item.Voted, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanEditorialSummary(scanner interface{ Scan(...any) error }) (EditorialSummary, error) {
	var item EditorialSummary
	err := scanner.Scan(&item.ID, &item.PublicID, &item.ProblemID, &item.ProblemPublicID, &item.ProblemTitle, &item.AuthorID,
		&item.AuthorName, &item.Title, &item.Visibility, &item.Status,
		&item.SolvedOnly, &item.VoteCount, &item.Voted, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// editorialConditions builds the filter clause. offset is how many
// placeholders the caller already used: the page query starts with the viewer
// ID that the list projection reads, and the count query has no such parameter.
func editorialConditions(filters EditorialFilters, offset int) (string, []any) {
	clauses := []string{"1 = 1"}
	args := []any{}
	placeholder := func() string { return "$" + strconv.Itoa(offset+len(args)) }
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, strings.Replace(clause, "?", placeholder(), 1))
	}

	if !filters.Admin {
		args = append(args, filters.ViewerID)
		viewer := placeholder()
		clauses = append(clauses,
			"((e.status = 'published' AND e.visibility = 'public') OR "+
				"CASE WHEN "+viewer+" = '' THEN FALSE ELSE e.author_id = "+viewer+"::uuid END)")
		// A public editorial must not reveal the title or solution of a problem
		// that the same viewer cannot open. Authors retain access to their own
		// work if the target problem is unpublished later.
		clauses = append(clauses,
			"(p.visibility = 'public' OR CASE WHEN "+viewer+" = '' THEN FALSE ELSE "+
				"p.author_id = "+viewer+"::uuid OR e.author_id = "+viewer+"::uuid END)")
	}
	if filters.ProblemID != "" {
		add("e.problem_id = ?::uuid", filters.ProblemID)
	}
	if filters.AuthorID != "" {
		add("e.author_id = ?::uuid", filters.AuthorID)
	}
	if filters.Keyword != "" {
		args = append(args, "%"+filters.Keyword+"%")
		position := placeholder()
		clauses = append(clauses, "(e.title ILIKE "+position+" OR p.title ILIKE "+position+")")
	}
	return strings.Join(clauses, " AND "), args
}

// List returns editorials the viewer may see. Drafts and private write-ups are
// filtered in SQL so paging counts stay honest.
func (s *EditorialStore) List(ctx context.Context, filters EditorialFilters) ([]EditorialSummary, int, error) {
	countWhere, countArgs := editorialConditions(filters, 0)
	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT count(*)::int `+editorialJoins+` WHERE `+countWhere, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "e.created_at DESC, e.id DESC"
	if filters.Sort == "votes" {
		order = "e.vote_count DESC, e.created_at DESC, e.id DESC"
	}
	pageWhere, pageArgs := editorialConditions(filters, 1)
	args := append([]any{filters.ViewerID}, pageArgs...)
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+editorialSummaryColumns+` `+editorialJoins+`
		 WHERE `+pageWhere+` ORDER BY `+order+`
		 LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []EditorialSummary{}
	for rows.Next() {
		item, err := scanEditorialSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}

func (s *EditorialStore) Get(ctx context.Context, id, viewerID string) (*Editorial, error) {
	item, err := scanEditorial(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+editorialColumns+` `+editorialJoins+` WHERE e.id = $2`, viewerID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *EditorialStore) Create(ctx context.Context, authorID string, input EditorialInput) (*Editorial, error) {
	var id string
	if err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO editorials (problem_id, author_id, title, content_md, visibility, status, solved_only)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		input.ProblemID, authorID, input.Title, input.ContentMD,
		input.Visibility, input.Status, input.SolvedOnly).Scan(&id); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, authorID)
}

func (s *EditorialStore) Update(ctx context.Context, id string, input EditorialInput) (*Editorial, error) {
	var authorID sql.NullString
	err := s.db.Pool.QueryRowContext(ctx,
		`UPDATE editorials SET title = $2, content_md = $3, visibility = $4,
		        status = $5, solved_only = $6, updated_at = now()
		 WHERE id = $1 RETURNING author_id`,
		id, input.Title, input.ContentMD, input.Visibility, input.Status, input.SolvedOnly).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id, authorID.String)
}

func (s *EditorialStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.Pool.ExecContext(ctx, `DELETE FROM editorials WHERE id = $1`, id)
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

// Vote records or withdraws an upvote and recomputes the cached total from the
// vote table, so a double click or a retry cannot drift the counter.
func (s *EditorialStore) Vote(ctx context.Context, id, userID string, up bool) (int, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if up {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO editorial_votes (editorial_id, user_id) VALUES ($1, $2)
			 ON CONFLICT (editorial_id, user_id) DO NOTHING`, id, userID); err != nil {
			return 0, err
		}
	} else if _, err := tx.ExecContext(ctx,
		`DELETE FROM editorial_votes WHERE editorial_id = $1 AND user_id = $2`, id, userID); err != nil {
		return 0, err
	}

	var total int
	if err := tx.QueryRowContext(ctx,
		`UPDATE editorials
		 SET vote_count = (SELECT count(*) FROM editorial_votes WHERE editorial_id = $1)
		 WHERE id = $1 RETURNING vote_count`, id).Scan(&total); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}

// HasSolved reports whether the viewer ever solved the problem, which is what
// unlocks a solved-only editorial.
func (s *EditorialStore) HasSolved(ctx context.Context, problemID, userID string) (bool, error) {
	var solved bool
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM submissions
		   WHERE user_id = $1::uuid AND problem_id = $2::uuid
		     AND contest_id IS NULL AND status = 'Accepted')`,
		userID, problemID).Scan(&solved)
	return solved, err
}
