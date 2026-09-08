package postgres

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/modules/console/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"

	"github.com/jmoiron/sqlx"
)

type resourceQueryer = dbgen.DBTX

func (s *Repository) RequireResourceManagement(ctx context.Context, write bool) error {
	scope, err := tenancypg.ResourceScope(ctx, s.db.Pool, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !write {
		scope.Domain.Archived = false
	}
	if !scope.Allows(tenancydomain.ManageResources) {
		return tenancydomain.ErrForbidden
	}
	return nil
}

// Resource mutations recheck current membership under the governance row lock.
func (s *Repository) resourceTransaction(ctx context.Context) (*sqlx.Tx, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	scope, err := tenancypg.LockScope(ctx, tx, tenancydomain.ActorID(ctx))
	if err == nil && !scope.Allows(tenancydomain.ManageResources) {
		err = tenancydomain.ErrForbidden
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func recordResourceChange(ctx context.Context, tx *sqlx.Tx, action, target string) error {
	err := dbgen.New(tx).RecordResourceAudit(ctx, dbgen.RecordResourceAuditParams{DomainID: tenancydomain.ID(ctx), UserID: tenancydomain.ActorID(ctx), Action: action, Target: target})
	return err
}
