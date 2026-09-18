package postgres

import (
	"context"
	"database/sql"
	"errors"

	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres/internal/dbgen"
	reference "github.com/RimuruChan/Vertex/server/internal/shared/resourceid"
)

// ResolveNumber belongs to the resource owner. Resolution never grants access.
func (r *Repository) ResolveNumber(ctx context.Context, ref string) (string, error) {
	scope, err := tenancy.RequireScope(ctx)
	if err != nil {
		return "", err
	}
	number, err := reference.ParseNumber(ref)
	if err != nil {
		return "", err
	}
	id, err := r.queries.ResolveGroupNumber(ctx, dbgen.ResolveGroupNumberParams{DomainID: scope.Domain.ID, PublicID: int64(number)})
	if errors.Is(err, sql.ErrNoRows) {
		return "", reference.ErrNotFound
	}
	return id, err
}
