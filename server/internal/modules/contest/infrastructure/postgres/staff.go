package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (r *Repository) StaffRole(ctx context.Context, contestID, userID string) (string, error) {
	role, err := r.queries.GetContestStaffRole(ctx, dbgen.GetContestStaffRoleParams{ContestID: contestID, UserID: userID, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (r *Repository) ListStaff(ctx context.Context, contestID string) ([]domain.Staff, error) {
	rows, err := r.queries.ListContestStaff(ctx, dbgen.ListContestStaffParams{ContestID: contestID, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, err
	}
	items := make([]domain.Staff, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.Staff{ContestID: row.ContestID, UserID: row.UserID, Username: row.Username, Role: row.Role, CreatedAt: row.CreatedAt})
	}
	return items, nil
}

func activeMember(ctx context.Context, q *dbgen.Queries, domainID, username string) (string, error) {
	id, err := q.FindContestCollaborator(ctx, dbgen.FindContestCollaboratorParams{DomainID: domainID, Username: username})
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.Invalid("target must be an active member of this domain")
	}
	return id, err
}

func auditAccess(ctx context.Context, q *dbgen.Queries, access domain.Access, action, target string) error {
	return q.RecordContestAccessAudit(ctx, dbgen.RecordContestAccessAuditParams{DomainID: access.Scope.Domain.ID, ActorID: access.Scope.UserID, Action: action, Target: "contest:" + access.ContestID + " " + target})
}

func (r *Repository) AddStaff(ctx context.Context, contestID, username, role string) (*domain.Staff, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, contestID, tenancy.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !access.Permissions.ManageAccess {
		return nil, domain.ErrForbidden
	}
	q := r.queries.WithTx(tx.Tx)
	userID, err := activeMember(ctx, q, access.Scope.Domain.ID, username)
	if err != nil {
		return nil, err
	}
	if err := q.ClearContestStaffRoles(ctx, dbgen.ClearContestStaffRolesParams{ContestID: contestID, UserID: userID}); err != nil {
		return nil, err
	}
	row, err := q.GrantContestStaffRole(ctx, dbgen.GrantContestStaffRoleParams{DomainID: access.Scope.Domain.ID, ContestID: contestID, UserID: userID, Role: role, ActorID: access.Scope.UserID})
	if err != nil {
		return nil, err
	}
	if row.UserID == nil {
		return nil, errors.New("staff grant has no user identity")
	}
	if err := auditAccess(ctx, q, access, "contest.staff.set", userID+" role:"+role); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &domain.Staff{ContestID: row.ContestID, UserID: *row.UserID, Username: username, Role: row.Role, CreatedAt: row.CreatedAt}, nil
}

func (r *Repository) RemoveStaff(ctx context.Context, contestID, userID string) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, contestID, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return domain.ErrForbidden
	}
	q := r.queries.WithTx(tx.Tx)
	affected, err := q.RemoveContestStaff(ctx, dbgen.RemoveContestStaffParams{ContestID: contestID, UserID: userID, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	if err := auditAccess(ctx, q, access, "contest.staff.remove", userID); err != nil {
		return err
	}
	return tx.Commit()
}
