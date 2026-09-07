package content

import (
	"context"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

func (s *EditorialStore) Create(ctx context.Context, userID string, input EditorialInput) (*Editorial, error) {
	prepared, err := prepareEditorial(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ProblemID) == "" {
		return nil, invalid("a problem is required")
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	parent, err := problem.LockAuthorization(ctx, tx, prepared.ProblemID, userID)
	if err != nil {
		return nil, accessError(err)
	}
	if !parent.Permissions.View {
		return nil, ErrNotFound
	}
	if !parent.Scope.Allows(domain.CreateContent) {
		return nil, ErrForbidden
	}
	var id string
	err = tx.QueryRowxContext(ctx, `INSERT INTO editorials(domain_id,problem_id,author_id,title,content_md,visibility,status,solved_only)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, domain.ID(ctx), prepared.ProblemID, userID, prepared.Title, prepared.ContentMD, prepared.Visibility, prepared.Status, prepared.SolvedOnly).Scan(&id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *EditorialStore) Update(ctx context.Context, id, userID string, input EditorialInput) (*Editorial, error) {
	prepared, err := prepareEditorial(input)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	item, err := lockEditorial(ctx, tx, id, userID)
	if err != nil {
		return nil, err
	}
	if !item.Permissions.Edit {
		return nil, ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, `UPDATE editorials SET title=$2,content_md=$3,visibility=$4,status=$5,solved_only=$6,updated_at=now() WHERE id=$1`,
		id, prepared.Title, prepared.ContentMD, prepared.Visibility, prepared.Status, prepared.SolvedOnly); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *EditorialStore) Delete(ctx context.Context, id, userID string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	item, err := lockEditorial(ctx, tx, id, userID)
	if err != nil {
		return err
	}
	if !item.Permissions.Delete {
		return ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM editorials WHERE id=$1", id); err != nil {
		return err
	}
	if err := auditContent(ctx, tx, userID, "editorial.delete", id); err != nil {
		return err
	}
	return tx.Commit()
}

// A locked editorial serializes concurrent vote changes and visibility changes.
func (s *EditorialStore) Vote(ctx context.Context, id, userID string, up bool) (int, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	item, err := lockEditorial(ctx, tx, id, userID)
	if err != nil {
		return 0, err
	}
	if item.Status != StatusPublished {
		return 0, ErrNotFound
	}
	if item.Locked {
		return 0, ErrSpoilerLocked
	}
	if !item.Permissions.Vote {
		return 0, ErrForbidden
	}
	if up {
		_, err = tx.ExecContext(ctx, "INSERT INTO editorial_votes(editorial_id,user_id) VALUES($1,$2) ON CONFLICT(editorial_id,user_id) DO NOTHING", id, userID)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM editorial_votes WHERE editorial_id=$1 AND user_id=$2", id, userID)
	}
	if err != nil {
		return 0, err
	}
	var total int
	if err := tx.QueryRowxContext(ctx, "UPDATE editorials SET vote_count=(SELECT count(*) FROM editorial_votes WHERE editorial_id=$1) WHERE id=$1 RETURNING vote_count", id).Scan(&total); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}
