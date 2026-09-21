package postgres

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/jmoiron/sqlx"
)

type artifactStorageKey struct{}
type artifactStorageScope struct {
	problemID string
	tx        *sqlx.Tx
}

func (repo *BuildRepository) WithArtifactStorage(ctx context.Context, id string, fn func(context.Context) error) error {
	if scope, ok := ctx.Value(artifactStorageKey{}).(artifactStorageScope); ok && scope.problemID == id {
		return fn(ctx)
	}
	tx, err := repo.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tenancypg.LockContentStorage(ctx, tx, id); err != nil {
		return err
	}
	if err := fn(context.WithValue(ctx, artifactStorageKey{}, artifactStorageScope{id, tx})); err != nil {
		return err
	}
	return tx.Commit()
}
func artifactStorageQueries(ctx context.Context, id string) (*dbgen.Queries, bool) {
	scope, ok := ctx.Value(artifactStorageKey{}).(artifactStorageScope)
	if !ok || scope.problemID != id {
		return nil, false
	}
	return dbgen.New(scope.tx), true
}
