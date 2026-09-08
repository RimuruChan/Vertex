package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/publicid/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/publicid/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// Resolver maps a public number to a UUID within one domain. Resolution is
// not authorization; the resource's own policy must still check the caller.
type Resolver struct{ queries *dbgen.Queries }

func NewResolver(db *database.DB) *Resolver { return &Resolver{queries: dbgen.New(db.Pool.DB)} }

func (r *Resolver) Resolve(ctx context.Context, kind, ref string) (string, error) {
	number, err := strconv.ParseInt(ref, 10, 64)
	if err != nil || number <= 0 {
		return "", domain.ErrNotFound
	}
	domainID := tenancy.ID(ctx)
	var id string
	switch kind {
	case "problems":
		id, err = r.queries.ResolveProblemNumber(ctx, dbgen.ResolveProblemNumberParams{DomainID: domainID, PublicID: number})
	case "contests":
		id, err = r.queries.ResolveContestNumber(ctx, dbgen.ResolveContestNumberParams{DomainID: domainID, PublicID: number})
	case "submissions":
		id, err = r.queries.ResolveSubmissionNumber(ctx, dbgen.ResolveSubmissionNumberParams{DomainID: domainID, PublicID: number})
	case "editorials":
		id, err = r.queries.ResolveEditorialNumber(ctx, dbgen.ResolveEditorialNumberParams{DomainID: domainID, PublicID: number})
	case "problem-sets":
		id, err = r.queries.ResolveProblemSetNumber(ctx, dbgen.ResolveProblemSetNumberParams{DomainID: domainID, PublicID: number})
	case "announcements":
		id, err = r.queries.ResolveAnnouncementNumber(ctx, dbgen.ResolveAnnouncementNumberParams{DomainID: domainID, PublicID: number})
	default:
		return "", domain.ErrNotFound
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return id, err
}
