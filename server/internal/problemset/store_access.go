package problemset

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jmoiron/sqlx"
)

func accessError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, domain.ErrForbidden) {
		return ErrForbidden
	}
	return err
}

func rankRole(rank int) AccessRole {
	if rank == 2 {
		return AccessEditor
	}
	if rank == 1 {
		return AccessReader
	}
	return ""
}

// s is the set alias; $1 is the current viewer.
func grantRankSQL() string {
	return `COALESCE((SELECT max(CASE WHEN a.role='editor' THEN 2 ELSE 1 END) FROM problem_set_access a
	 WHERE a.set_id=s.id AND a.domain_id=s.domain_id AND (a.user_id=NULLIF($1::text,'')::uuid OR a.group_id IN (
	 SELECT group_id FROM domain_group_members WHERE domain_id=s.domain_id AND user_id=NULLIF($1::text,'')::uuid))),0)`
}

func (s *SetStore) lockAccess(ctx context.Context, tx *sqlx.Tx, id, viewerID string) (Access, error) {
	scope, err := domain.LockScope(ctx, tx, viewerID)
	if err != nil {
		return Access{}, accessError(err)
	}
	value := Access{Scope: scope}
	var rank int
	err = tx.QueryRowxContext(ctx, "SELECT id,owner_id,visibility FROM problem_sets WHERE id=$1 AND domain_id=$2 FOR UPDATE", id, scope.Domain.ID).
		Scan(&value.SetID, &value.OwnerID, &value.Visibility)
	if errors.Is(err, sql.ErrNoRows) {
		return Access{}, ErrNotFound
	}
	if err != nil {
		return Access{}, err
	}
	// Read grants after acquiring the parent lock, so a writer we waited on
	// cannot leave this transaction using its pre-revocation statement snapshot.
	if err := tx.QueryRowxContext(ctx, "SELECT "+grantRankSQL()+" FROM problem_sets s WHERE s.id=$2", viewerID, id).Scan(&rank); err != nil {
		return Access{}, err
	}
	value.Role = rankRole(rank)
	value.Permissions = EffectivePermissions(scope, value.OwnerID, value.Visibility, value.Role)
	if !value.Permissions.View {
		return Access{}, ErrNotFound
	}
	return value, nil
}

func audit(ctx context.Context, tx *sqlx.Tx, access Access, action, target string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)",
		access.Scope.Domain.ID, access.Scope.UserID, action, "problem-set:"+access.SetID+" "+target)
	return err
}
