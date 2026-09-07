package console

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jmoiron/sqlx"
)

type resourceQueryer interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

func (s *ConsoleStore) RequireResourceManagement(ctx context.Context, write bool) error {
	scope, err := domain.ResourceScope(ctx, s.db.Pool, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !write {
		scope.Domain.Archived = false
	}
	if !scope.Allows(domain.ManageResources) {
		return domain.ErrForbidden
	}
	return nil
}

// Resource mutations recheck current membership under the governance row lock.
func (s *ConsoleStore) resourceTransaction(ctx context.Context) (*sqlx.Tx, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	scope, err := domain.LockScope(ctx, tx, domain.ActorID(ctx))
	if err == nil && !scope.Allows(domain.ManageResources) {
		err = domain.ErrForbidden
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func recordResourceChange(ctx context.Context, tx *sqlx.Tx, action, target string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)`, domain.ID(ctx), domain.ActorID(ctx), action, target)
	return err
}
