package postgres

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres/internal/dbgen"

	"github.com/jmoiron/sqlx"
)

func resourceScope(ctx context.Context, q dbgen.DBTX, actor subject) (tenancydomain.Scope, error) {
	row, err := dbgen.New(q).GetDomainScopeByID(ctx, dbgen.GetDomainScopeByIDParams{ViewerID: actor.ID, DomainID: tenancydomain.ID(ctx)})
	if err != nil {
		return tenancydomain.Scope{}, notFound(err)
	}
	value, err := scopeFromRow(row, actor)
	if err != nil {
		return tenancydomain.Scope{}, err
	}
	if !value.CanEnter() {
		return tenancydomain.Scope{}, tenancydomain.ErrNotFound
	}
	return value, nil
}

// ResourceScope reloads the account and membership. A cached request scope
// supplies only the domain identity; its roles are not authorization evidence.
func ResourceScope(ctx context.Context, db *sqlx.DB, userID string) (tenancydomain.Scope, error) {
	actor, err := loadSubject(ctx, db, userID, false)
	if err != nil {
		return tenancydomain.Scope{}, err
	}
	return resourceScope(ctx, db, actor)
}

// LockScope stabilizes account, membership, roles and group membership for a
// resource mutation. Domain governance takes an exclusive lock on the same
// row. Callers acquire this account/domain lock before their resource lock.
func LockScope(ctx context.Context, tx *sqlx.Tx, userID string) (tenancydomain.Scope, error) {
	scopes, err := LockScopes(ctx, tx, userID, tenancydomain.ID(ctx))
	return scopes[tenancydomain.ID(ctx)], err
}

// LockScopes acquires account -> sorted domain locks before cross-domain work.
// Returned policies are fresh; the context's cached roles are never trusted.
func LockScopes(ctx context.Context, tx *sqlx.Tx, userID string, ids ...string) (map[string]tenancydomain.Scope, error) {
	if userID == "" {
		return nil, tenancydomain.ErrUnauthenticated
	}
	if bound, ok := tenancydomain.FromContext(ctx); ok && bound.UserID != userID {
		return nil, tenancydomain.ErrForbidden
	}
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return nil, err
	}
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	result := make(map[string]tenancydomain.Scope, len(ordered))
	for _, id := range slices.Compact(ordered) {
		_, err = dbgen.New(tx).LockDomainForResourceWrite(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tenancydomain.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		scope, err := resourceScope(tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: id}, UserID: userID}), tx, actor)
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
	queries := dbgen.New(tx)
	key := "resource-authorization:" + kind + ":" + id
	if exclusive {
		return queries.LockResourceExclusive(ctx, key)
	}
	return queries.LockResourceShared(ctx, key)
}
