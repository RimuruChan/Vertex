package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

func (r *Queries) Grants(ctx context.Context, id string) ([]domain.AccessGrant, error) {
	access, err := r.Access(ctx, id, tenancy.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !access.Permissions.ReadPackage {
		return nil, tenancy.ErrForbidden
	}
	rows, err := r.queries.ListProblemGrants(ctx, dbgen.ListProblemGrantsParams{ProblemID: id, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return nil, err
	}
	result := make([]domain.AccessGrant, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.AccessGrant{ID: row.ID, UserID: row.UserID, Username: database.StringPtr(row.Username), GroupID: row.GroupID, GroupName: database.StringPtr(row.GroupName), Role: domain.AccessRole(row.Role)})
	}
	return result, nil
}

func activeMember(ctx context.Context, queries *dbgen.Queries, domainID, username string) (string, error) {
	id, err := queries.FindProblemCollaborator(ctx, dbgen.FindProblemCollaboratorParams{DomainID: domainID, Username: username})
	if errors.Is(err, sql.ErrNoRows) {
		return "", &domain.ValidationError{Message: "target must be an active member of this domain"}
	}
	return id, err
}

func recordAccessAudit(ctx context.Context, queries *dbgen.Queries, access domain.Access, action, target string) error {
	return queries.RecordProblemAccessAudit(ctx, dbgen.RecordProblemAccessAuditParams{DomainID: access.Scope.Domain.ID, ActorID: access.Scope.UserID, Action: action, Target: "problem:" + access.ProblemID + " " + target})
}

func (r *Repository) SetGrant(ctx context.Context, id string, input domain.GrantInput) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return tenancy.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	var target, subject string
	if input.Username != "" {
		target, err = activeMember(ctx, queries, access.Scope.Domain.ID, input.Username)
		if err != nil {
			return err
		}
		if target == access.OwnerID {
			return &domain.ValidationError{Message: "use ownership transfer to change the owner"}
		}
		subject = "user_id"
		err = queries.GrantProblemUser(ctx, dbgen.GrantProblemUserParams{DomainID: access.Scope.Domain.ID, ProblemID: id, SubjectID: target, Role: string(input.Role), ActorID: access.Scope.UserID})
	} else {
		target, err = queries.ResolveProblemGrantGroup(ctx, dbgen.ResolveProblemGrantGroupParams{DomainID: access.Scope.Domain.ID, GroupRef: input.Group})
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.ValidationError{Message: "group must belong to this domain"}
		}
		if err != nil {
			return err
		}
		subject = "group_id"
		err = queries.GrantProblemGroup(ctx, dbgen.GrantProblemGroupParams{DomainID: access.Scope.Domain.ID, ProblemID: id, SubjectID: target, Role: string(input.Role), ActorID: access.Scope.UserID})
	}
	if err != nil {
		return err
	}
	if err := recordAccessAudit(ctx, queries, access, "problem.access.grant", subject+":"+target+" role:"+string(input.Role)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return tenancy.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	affected, err := queries.DeleteProblemGrant(ctx, dbgen.DeleteProblemGrantParams{ProblemID: id, GrantID: grantID, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	if err := recordAccessAudit(ctx, queries, access, "problem.access.remove", strconv.FormatInt(grantID, 10)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) Transfer(ctx context.Context, id, username string) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Transfer {
		return tenancy.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	owner, err := activeMember(ctx, queries, access.Scope.Domain.ID, username)
	if err != nil {
		return err
	}
	if owner == access.OwnerID {
		return nil
	}
	if err := queries.TransferProblemOwner(ctx, dbgen.TransferProblemOwnerParams{ProblemID: id, OwnerID: owner, DomainID: access.Scope.Domain.ID}); err != nil {
		return err
	}
	if err := queries.DeleteProblemOwnerGrants(ctx, dbgen.DeleteProblemOwnerGrantsParams{ProblemID: id, UserID: owner}); err != nil {
		return err
	}
	if err := recordAccessAudit(ctx, queries, access, "problem.owner.transfer", owner); err != nil {
		return err
	}
	return tx.Commit()
}
