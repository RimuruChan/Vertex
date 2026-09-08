package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/problemset/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/problemset/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/jmoiron/sqlx"
)

func accessError(err error) error {
	if errors.Is(err, tenancy.ErrNotFound) {
		return domain.ErrNotFound
	}
	if errors.Is(err, tenancy.ErrForbidden) {
		return domain.ErrForbidden
	}
	return err
}
func rankRole(rank int) domain.AccessRole {
	switch rank {
	case 1:
		return domain.AccessReader
	case 2:
		return domain.AccessEditor
	}
	return ""
}

func (r *Repository) lockAccess(ctx context.Context, tx *sqlx.Tx, id, viewerID string) (domain.Access, error) {
	scope, err := tenancypg.LockScope(ctx, tx, viewerID)
	if err != nil {
		return domain.Access{}, accessError(err)
	}
	queries := r.queries.WithTx(tx.Tx)
	row, err := queries.LockProblemSetAccess(ctx, dbgen.LockProblemSetAccessParams{SetID: id, DomainID: scope.Domain.ID})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Access{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Access{}, err
	}
	// Acquire the parent first, then read grants with a new statement snapshot.
	rank, err := queries.GetSetGrantRank(ctx, dbgen.GetSetGrantRankParams{SetID: id, DomainID: scope.Domain.ID, ViewerID: scope.UserID})
	if err != nil {
		return domain.Access{}, err
	}
	access := domain.Access{Scope: scope, SetID: row.ID, OwnerID: row.OwnerID, Visibility: row.Visibility, Role: rankRole(rank)}
	access.Permissions = domain.EffectivePermissions(scope, access.OwnerID, access.Visibility, access.Role)
	if !access.Permissions.View {
		return domain.Access{}, domain.ErrNotFound
	}
	return access, nil
}

func audit(ctx context.Context, queries *dbgen.Queries, access domain.Access, action, target string) error {
	return queries.RecordProblemSetAudit(ctx, dbgen.RecordProblemSetAuditParams{DomainID: access.Scope.Domain.ID, ActorID: access.Scope.UserID, Action: action, Target: "problem-set:" + access.SetID + " " + target})
}
