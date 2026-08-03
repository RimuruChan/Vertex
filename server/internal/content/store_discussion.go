package content

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

type DiscussionStore struct{ db *database.DB }

func NewDiscussionStore(db *database.DB) *DiscussionStore { return &DiscussionStore{db: db} }

// ListByProblem 列出某题的评论(含作者名,父评论在前)。
func (s *DiscussionStore) ListByProblem(ctx context.Context, problemID string) ([]DiscussionPost, error) {
	return s.list(ctx, "problem_id = $1", problemID)
}

// ListByEditorial 列出某题解的评论。
func (s *DiscussionStore) ListByEditorial(ctx context.Context, editorialID string) ([]DiscussionPost, error) {
	return s.list(ctx, "editorial_id = $1", editorialID)
}

func (s *DiscussionStore) list(ctx context.Context, where string, arg string) ([]DiscussionPost, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT d.id, d.problem_id, d.editorial_id, d.contest_id, d.author_id,
		        COALESCE(u.username, ''), d.content_md, d.parent_id, d.created_at
		 FROM discussion_posts d LEFT JOIN users u ON u.id = d.author_id
		 WHERE `+where+`
		 ORDER BY d.created_at ASC`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []DiscussionPost{}
	for rows.Next() {
		var p DiscussionPost
		if err := rows.Scan(&p.ID, &p.ProblemID, &p.EditorialID, &p.ContestID, &p.AuthorID,
			&p.AuthorName, &p.ContentMD, &p.ParentID, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// CreateProblemPost 发表题目评论(可回复)。
func (s *DiscussionStore) CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error) {
	var p DiscussionPost
	err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO discussion_posts (problem_id, author_id, content_md, parent_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, problem_id, editorial_id, contest_id, author_id, content_md, parent_id, created_at`,
		problemID, authorID, contentMD, parentID,
	).Scan(&p.ID, &p.ProblemID, &p.EditorialID, &p.ContestID, &p.AuthorID,
		&p.ContentMD, &p.ParentID, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateEditorialPost 发表题解评论。
func (s *DiscussionStore) CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string) (*DiscussionPost, error) {
	var p DiscussionPost
	err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO discussion_posts (editorial_id, author_id, content_md)
		 VALUES ($1, $2, $3)
		 RETURNING id, problem_id, editorial_id, contest_id, author_id, content_md, parent_id, created_at`,
		editorialID, authorID, contentMD,
	).Scan(&p.ID, &p.ProblemID, &p.EditorialID, &p.ContestID, &p.AuthorID,
		&p.ContentMD, &p.ParentID, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// IsPostOwner 判断某条评论是否为该作者(删除权限校验)。
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

// Delete 删除评论(作者或 admin)。
func (s *DiscussionStore) Delete(ctx context.Context, postID int64) error {
	_, err := s.db.Pool.ExecContext(ctx, `DELETE FROM discussion_posts WHERE id = $1`, postID)
	return err
}
