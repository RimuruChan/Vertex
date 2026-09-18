package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	reference "github.com/RimuruChan/Vertex/server/internal/shared/resourceid"
)

// ResolveNumber belongs to the resource owner. Resolution never grants access.
func (r *Queries) ResolveNumber(ctx context.Context, ref string) (string, error) {
	scope, err := tenancy.RequireScope(ctx)
	if err != nil {
		return "", err
	}
	number, err := reference.ParseNumber(ref)
	if err != nil {
		return "", err
	}
	id, err := r.queries.ResolveContestNumber(ctx, dbgen.ResolveContestNumberParams{DomainID: scope.Domain.ID, PublicID: int64(number)})
	if errors.Is(err, sql.ErrNoRows) {
		return "", reference.ErrNotFound
	}
	return id, err
}
