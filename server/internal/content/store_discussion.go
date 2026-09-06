package content

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

type DiscussionStore struct{ db *database.DB }

func NewDiscussionStore(db *database.DB) *DiscussionStore { return &DiscussionStore{db: db} }

const postColumns = `d.id, d.problem_id, d.editorial_id, d.contest_id, d.author_id,
	COALESCE(u.username, ''), d.content_md, d.parent_id, d.created_at, d.updated_at`

const postJoins = `FROM discussion_posts AS d LEFT JOIN users AS u ON u.id = d.author_id`

func scanPost(scanner interface{ Scan(...any) error }) (DiscussionPost, error) {
	var item DiscussionPost
	err := scanner.Scan(&item.ID, &item.ProblemID, &item.EditorialID, &item.ContestID,
		&item.AuthorID, &item.AuthorName, &item.ContentMD, &item.ParentID,
		&item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// ListByProblem returns a problem's comments, oldest first so replies read in
// the order they were written.
func (s *DiscussionStore) ListByProblem(ctx context.Context, problemID string) ([]DiscussionPost, error) {
	return s.list(ctx, "d.problem_id = $1", problemID)
}

func (s *DiscussionStore) ListByEditorial(ctx context.Context, editorialID string) ([]DiscussionPost, error) {
	return s.list(ctx, "d.editorial_id = $1", editorialID)
}

func (s *DiscussionStore) ListByContest(ctx context.Context, contestID string) ([]DiscussionPost, error) {
	return s.list(ctx, "d.contest_id = $1", contestID)
}

func (s *DiscussionStore) list(ctx context.Context, where, arg string) ([]DiscussionPost, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+postColumns+` `+postJoins+`
		 WHERE `+where+` ORDER BY d.created_at ASC`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []DiscussionPost{}
	for rows.Next() {
		item, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *DiscussionStore) Get(ctx context.Context, postID int64) (*DiscussionPost, error) {
	item, err := scanPost(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+postColumns+` `+postJoins+` WHERE d.id = $1`, postID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *DiscussionStore) CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error) {
	return s.create(ctx, "problem_id", problemID, authorID, contentMD, parentID)
}

func (s *DiscussionStore) CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string) (*DiscussionPost, error) {
	return s.create(ctx, "editorial_id", editorialID, authorID, contentMD, nil)
}

func (s *DiscussionStore) CreateContestPost(ctx context.Context, contestID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error) {
	return s.create(ctx, "contest_id", contestID, authorID, contentMD, parentID)
}

// create inserts into exactly one scope column. The column name comes from
// this package's own callers, never from a request.
func (s *DiscussionStore) create(
	ctx context.Context, scopeColumn, scopeID, authorID, contentMD string, parentID *int64,
) (*DiscussionPost, error) {
	var id int64
	if err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO discussion_posts (`+scopeColumn+`, author_id, content_md, parent_id)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		scopeID, authorID, contentMD, parentID).Scan(&id); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Update rewrites a comment and stamps updated_at, which is what the client
// renders as "edited".
func (s *DiscussionStore) Update(ctx context.Context, postID int64, contentMD string) (*DiscussionPost, error) {
	result, err := s.db.Pool.ExecContext(ctx,
		`UPDATE discussion_posts SET content_md = $2, updated_at = now() WHERE id = $1`,
		postID, contentMD)
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
	return s.Get(ctx, postID)
}

func (s *DiscussionStore) IsPostOwner(ctx context.Context, postID int64, userID string) (bool, error) {
	var ownerID *string
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT author_id FROM discussion_posts WHERE id = $1`, postID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ownerID != nil && *ownerID == userID, nil
}

// Delete removes a comment. Replies cascade through the parent_id foreign key,
// so a deleted thread does not leave orphaned answers behind.
func (s *DiscussionStore) Delete(ctx context.Context, postID int64) error {
	_, err := s.db.Pool.ExecContext(ctx, `DELETE FROM discussion_posts WHERE id = $1`, postID)
	return err
}
