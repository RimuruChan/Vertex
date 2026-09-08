package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
	domain "github.com/RimuruChan/Vertex/server/internal/problemset/domain"
	"github.com/RimuruChan/Vertex/server/internal/problemset/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
)

// Repository persists curated sets and derives their viewer-specific read model.
// Access to a set never grants access to the problems it references.
type Repository struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db, queries: dbgen.New(db.Pool.DB)}
}

func canManageRead(scope tenancy.Scope) bool {
	scope.Domain.Archived = false
	return scope.Allows(tenancy.ManageResources)
}

func setFromRow(row dbgen.GetVisibleSetRow, scope tenancy.Scope) domain.Set {
	item := domain.Set{ID: row.ID, PublicID: row.PublicID, Title: row.Title, Description: row.Description, AuthorID: row.AuthorID, AuthorName: row.AuthorName,
		Visibility: row.Visibility, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DomainID: row.DomainID, OwnerID: row.OwnerID, OwnerName: row.OwnerName, ProblemCount: row.ProblemCount, SolvedCount: row.SolvedCount}
	item.Permissions = domain.EffectivePermissions(scope, item.OwnerID, item.Visibility, rankRole(row.GrantRank))
	item.Permissions.EditItems = item.Permissions.EditItems && row.AllItemsVisible
	return item
}

func (r *Repository) List(ctx context.Context, filters domain.Filters) ([]domain.Set, int, error) {
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, filters.ViewerID)
	if err != nil {
		return nil, 0, accessError(err)
	}
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	manager := canManageRead(scope)
	total, err := r.queries.CountVisibleSets(ctx, dbgen.CountVisibleSetsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID, IsManager: manager, ActiveMember: scope.ActiveMember(), AuthorID: filters.AuthorID, Keyword: filters.Keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListVisibleSets(ctx, dbgen.ListVisibleSetsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID, IsManager: manager, ActiveMember: scope.ActiveMember(), AuthorID: filters.AuthorID, Keyword: filters.Keyword, PageLimit: filters.Limit, PageOffset: filters.Offset})
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.Set, 0, len(rows))
	for _, row := range rows {
		result = append(result, setFromRow(dbgen.GetVisibleSetRow(row), scope))
	}
	return result, int(total), nil
}

func (r *Repository) Get(ctx context.Context, id, viewerID string) (*domain.Set, error) {
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, viewerID)
	if err != nil {
		return nil, accessError(err)
	}
	manager := canManageRead(scope)
	row, err := r.queries.GetVisibleSet(ctx, dbgen.GetVisibleSetParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID, IsManager: manager, ActiveMember: scope.ActiveMember(), SetID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := setFromRow(row, scope)
	rows, err := r.queries.ListVisibleItems(ctx, dbgen.ListVisibleItemsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID, IsManager: manager, ActiveMember: scope.ActiveMember(), SetID: id})
	if err != nil {
		return nil, err
	}
	item.Items = make([]domain.Item, 0, len(rows))
	for _, row := range rows {
		entry := domain.Item{ProblemID: row.ProblemID, ProblemPublicID: row.PublicID, OwnerID: database.Ptr(row.OwnerID), SortOrder: row.SortOrder, Note: row.Note,
			Title: row.Title, Difficulty: row.Difficulty, Visibility: row.Visibility, SubmitCount: row.SubmissionCount, AcceptCount: row.AcceptedCount, UserStatus: row.UserStatus}
		if err := json.Unmarshal(row.Tags, &entry.Tags); err != nil {
			return nil, err
		}
		item.Items = append(item.Items, entry)
	}
	return &item, nil
}
