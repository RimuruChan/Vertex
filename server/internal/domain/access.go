package domain

import (
	"context"
	"database/sql"
	"errors"
	"slices"

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
	scopes, err := LockScopes(ctx, tx, userID, ID(ctx))
	return scopes[ID(ctx)], err
}

// LockScopes acquires account -> sorted domain locks before cross-domain work.
// Returned policies are fresh; the context's cached roles are never trusted.
func LockScopes(ctx context.Context, tx *sqlx.Tx, userID string, ids ...string) (map[string]Scope, error) {
	if userID == "" {
		return nil, ErrUnauthenticated
	}
	if bound, ok := FromContext(ctx); ok && bound.UserID != userID {
		return nil, ErrForbidden
	}
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return nil, err
	}
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	result := make(map[string]Scope, len(ordered))
	for _, id := range slices.Compact(ordered) {
		var present int
		err = tx.QueryRowxContext(ctx, "SELECT 1 FROM domains WHERE id=$1 FOR SHARE", id).Scan(&present)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		scope, err := resourceScope(WithScope(ctx, Scope{Domain: Domain{ID: id}, UserID: userID}), tx, actor)
		if err != nil {
			return nil, err
		}
		result[id] = scope
	}
	return result, nil
}

// ResourceGuard separates authorization mutations from worker-owned counters.
// Rejudging takes a shared guard before queue locks; owner/grant edits take an
// exclusive guard. Holding a problem row here would invert worker lock order.
func ResourceGuard(ctx context.Context, tx *sqlx.Tx, kind, id string, exclusive bool) error {
	query := "SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))"
	if exclusive {
		query = "SELECT pg_advisory_xact_lock(hashtextextended($1,0))"
	}
	_, err := tx.ExecContext(ctx, query, "resource-authorization:"+kind+":"+id)
	return err
}
