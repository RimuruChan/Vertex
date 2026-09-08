package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/database"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

func (s *Repository) Grants(ctx context.Context, id string) ([]contestdomain.AccessGrant, error) {
	access, err := s.Access(ctx, id, tenancydomain.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !access.Permissions.PreviewProblems {
		return nil, contestdomain.ErrForbidden
	}
	rows, err := s.queries.ListContestAccessGrants(ctx, dbgen.ListContestAccessGrantsParams{ContestID: id, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return nil, err
	}
	result := make([]contestdomain.AccessGrant, 0, len(rows))
	for _, row := range rows {
		result = append(result, contestdomain.AccessGrant{ID: row.ID, UserID: row.UserID, Username: database.StringPtr(row.Username), GroupID: row.GroupID, GroupName: database.StringPtr(row.GroupName), Role: row.Role})
	}
	return result, nil
}

func (s *Repository) SetGrant(ctx context.Context, id string, input contestdomain.GrantInput) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return contestdomain.ErrForbidden
	}
	var target, column string
	if input.Username != "" {
		target, err = activeMember(ctx, s.queries.WithTx(tx.Tx), access.Scope.Domain.ID, input.Username)
		if err != nil {
			return err
		}
		if target == access.OwnerID {
			return &contestdomain.ValidationError{Message: "use ownership transfer to change the owner"}
		}
		column = "user_id"
	} else {
		target, err = s.queries.WithTx(tx.Tx).ResolveContestGrantGroup(ctx, dbgen.ResolveContestGrantGroupParams{DomainID: access.Scope.Domain.ID, GroupRef: input.Group})
		if errors.Is(err, sql.ErrNoRows) {
			return &contestdomain.ValidationError{Message: "group must belong to this domain"}
		}
		if err != nil {
			return err
		}
		column = "group_id"
	}
	if err := persistGrant(ctx, tx, access, target, input.Role, column == "group_id"); err != nil {
		return err
	}

	if err := auditAccess(ctx, s.queries.WithTx(tx.Tx), access, "contest.access.grant", column+":"+target+" role:"+string(input.Role)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return contestdomain.ErrForbidden
	}
	affected, err := s.queries.WithTx(tx.Tx).DeleteContestGrant(ctx, dbgen.DeleteContestGrantParams{ContestID: id, GrantID: grantID, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return contestdomain.ErrNotFound
	}
	if err := auditAccess(ctx, s.queries.WithTx(tx.Tx), access, "contest.access.remove", fmt.Sprint(grantID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) Transfer(ctx context.Context, id, username string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Transfer {
		return contestdomain.ErrForbidden
	}
	owner, err := activeMember(ctx, s.queries.WithTx(tx.Tx), access.Scope.Domain.ID, username)
	if err != nil {
		return err
	}
	if owner == access.OwnerID {
		return nil
	}
	if err := s.queries.WithTx(tx.Tx).TransferContestOwner(ctx, dbgen.TransferContestOwnerParams{ContestID: id, OwnerID: owner, DomainID: access.Scope.Domain.ID}); err != nil {
		return err
	}
	if err := s.queries.WithTx(tx.Tx).DeleteContestOwnerGrants(ctx, dbgen.DeleteContestOwnerGrantsParams{ContestID: id, UserID: owner}); err != nil {
		return err
	}
	if err := auditAccess(ctx, s.queries.WithTx(tx.Tx), access, "contest.owner.transfer", owner); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) Delete(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Delete {
		return contestdomain.ErrForbidden
	}
	hasActivity, err := s.queries.WithTx(tx.Tx).HasContestReferences(ctx, id)
	if err != nil {
		return err
	}

	if hasActivity {
		return contestdomain.Invalid("contest has participation history; change visibility instead of deleting")
	}
	if err := auditAccess(ctx, s.queries.WithTx(tx.Tx), access, "contest.delete", ""); err != nil {
		return err
	}
	if err := s.queries.WithTx(tx.Tx).DeleteContest(ctx, dbgen.DeleteContestParams{ContestID: id, DomainID: access.Scope.Domain.ID}); err != nil {
		return err
	}
	return tx.Commit()
}
