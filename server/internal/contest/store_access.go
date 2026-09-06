package contest

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

func accessError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, domain.ErrForbidden) {
		return ErrForbidden
	}
	return err
}

func readAccess(ctx context.Context, q accessQueryer, scope domain.Scope, id string, lock bool) (Access, error) {
	value := Access{Scope: scope}
	query := "SELECT id,owner_id,visibility,admission,begin_at,end_at,password_hash FROM contests WHERE id=$1 AND domain_id=$2"
	if lock {
		query += " FOR UPDATE"
	}
	err := q.QueryRowxContext(ctx, query, id, scope.Domain.ID).Scan(&value.ContestID, &value.OwnerID, &value.Visibility, &value.Admission, &value.BeginAt, &value.EndAt, &value.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Access{}, ErrNotFound
	}
	if err != nil {
		return Access{}, err
	}
	if scope.ActiveMember() {
		err = q.QueryRowxContext(ctx, `SELECT COALESCE(bool_or(role='editor'),false),COALESCE(bool_or(role='jury'),false),
		 COALESCE(bool_or(role='observer'),false),COALESCE(bool_or(role='participant'),false)
		 FROM contest_access a WHERE a.domain_id=$1 AND a.contest_id=$2 AND (a.user_id=$3 OR a.group_id IN (
		 SELECT group_id FROM domain_group_members WHERE domain_id=$1 AND user_id=$3))`, scope.Domain.ID, id, scope.UserID).
			Scan(&value.Grants.Editor, &value.Grants.Jury, &value.Grants.Observer, &value.Grants.Participant)
		if err != nil {
			return Access{}, err
		}
	}
	if scope.UserID != "" {
		err = q.QueryRowxContext(ctx, "SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id=$1 AND user_id=$2)", id, scope.UserID).Scan(&value.Registered)
		if err != nil {
			return Access{}, err
		}
	}
	value.Permissions = EffectivePermissions(scope, value.OwnerID, value.Visibility, value.Admission, value.Grants, value.Registered)
	return value, nil
}

func LoadAccess(ctx context.Context, db *sqlx.DB, id, userID string) (Access, error) {
	scope, err := domain.ResourceScope(ctx, db, userID)
	if err != nil {
		return Access{}, accessError(err)
	}
	return readAccess(ctx, db, scope, id, false)
}

func LockAccess(ctx context.Context, tx *sqlx.Tx, id, userID string) (Access, error) {
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return Access{}, accessError(err)
	}
	if err := domain.ResourceGuard(ctx, tx, "contest", id, true); err != nil {
		return Access{}, err
	}
	return readAccess(ctx, tx, scope, id, true)
}

func LockAuthorization(ctx context.Context, tx *sqlx.Tx, id, userID string) (Access, error) {
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return Access{}, accessError(err)
	}
	if err := domain.ResourceGuard(ctx, tx, "contest", id, false); err != nil {
		return Access{}, err
	}
	return readAccess(ctx, tx, scope, id, false)
}

func (s *ContestStore) Access(ctx context.Context, id, userID string) (Access, error) {
	return LoadAccess(ctx, s.db.Pool, id, userID)
}
