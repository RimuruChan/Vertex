package content

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

type EditorialStore struct{ db *database.DB }

func NewEditorialStore(db *database.DB) *EditorialStore { return &EditorialStore{db: db} }

// ListByProblem 列出某题已发布的题解。
func (s *EditorialStore) ListByProblem(ctx context.Context, problemID string) ([]Editorial, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT e.id, e.problem_id, e.author_id, COALESCE(u.username, ''), e.title,
		        e.content_md, e.visibility, e.status, e.created_at, e.updated_at
		 FROM editorials e LEFT JOIN users u ON u.id = e.author_id
		 WHERE e.problem_id = $1 AND e.status = 'published'
		 ORDER BY e.created_at DESC`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Editorial{}
	for rows.Next() {
		var e Editorial
		if err := rows.Scan(&e.ID, &e.ProblemID, &e.AuthorID, &e.AuthorName, &e.Title,
			&e.ContentMD, &e.Visibility, &e.Status, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// Get 取单篇题解。
func (s *EditorialStore) Get(ctx context.Context, id string) (*Editorial, error) {
	var e Editorial
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT e.id, e.problem_id, e.author_id, COALESCE(u.username, ''), e.title,
		        e.content_md, e.visibility, e.status, e.created_at, e.updated_at
		 FROM editorials e LEFT JOIN users u ON u.id = e.author_id
		 WHERE e.id = $1`, id,
	).Scan(&e.ID, &e.ProblemID, &e.AuthorID, &e.AuthorName, &e.Title,
		&e.ContentMD, &e.Visibility, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Create 发布题解。
func (s *EditorialStore) Create(ctx context.Context, problemID, authorID, title, contentMD string) (*Editorial, error) {
	var e Editorial
	err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO editorials (problem_id, author_id, title, content_md)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, problem_id, author_id, title, content_md, visibility, status, created_at, updated_at`,
		problemID, authorID, title, contentMD,
	).Scan(&e.ID, &e.ProblemID, &e.AuthorID, &e.Title, &e.ContentMD,
		&e.Visibility, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
