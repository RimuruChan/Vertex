package content

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/jmoiron/sqlx"
)

type AccessStore struct{ db *database.DB }

func NewAccessStore(db *database.DB) *AccessStore { return &AccessStore{db: db} }

// The administrator hint is retained only for existing callers; rights are
// resolved from the database and the problem's current grants.
func (s *AccessStore) CanViewProblem(ctx context.Context, problemID, userID string, _ bool) (bool, error) {
	access, err := problem.LoadAccess(ctx, s.db.Pool, problemID, userID)
	if errors.Is(err, problem.ErrNotFound) {
		return false, nil
	}
	return access.Permissions.View, err
}

func accessError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, problem.ErrNotFound) || errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, domain.ErrForbidden) {
		return ErrForbidden
	}
	return err
}

func readArgs(scope domain.Scope) []any {
	return []any{scope.UserID, contentManager(scope), scope.ActiveMember(), scope.Domain.ID}
}

type rowReader interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

func lockEditorial(ctx context.Context, tx *sqlx.Tx, id, userID string) (*Editorial, error) {
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return nil, accessError(err)
	}
	var problemID string
	if err := tx.GetContext(ctx, &problemID, "SELECT problem_id FROM editorials WHERE id=$1 AND domain_id=$2", id, scope.Domain.ID); err != nil {
		return nil, accessError(err)
	}
	// Resource authorization is stable without occupying worker-owned statistics rows.
	if _, err := problem.LockAuthorization(ctx, tx, problemID, userID); err != nil {
		return nil, accessError(err)
	}
	return readEditorial(ctx, tx, scope, id, true)
}

func auditContent(ctx context.Context, tx *sqlx.Tx, userID, action, target string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)", domain.ID(ctx), userID, action, target)
	return err
}
