package postgres

import (
	"context"
	"strings"

	contentdomain "github.com/RimuruChan/Vertex/server/internal/content/domain"
	"github.com/RimuruChan/Vertex/server/internal/content/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

func (s *EditorialRepository) Create(ctx context.Context, userID string, input contentdomain.EditorialInput) (*contentdomain.Editorial, error) {
	prepared, err := contentdomain.PrepareEditorial(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ProblemID) == "" {
		return nil, contentdomain.Invalid("a problem is required")
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	parent, err := problempg.LockAuthorization(ctx, tx, prepared.ProblemID, userID)
	if err != nil {
		return nil, accessError(err)
	}
	if !parent.Permissions.View {
		return nil, contentdomain.ErrNotFound
	}
	if !parent.Scope.Allows(tenancydomain.CreateContent) {
		return nil, contentdomain.ErrForbidden
	}
	id, err := s.queries.WithTx(tx.Tx).CreateEditorial(ctx, dbgen.CreateEditorialParams{DomainID: tenancydomain.ID(ctx), ProblemID: prepared.ProblemID, UserID: userID, Title: prepared.Title, Body: prepared.ContentMD, Visibility: prepared.Visibility, Status: prepared.Status, SolvedOnly: prepared.SolvedOnly})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *EditorialRepository) Update(ctx context.Context, id, userID string, input contentdomain.EditorialInput) (*contentdomain.Editorial, error) {
	prepared, err := contentdomain.PrepareEditorial(input)
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
		return nil, contentdomain.ErrForbidden
	}
	if err := s.queries.WithTx(tx.Tx).UpdateEditorial(ctx, dbgen.UpdateEditorialParams{EditorialID: id, Title: prepared.Title, Body: prepared.ContentMD, Visibility: prepared.Visibility, Status: prepared.Status, SolvedOnly: prepared.SolvedOnly}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *EditorialRepository) Delete(ctx context.Context, id, userID string) error {
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
		return contentdomain.ErrForbidden
	}
	if err := s.queries.WithTx(tx.Tx).DeleteEditorial(ctx, id); err != nil {
		return err
	}
	if err := auditContent(ctx, tx, userID, "editorial.delete", id); err != nil {
		return err
	}
	return tx.Commit()
}

// A locked editorial serializes concurrent vote changes and visibility changes.
func (s *EditorialRepository) Vote(ctx context.Context, id, userID string, up bool) (int, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	item, err := lockEditorial(ctx, tx, id, userID)
	if err != nil {
		return 0, err
	}
	if item.Status != contentdomain.StatusPublished {
		return 0, contentdomain.ErrNotFound
	}
	if item.Locked {
		return 0, contentdomain.ErrSpoilerLocked
	}
	if !item.Permissions.Vote {
		return 0, contentdomain.ErrForbidden
	}
	if up {
		err = s.queries.WithTx(tx.Tx).AddEditorialVote(ctx, dbgen.AddEditorialVoteParams{EditorialID: id, UserID: userID})
	} else {
		err = s.queries.WithTx(tx.Tx).RemoveEditorialVote(ctx, dbgen.RemoveEditorialVoteParams{EditorialID: id, UserID: userID})
	}
	if err != nil {
		return 0, err
	}
	total, err := s.queries.WithTx(tx.Tx).RecountEditorialVotes(ctx, id)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}
