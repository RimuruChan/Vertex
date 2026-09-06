package problem

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jmoiron/sqlx"
)

type accessQueryer interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

// The alias and placeholder come from this package's fixed list query.
func grantRankSQL(viewer string) string {
	return `COALESCE((SELECT max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END)
	 FROM problem_access a WHERE a.problem_id=p.id AND a.domain_id=p.domain_id
	 AND (a.user_id=NULLIF(` + viewer + `::text,'')::uuid OR a.group_id IN (
	   SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(` + viewer + `::text,'')::uuid))),0)`
}

func accessError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func readAccess(ctx context.Context, q accessQueryer, scope domain.Scope, problemID string, lock bool) (Access, error) {
	value := Access{Scope: scope}
	query := "SELECT id,owner_id,visibility FROM problems WHERE id=$1 AND domain_id=$2"
	if lock {
		query += " FOR UPDATE"
	}
	err := q.QueryRowxContext(ctx, query, problemID, scope.Domain.ID).Scan(&value.ProblemID, &value.OwnerID, &value.Visibility)
	if errors.Is(err, sql.ErrNoRows) {
		return Access{}, ErrNotFound
	}
	if err != nil {
		return Access{}, err
	}
	if scope.ActiveMember() {
		var rank int
		err = q.QueryRowxContext(ctx, `SELECT COALESCE(max(CASE WHEN role='editor' THEN 2 ELSE 1 END),0)
		 FROM problem_access a WHERE a.domain_id=$1 AND a.problem_id=$2
		 AND (a.user_id=$3 OR a.group_id IN (
		   SELECT group_id FROM domain_group_members WHERE domain_id=$1 AND user_id=$3))`,
			scope.Domain.ID, problemID, scope.UserID).Scan(&rank)
		if err != nil {
			return Access{}, err
		}
		switch rank {
		case 1:
			value.Role = AccessReader
		case 2:
			value.Role = AccessEditor
		}
		if value.OwnerID == scope.UserID {
			value.Role = AccessOwner
		}
	}
	value.Permissions = EffectivePermissions(scope, value.OwnerID, value.Visibility, value.Role)
	return value, nil
}

// LoadAccess is shared with the authoring extension of the problem aggregate.
func LoadAccess(ctx context.Context, db *sqlx.DB, problemID, userID string) (Access, error) {
	scope, err := domain.ResourceScope(ctx, db, userID)
	if err != nil {
		return Access{}, accessError(err)
	}
	return readAccess(ctx, db, scope, problemID, false)
}

// LockAccess must run before child/package mutations in the same transaction.
func LockAccess(ctx context.Context, tx *sqlx.Tx, problemID, userID string) (Access, error) {
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return Access{}, accessError(err)
	}
	if err := domain.ResourceGuard(ctx, tx, "problem", problemID, true); err != nil {
		return Access{}, err
	}
	return readAccess(ctx, tx, scope, problemID, true)
}

func LockAuthorization(ctx context.Context, tx *sqlx.Tx, id, userID string) (Access, error) {
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return Access{}, accessError(err)
	}
	if err := domain.ResourceGuard(ctx, tx, "problem", id, false); err != nil {
		return Access{}, err
	}
	return readAccess(ctx, tx, scope, id, false)
}

func (s *ProblemStore) Access(ctx context.Context, id, userID string) (Access, error) {
	return LoadAccess(ctx, s.db.Pool, id, userID)
}
