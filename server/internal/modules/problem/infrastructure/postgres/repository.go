package postgres

import (
	"context"
	"encoding/json"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// Repository owns resource creation, access policy and deletion.
// Authoring materials and immutable releases are owned by the authoring module.
type Repository struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db, queries: dbgen.New(db.Pool.DB)}
}

func encodeTags(tags []string) ([]byte, error) {
	if tags == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(tags)
}

func (r *Repository) Create(ctx context.Context, authorID string, in *domain.CreateInput) (*domain.ProblemView, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scope, err := tenancypg.LockScope(ctx, tx, authorID)
	if err != nil {
		return nil, accessError(err)
	}
	if !scope.Allows(tenancy.CreateProblem) {
		return nil, tenancy.ErrForbidden
	}
	if in.TimeLimitMs <= 0 {
		in.TimeLimitMs = 1000
	}
	if in.MemoryLimitKb <= 0 {
		in.MemoryLimitKb = 262144
	}
	if in.Visibility == "" {
		in.Visibility = "draft"
	}
	queries := r.queries.WithTx(tx.Tx)
	row, err := queries.CreateProblem(ctx, dbgen.CreateProblemParams{Title: in.Title, StatementMd: in.StatementMD, Difficulty: in.Difficulty, Source: in.Source,
		TimeLimitMs: in.TimeLimitMs, MemoryLimitKb: in.MemoryLimitKb, Visibility: in.Visibility, AuthorID: authorID, DomainID: scope.Domain.ID})
	if err != nil {
		return nil, err
	}
	if err := tenancypg.ResourceGuard(ctx, tx, "tag-catalog", scope.Domain.ID, false); err != nil {
		return nil, err
	}
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	if err := queries.CreateProblemTags(ctx, dbgen.CreateProblemTagsParams{ProblemID: row.ID, DomainID: scope.Domain.ID, Tags: tags}); err != nil {
		return nil, err
	}
	result, err := problemFromRow(dbgen.GetProblemRow(row))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result.Tags = in.Tags
	result.Permissions = domain.EffectivePermissions(scope, authorID, result.Visibility, domain.AccessOwner)
	return &result, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Delete {
		return tenancy.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	referenced, err := queries.HasProblemReferences(ctx, id)
	if err != nil {
		return err
	}
	if referenced {
		return domain.ErrReferenced
	}
	if err := recordAccessAudit(ctx, queries, access, "problem.deleted", ""); err != nil {
		return err
	}
	affected, err := queries.DeleteProblem(ctx, dbgen.DeleteProblemParams{ProblemID: id, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	// Content-addressed authoring/publication data may still be needed if
	// COMMIT fails or has an unknown outcome. Reference-aware collection owns
	// physical deletion after the database has durably removed all references.
	return tx.Commit()
}
