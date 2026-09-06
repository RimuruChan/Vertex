package problemset

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jmoiron/sqlx"
)

func (s *SetStore) Grants(ctx context.Context, id string) ([]AccessGrant, error) {
	item, err := s.Get(ctx, id, domain.ActorID(ctx), false)
	if err != nil {
		return nil, err
	}
	if !item.Permissions.ViewAccess {
		return nil, ErrForbidden
	}
	rows, err := s.db.Pool.QueryxContext(ctx, `SELECT a.id,a.user_id,u.username,a.group_id,g.name,a.role
	 FROM problem_set_access a LEFT JOIN users u ON u.id=a.user_id LEFT JOIN domain_groups g ON g.id=a.group_id
	 WHERE a.domain_id=$1 AND a.set_id=$2 ORDER BY a.id`, domain.ID(ctx), id)
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
	 WHERE m.domain_id=$1 AND u.username=$2 AND m.status='active' AND u.disabled_at IS NULL FOR SHARE OF u`, domainID, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", invalid("target must be an active member of this domain")
	}
	return id, err
}

func (s *SetStore) SetGrant(ctx context.Context, id string, input GrantInput) error {
	if (input.Username == "") == (input.Group == "") || (input.Role != AccessReader && input.Role != AccessEditor) {
		return invalid("select one user or group and a reader/editor role")
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := s.lockAccess(ctx, tx, id, domain.ActorID(ctx))
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
			return invalid("use ownership transfer to change the owner")
		}
		column = "user_id"
	} else {
		err = tx.QueryRowxContext(ctx, "SELECT id FROM domain_groups WHERE domain_id=$1 AND (id::text=$2 OR public_id::text=$2)", access.Scope.Domain.ID, input.Group).Scan(&target)
		if errors.Is(err, sql.ErrNoRows) {
			return invalid("group must belong to this domain")
		}
		if err != nil {
			return err
		}
		column = "group_id"
	}
	// The column comes only from the fixed branches above.
	query := fmt.Sprintf(`INSERT INTO problem_set_access(domain_id,set_id,%s,role,granted_by) VALUES($1,$2,$3,$4,$5)
	 ON CONFLICT(set_id,%s) WHERE %s IS NOT NULL DO UPDATE SET role=EXCLUDED.role,granted_by=EXCLUDED.granted_by`, column, column, column)
	if _, err := tx.ExecContext(ctx, query, access.Scope.Domain.ID, id, target, input.Role, access.Scope.UserID); err != nil {
		return err
	}
	if err := audit(ctx, tx, access, "problem-set.access.grant", column+":"+target+" role:"+string(input.Role)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SetStore) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := s.lockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return ErrForbidden
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM problem_set_access WHERE set_id=$1 AND id=$2 AND domain_id=$3", id, grantID, access.Scope.Domain.ID)
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
	if err := audit(ctx, tx, access, "problem-set.access.remove", fmt.Sprint(grantID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SetStore) Transfer(ctx context.Context, id, username string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := s.lockAccess(ctx, tx, id, domain.ActorID(ctx))
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
	if _, err := tx.ExecContext(ctx, "UPDATE problem_sets SET owner_id=$2,updated_at=now() WHERE id=$1", id, owner); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM problem_set_access WHERE set_id=$1 AND user_id=$2", id, owner); err != nil {
		return err
	}
	if err := audit(ctx, tx, access, "problem-set.owner.transfer", owner); err != nil {
		return err
	}
	return tx.Commit()
}
