package contest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jmoiron/sqlx"
)

func (s *ContestStore) Grants(ctx context.Context, id string) ([]AccessGrant, error) {
	access, err := s.Access(ctx, id, domain.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !access.Permissions.PreviewProblems {
		return nil, ErrForbidden
	}
	rows, err := s.db.Pool.QueryxContext(ctx, `SELECT a.id,a.user_id,u.username,a.group_id,g.name,a.role
	 FROM contest_access a LEFT JOIN users u ON u.id=a.user_id LEFT JOIN domain_groups g ON g.id=a.group_id
	 WHERE a.contest_id=$1 AND a.domain_id=$2 ORDER BY a.id`, id, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AccessGrant{}
	for rows.Next() {
		var grant AccessGrant
		if err := rows.Scan(&grant.ID, &grant.UserID, &grant.Username, &grant.GroupID, &grant.GroupName, &grant.Role); err != nil {
			return nil, err
		}
		result = append(result, grant)
	}
	return result, rows.Err()
}

func activeMember(ctx context.Context, tx *sqlx.Tx, domainID, username string) (string, error) {
	var id string
	err := tx.QueryRowxContext(ctx, `SELECT m.user_id FROM domain_members m JOIN users u ON u.id=m.user_id
	 WHERE m.domain_id=$1 AND u.username=$2 AND m.status='active' AND u.disabled_at IS NULL
	 FOR SHARE OF u`, domainID, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &ValidationError{Message: "target must be an active member of this domain"}
	}
	return id, err
}

func recordAccessAudit(ctx context.Context, tx *sqlx.Tx, access Access, action, target string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)`,
		access.Scope.Domain.ID, access.Scope.UserID, action, "contest:"+access.ContestID+" "+target)
	return err
}

func (s *ContestStore) SetGrant(ctx context.Context, id string, input GrantInput) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return ErrForbidden
	}
	var target, column string
	if input.Username != "" {
		target, err = activeMember(ctx, tx, access.Scope.Domain.ID, input.Username)
		if err != nil {
			return err
		}
		if target == access.OwnerID {
			return &ValidationError{Message: "use ownership transfer to change the owner"}
		}
		column = "user_id"
	} else {
		err = tx.QueryRowxContext(ctx, `SELECT id FROM domain_groups WHERE domain_id=$1 AND (id::text=$2 OR public_id::text=$2)`, access.Scope.Domain.ID, input.Group).Scan(&target)
		if errors.Is(err, sql.ErrNoRows) {
			return &ValidationError{Message: "group must belong to this domain"}
		}
		if err != nil {
			return err
		}
		column = "group_id"
	}
	// column is selected by the two branches above, never supplied as SQL.
	query := fmt.Sprintf(`INSERT INTO contest_access(domain_id,contest_id,%s,role,granted_by) VALUES($1,$2,$3,$4,$5)
	 ON CONFLICT(contest_id,%s,role) WHERE %s IS NOT NULL DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by`, column, column, column)
	if _, err := tx.ExecContext(ctx, query, access.Scope.Domain.ID, id, target, input.Role, access.Scope.UserID); err != nil {
		return err
	}
	if err := recordAccessAudit(ctx, tx, access, "contest.access.grant", column+":"+target+" role:"+string(input.Role)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ContestStore) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return ErrForbidden
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM contest_access WHERE contest_id=$1 AND id=$2 AND domain_id=$3", id, grantID, access.Scope.Domain.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	if err := recordAccessAudit(ctx, tx, access, "contest.access.remove", fmt.Sprint(grantID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ContestStore) Transfer(ctx context.Context, id, username string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Transfer {
		return ErrForbidden
	}
	owner, err := activeMember(ctx, tx, access.Scope.Domain.ID, username)
	if err != nil {
		return err
	}
	if owner == access.OwnerID {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE contests SET owner_id=$2 WHERE id=$1 AND domain_id=$3", id, owner, access.Scope.Domain.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM contest_access WHERE contest_id=$1 AND user_id=$2", id, owner); err != nil {
		return err
	}
	if err := recordAccessAudit(ctx, tx, access, "contest.owner.transfer", owner); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ContestStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Delete {
		return ErrForbidden
	}
	var hasActivity bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id=$1)
	 OR EXISTS(SELECT 1 FROM submissions WHERE contest_id=$1)
	 OR EXISTS(SELECT 1 FROM clarifications WHERE contest_id=$1)
	 OR EXISTS(SELECT 1 FROM rejudgings WHERE contest_id=$1)`, id).Scan(&hasActivity); err != nil {
		return err
	}
	if hasActivity {
		return invalid("contest has participation history; change visibility instead of deleting")
	}
	if err := recordAccessAudit(ctx, tx, access, "contest.delete", ""); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM contests WHERE id=$1 AND domain_id=$2", id, access.Scope.Domain.ID); err != nil {
		return err
	}
	return tx.Commit()
}
