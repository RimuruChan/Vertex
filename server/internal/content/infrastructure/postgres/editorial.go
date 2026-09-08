package postgres

import (
	"context"

	domain "github.com/RimuruChan/Vertex/server/internal/content/domain"
	"github.com/RimuruChan/Vertex/server/internal/content/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/database"
	problem "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
)

type EditorialRepository struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.EditorialRepository = (*EditorialRepository)(nil)

func NewEditorialRepository(db *database.DB) *EditorialRepository {
	return &EditorialRepository{db: db, queries: dbgen.New(db.Pool.DB)}
}
func editorialFromRow(row dbgen.GetVisibleEditorialRow, scope tenancy.Scope) domain.Editorial {
	item := domain.Editorial{ID: row.ID, PublicID: row.PublicID, ProblemID: row.ProblemID, ProblemPublicID: row.ProblemPublicID, ProblemTitle: row.ProblemTitle,
		AuthorID: row.AuthorID, AuthorName: row.AuthorName, Title: row.Title, Visibility: row.Visibility, Status: row.Status, SolvedOnly: row.SolvedOnly,
		VoteCount: row.VoteCount, Voted: row.Voted, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DomainID: row.DomainID, ContentMD: row.ContentMd}
	item.Permissions = domain.EditorialPermissions(problem.Access{Scope: scope, OwnerID: row.ProblemOwnerID, Permissions: problem.Permissions{View: true}}, item.AuthorID, item.Visibility, item.Status, item.SolvedOnly, row.Solved)
	item.Locked = item.Permissions.View && !item.Permissions.ViewBody
	return item
}
func (r *EditorialRepository) List(ctx context.Context, f domain.EditorialFilters) ([]domain.EditorialSummary, int, error) {
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, f.ViewerID)
	if err != nil {
		return nil, 0, accessError(err)
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	manager := domain.ContentManager(scope)
	total, err := r.queries.CountVisibleEditorials(ctx, dbgen.CountVisibleEditorialsParams{DomainID: scope.Domain.ID, ViewerID: scope.UserID, IsManager: manager, ActiveMember: scope.ActiveMember(), ProblemFilter: f.ProblemID, AuthorFilter: f.AuthorID, Keyword: f.Keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListVisibleEditorials(ctx, dbgen.ListVisibleEditorialsParams{DomainID: scope.Domain.ID, ViewerID: scope.UserID, IsManager: manager, ActiveMember: scope.ActiveMember(), ProblemFilter: f.ProblemID, AuthorFilter: f.AuthorID, Keyword: f.Keyword, SortVotes: f.Sort == "votes", PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.EditorialSummary, 0, len(rows))
	for _, row := range rows {
		item := editorialFromRow(dbgen.GetVisibleEditorialRow(row), scope)
		result = append(result, domain.EditorialSummary{ID: item.ID, PublicID: item.PublicID, ProblemID: item.ProblemID, ProblemPublicID: item.ProblemPublicID, ProblemTitle: item.ProblemTitle,
			AuthorID: item.AuthorID, AuthorName: item.AuthorName, Title: item.Title, Visibility: item.Visibility, Status: item.Status, SolvedOnly: item.SolvedOnly, VoteCount: item.VoteCount, Voted: item.Voted,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, DomainID: item.DomainID, Permissions: item.Permissions, Locked: item.Locked})
	}
	return result, int(total), nil
}
func readEditorial(ctx context.Context, db dbgen.DBTX, scope tenancy.Scope, id string, lock bool) (*domain.Editorial, error) {
	q := dbgen.New(db)
	args := dbgen.GetVisibleEditorialParams{DomainID: scope.Domain.ID, ViewerID: scope.UserID, IsManager: domain.ContentManager(scope), ActiveMember: scope.ActiveMember(), EditorialID: id}
	var row dbgen.GetVisibleEditorialRow
	var err error
	if lock {
		locked, e := q.LockVisibleEditorial(ctx, dbgen.LockVisibleEditorialParams(args))
		row, err = dbgen.GetVisibleEditorialRow(locked), e
	} else {
		row, err = q.GetVisibleEditorial(ctx, args)
	}
	if err != nil {
		return nil, accessError(err)
	}
	item := editorialFromRow(row, scope)
	return &item, nil
}
func (r *EditorialRepository) Get(ctx context.Context, id, viewerID string) (*domain.Editorial, error) {
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, viewerID)
	if err != nil {
		return nil, accessError(err)
	}
	return readEditorial(ctx, r.db.Pool, scope, id, false)
}
func (r *EditorialRepository) HasSolved(ctx context.Context, problemID, userID string) (bool, error) {
	return r.queries.HasSolvedProblem(ctx, dbgen.HasSolvedProblemParams{ViewerID: userID, ProblemID: problemID, DomainID: tenancy.ID(ctx)})
}
