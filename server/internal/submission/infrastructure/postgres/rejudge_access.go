package postgres

import (
	"context"

	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
	"github.com/jmoiron/sqlx"
)

// A problem owner can rejudge practice, not contests reusing that problem.
func lockRejudgeTarget(ctx context.Context, tx *sqlx.Tx, contestID, problemID, userID string) (bool, error) {
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if scope.Domain.Archived {
		return false, tenancy.ErrForbidden
	}
	if scope.Allows(tenancy.ManageResources) {
		return contestID == "" && problemID != "", nil
	}
	if contestID != "" {
		access, err := contestpg.LockAuthorization(ctx, tx, contestID, userID)
		if err != nil {
			return false, err
		}
		if !access.Permissions.Rejudge {
			return false, tenancy.ErrForbidden
		}
		return false, nil
	}
	if problemID != "" {
		access, err := problempg.LockAuthorization(ctx, tx, problemID, userID)
		if err != nil {
			return false, err
		}
		if access.Permissions.ManageAccess {
			return true, nil
		}
	}
	return false, tenancy.ErrForbidden
}

func lockRejudgeGeneration(ctx context.Context, tx *sqlx.Tx) error {
	return dbgen.New(tx).LockRejudgeGeneration(ctx, "rejudge-generation:"+tenancy.ID(ctx))
}
func auditRejudge(ctx context.Context, tx *sqlx.Tx, userID, action, target string) error {
	return dbgen.New(tx).RecordRejudgeAudit(ctx, dbgen.RecordRejudgeAuditParams{DomainID: tenancy.ID(ctx), ActorID: userID, Action: action, Target: target})
}
