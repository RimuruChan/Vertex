package postgres

import (
	"context"
	"database/sql"
	"errors"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"

	"github.com/jmoiron/sqlx"
)

func accessError(err error) error {
	if errors.Is(err, tenancydomain.ErrNotFound) {
		return problemdomain.ErrNotFound
	}
	return err
}

func rankRole(rank int) problemdomain.AccessRole {
	switch rank {
	case 1:
		return problemdomain.AccessReader
	case 2:
		return problemdomain.AccessEditor
	}
	return ""
}

func readAccess(ctx context.Context, db dbgen.DBTX, scope tenancydomain.Scope, problemID string, lock bool) (problemdomain.Access, error) {
	queries := dbgen.New(db)
	args := dbgen.GetProblemAccessParams{ProblemID: problemID, DomainID: scope.Domain.ID}
	var row dbgen.GetProblemAccessRow
	var err error
	if lock {
		locked, e := queries.LockProblemAccess(ctx, dbgen.LockProblemAccessParams(args))
		row, err = dbgen.GetProblemAccessRow(locked), e
	} else {
		row, err = queries.GetProblemAccess(ctx, args)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return problemdomain.Access{}, problemdomain.ErrNotFound
	}
	if err != nil {
		return problemdomain.Access{}, err
	}
	value := problemdomain.Access{Scope: scope, ProblemID: row.ID, OwnerID: row.OwnerID, Visibility: row.Visibility, PublishedVersion: row.PublishedVersion}
	if scope.ActiveMember() {
		rank, err := queries.GetProblemGrantRank(ctx, dbgen.GetProblemGrantRankParams{DomainID: scope.Domain.ID, ProblemID: problemID, ViewerID: scope.UserID})
		if err != nil {
			return problemdomain.Access{}, err
		}
		value.Role = rankRole(rank)
		if value.OwnerID == scope.UserID {
			value.Role = problemdomain.AccessOwner
		}
	}
	value.Permissions = problemdomain.EffectivePermissions(scope, value.OwnerID, value.Visibility, value.Role)
	if value.PublishedVersion == 0 && !value.Permissions.ReadPackage {
		value.Permissions.View = false
	}
	return value, nil
}

// LoadAccess is shared with the authoring extension of the problem aggregate.
func LoadAccess(ctx context.Context, db *sqlx.DB, problemID, userID string) (problemdomain.Access, error) {
	scope, err := tenancypg.ResourceScope(ctx, db, userID)
	if err != nil {
		return problemdomain.Access{}, accessError(err)
	}
	return readAccess(ctx, db, scope, problemID, false)
}

// LockAccess must run before child/package mutations in the same transaction.
func LockAccess(ctx context.Context, tx *sqlx.Tx, problemID, userID string) (problemdomain.Access, error) {
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return problemdomain.Access{}, accessError(err)
	}
	// Taxonomy governance updates mutable workspace labels in one transaction.
	// Take its shared guard before the problem guard so publication cannot race it.
	if err := tenancypg.ResourceGuard(ctx, tx, "tag-catalog", scope.Domain.ID, false); err != nil {
		return problemdomain.Access{}, err
	}
	if err := tenancypg.ResourceGuard(ctx, tx, "problem", problemID, true); err != nil {
		return problemdomain.Access{}, err
	}
	return readAccess(ctx, tx, scope, problemID, true)
}

func LockAuthorization(ctx context.Context, tx *sqlx.Tx, id, userID string) (problemdomain.Access, error) {
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return problemdomain.Access{}, accessError(err)
	}
	if err := tenancypg.ResourceGuard(ctx, tx, "problem", id, false); err != nil {
		return problemdomain.Access{}, err
	}
	return readAccess(ctx, tx, scope, id, false)
}

func (s *Queries) Access(ctx context.Context, id, userID string) (problemdomain.Access, error) {
	return LoadAccess(ctx, s.db.Pool, id, userID)
}
