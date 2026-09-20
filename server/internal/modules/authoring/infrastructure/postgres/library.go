package postgres

import (
	"context"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	problem "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
)

func (repo *RevisionRepository) Library(ctx context.Context, input domain.LibraryQuery) (*domain.LibraryPage, error) {
	if input.Limit < 1 || input.Limit > 100 || input.Offset < 0 || input.Offset > 10000000 || !utf8.ValidString(input.Keyword) || len(input.Keyword) > 500 {
		return nil, domain.InvalidInput("invalid authoring list query")
	}
	if input.Visibility != "" && input.Visibility != "draft" && input.Visibility != "private" && input.Visibility != "public" {
		return nil, domain.InvalidInput("invalid visibility")
	}
	if input.Status != "" && input.Status != "changes" && input.Status != "conflicts" && input.Status != "unpublished" {
		return nil, domain.InvalidInput("invalid authoring status")
	}
	tx, err := repo.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scope, err := tenancypg.LockScope(ctx, tx, tenancy.ActorID(ctx))
	if err != nil {
		return nil, packageReadError(err)
	}
	readScope := scope
	readScope.Domain.Archived = false
	args := dbgen.ListAuthoringLibraryParams{ActorID: scope.UserID, DomainID: scope.Domain.ID, IsManager: readScope.Allows(tenancy.ManageResources), ActiveMember: scope.ActiveMember(), Keyword: input.Keyword, Visibility: input.Visibility, Status: input.Status, PageLimit: input.Limit, PageOffset: input.Offset}
	q := dbgen.New(tx)
	rows, err := q.ListAuthoringLibrary(ctx, args)
	if err != nil {
		return nil, err
	}
	result := &domain.LibraryPage{Items: []domain.LibraryItem{}}
	for _, row := range rows {
		role := problem.AccessRole("")
		if row.GrantRank == 1 {
			role = problem.AccessReader
		} else if row.GrantRank >= 2 {
			role = problem.AccessEditor
		}
		permissions := problem.EffectivePermissions(scope, row.OwnerID, row.Visibility, role)
		result.Items = append(result.Items, domain.LibraryItem{ID: row.PublicID, Title: row.Title, Source: row.Source, Visibility: row.Visibility, OwnerName: row.OwnerName, CanEdit: permissions.Edit, CanPublish: permissions.Publish, HasCopy: row.HasCopy, HasChanges: row.HasChanges, HasConflict: row.HasConflict, BaseRevision: row.BaseRevision, HeadRevision: row.HeadRevision, PublishedVersion: row.PublishedVersion, PublishedRevision: row.PublishedRevision, CheckID: row.CheckID, CheckState: row.CheckState, CheckMatches: row.CheckMatches, UpdatedAt: row.UpdatedAt})
		result.Total = int(row.Total)
	}
	// An empty out-of-range page must still report the actual total.
	if len(rows) == 0 && input.Offset > 0 {
		args.PageLimit = 1
		args.PageOffset = 0
		first, err := q.ListAuthoringLibrary(ctx, args)
		if err != nil {
			return nil, err
		}
		if len(first) > 0 {
			result.Total = int(first[0].Total)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
