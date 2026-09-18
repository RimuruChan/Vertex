package references

import (
	"context"

	consolepg "github.com/RimuruChan/Vertex/server/internal/modules/console/infrastructure/postgres"
	contentpg "github.com/RimuruChan/Vertex/server/internal/modules/content/infrastructure/postgres"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	problemsetpg "github.com/RimuruChan/Vertex/server/internal/modules/problemset/infrastructure/postgres"
	submissionpg "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	reference "github.com/RimuruChan/Vertex/server/internal/shared/resourceid"
)

type Lookup func(context.Context, string) (string, error)
type Resolver struct{ lookups map[string]Lookup }

func NewResolver(db *database.DB) *Resolver {
	return &Resolver{lookups: map[string]Lookup{
		"problems":      problempg.NewQueries(db).ResolveNumber,
		"contests":      contestpg.NewQueries(db).ResolveNumber,
		"submissions":   submissionpg.NewQueries(db).ResolveNumber,
		"editorials":    contentpg.NewEditorialRepository(db).ResolveNumber,
		"problem-sets":  problemsetpg.NewRepository(db).ResolveNumber,
		"announcements": consolepg.NewRepository(db).ResolveNumber,
		"groups":        tenancypg.NewRepository(db).ResolveNumber,
	}}
}
func (r *Resolver) Resolve(ctx context.Context, kind, ref string) (string, error) {
	lookup, ok := r.lookups[kind]
	if !ok {
		return "", reference.ErrNotFound
	}
	return lookup(ctx, ref)
}
