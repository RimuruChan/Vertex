package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/database"
	domain "github.com/RimuruChan/Vertex/server/internal/problemset/domain"
	"github.com/RimuruChan/Vertex/server/internal/problemset/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

func (r *Repository) Grants(ctx context.Context, id string) ([]domain.AccessGrant, error) {
	item, err := r.Get(ctx, id, tenancy.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !item.Permissions.ViewAccess {
		return nil, domain.ErrForbidden
	}
	rows, err := r.queries.ListProblemSetGrants(ctx, dbgen.ListProblemSetGrantsParams{SetID: id, DomainID: tenancy.ID(ctx)})
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
	id, err := queries.FindProblemSetCollaborator(ctx, dbgen.FindProblemSetCollaboratorParams{DomainID: domainID, Username: username})
	if errors.Is(err, sql.ErrNoRows) {
		return "", &domain.ValidationError{Message: "target must be an active member of this domain"}
	}
	return id, err
}

func (r *Repository) SetGrant(ctx context.Context, id string, input domain.GrantInput) error {
	if (input.Username == "") == (input.Group == "") || (input.Role != domain.AccessReader && input.Role != domain.AccessEditor) {
		return domain.Invalid("select one user or group and a reader/editor role")
	}
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := r.lockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return domain.ErrForbidden
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
		err = queries.GrantSetUser(ctx, dbgen.GrantSetUserParams{DomainID: access.Scope.Domain.ID, SetID: id, SubjectID: target, Role: string(input.Role), ActorID: access.Scope.UserID})
	} else {
		target, err = queries.ResolveProblemSetGrantGroup(ctx, dbgen.ResolveProblemSetGrantGroupParams{DomainID: access.Scope.Domain.ID, GroupRef: input.Group})
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.ValidationError{Message: "group must belong to this domain"}
		}
		if err != nil {
			return err
		}
		subject = "group_id"
		err = queries.GrantSetGroup(ctx, dbgen.GrantSetGroupParams{DomainID: access.Scope.Domain.ID, SetID: id, SubjectID: target, Role: string(input.Role), ActorID: access.Scope.UserID})
	}
	if err != nil {
		return err
	}
	if err := audit(ctx, queries, access, "problem-set.access.grant", subject+":"+target+" role:"+string(input.Role)); err != nil {
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
	access, err := r.lockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return domain.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	affected, err := queries.DeleteProblemSetGrant(ctx, dbgen.DeleteProblemSetGrantParams{SetID: id, GrantID: grantID, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	if err := audit(ctx, queries, access, "problem-set.access.remove", strconv.FormatInt(grantID, 10)); err != nil {
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
	access, err := r.lockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Transfer {
		return domain.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	owner, err := activeMember(ctx, queries, access.Scope.Domain.ID, username)
	if err != nil {
		return err
	}
	if owner == access.OwnerID {
		return nil
	}
	if err := queries.TransferProblemSetOwner(ctx, dbgen.TransferProblemSetOwnerParams{SetID: id, OwnerID: owner}); err != nil {
		return err
	}
	if err := queries.DeleteProblemSetOwnerGrants(ctx, dbgen.DeleteProblemSetOwnerGrantsParams{SetID: id, UserID: owner}); err != nil {
		return err
	}
	if err := audit(ctx, queries, access, "problem-set.owner.transfer", owner); err != nil {
		return err
	}
	return tx.Commit()
}
