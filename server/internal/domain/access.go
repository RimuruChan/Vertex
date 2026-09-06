package domain

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// ActorID identifies the authenticated caller bound by domain middleware.
// No scope means no authenticated actor, never an implicit administrator.
func ActorID(ctx context.Context) string {
	scope, _ := FromContext(ctx)
	return scope.UserID
}

func resourceScope(ctx context.Context, q queryer, actor subject) (Scope, error) {
	value, err := scanScope(q.QueryRowxContext(ctx,
		"SELECT "+scopeColumns+" "+scopeJoins+" WHERE d.id=$2", actor.ID, ID(ctx)), actor)
	if err != nil {
		return Scope{}, err
	}
	if !value.CanEnter() {
		return Scope{}, ErrNotFound
	}
	return value, nil
}

// ResourceScope reloads the account and membership. A cached request scope
// supplies only the domain identity; its roles are not authorization evidence.
func ResourceScope(ctx context.Context, db *sqlx.DB, userID string) (Scope, error) {
	actor, err := loadSubject(ctx, db, userID, false)
	if err != nil {
		return Scope{}, err
	}
	return resourceScope(ctx, db, actor)
}

// LockScope stabilizes account, membership, roles and group membership for a
// resource mutation. Domain governance takes an exclusive lock on the same
// row. Callers acquire this account/domain lock before their resource lock.
func LockScope(ctx context.Context, tx *sqlx.Tx, userID string) (Scope, error) {
	if userID == "" {
		return Scope{}, ErrUnauthenticated
	}
	if bound, ok := FromContext(ctx); ok && bound.UserID != userID {
		return Scope{}, ErrForbidden
	}
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return Scope{}, err
	}
	var present int
	err = tx.QueryRowxContext(ctx, "SELECT 1 FROM domains WHERE id=$1 FOR SHARE", ID(ctx)).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, ErrNotFound
	}
	if err != nil {
		return Scope{}, err
	}
	return resourceScope(ctx, tx, actor)
}
