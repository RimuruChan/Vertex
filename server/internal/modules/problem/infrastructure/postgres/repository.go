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

// Repository owns authorized writes to a problem and its mutable workspace.
// Immutable testdata lives behind an injected artifact storage contract.
type Repository struct {
	db        *database.DB
	queries   *dbgen.Queries
	artifacts domain.ArtifactStorage
}

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *database.DB, artifacts domain.ArtifactStorage) *Repository {
	return &Repository{db: db, queries: dbgen.New(db.Pool.DB), artifacts: artifacts}
}

func encodeTags(tags []string) ([]byte, error) {
	if tags == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(tags)
}

func (r *Repository) Create(ctx context.Context, authorID string, in *domain.CreateInput) (*domain.Problem, error) {
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
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	if err := queries.SetWorkspaceTags(ctx, dbgen.SetWorkspaceTagsParams{ProblemID: row.ID, Tags: tags}); err != nil {
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

// Update changes the working copy; published content changes only on publication.
func (r *Repository) Update(ctx context.Context, id string, in *domain.UpdateInput) (*domain.Problem, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !access.Permissions.Edit || (in.Visibility != access.Visibility && !access.Permissions.Publish) {
		return nil, tenancy.ErrForbidden
	}
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	queries := r.queries.WithTx(tx.Tx)
	changes, err := queries.CompareProblemWorkspace(ctx, dbgen.CompareProblemWorkspaceParams{ProblemID: id, Title: in.Title, StatementMd: in.StatementMD, Difficulty: in.Difficulty, Source: in.Source, TimeLimitMs: in.TimeLimitMs, MemoryLimitKb: in.MemoryLimitKb, Tags: tags})
	if err != nil {
		return nil, err
	}
	if err := queries.UpdateProblemWorkspace(ctx, dbgen.UpdateProblemWorkspaceParams{ProblemID: id, Title: in.Title, StatementMd: in.StatementMD, Difficulty: in.Difficulty, Source: in.Source, TimeLimitMs: in.TimeLimitMs, MemoryLimitKb: in.MemoryLimitKb, Tags: tags}); err != nil {
		return nil, err
	}
	if changes.MetadataChanged {
		if err := queries.SyncDefaultStatementName(ctx, dbgen.SyncDefaultStatementNameParams{ProblemID: id, Title: in.Title}); err != nil {
			return nil, err
		}
	}
	if err := queries.UpdateProblemRevisions(ctx, dbgen.UpdateProblemRevisionsParams{ProblemID: id, Visibility: in.Visibility, MetadataChanged: changes.MetadataChanged, DataChanged: changes.DataChanged}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result, err := NewQueries(r.db).GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	result.Permissions = domain.EffectivePermissions(access.Scope, access.OwnerID, in.Visibility, access.Role)
	return result, nil
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
	if err := r.artifacts.RemoveProblem(id); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveTestdata replaces the candidate and advances its revision under the
// problem authorization lock. It never changes an immutable publication.
func (r *Repository) SaveTestdata(ctx context.Context, problemID string, data []byte, checker string) (int, string, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, problemID, tenancy.ActorID(ctx))
	if err != nil {
		return 0, "", err
	}
	if !access.Permissions.Edit {
		return 0, "", tenancy.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	revision, err := queries.AdvanceTestdataRevision(ctx, problemID)
	if err != nil {
		return 0, "", err
	}
	artifact, err := r.artifacts.Materialize(problemID, data)
	if err != nil {
		return 0, "", err
	}
	if err := queries.SaveImportedTestdata(ctx, dbgen.SaveImportedTestdataParams{ProblemID: problemID, StoragePath: artifact.StoragePath, Sha256: artifact.SHA256, CaseCount: artifact.CaseCount, Checker: checker, DataRevision: revision}); err != nil {
		return 0, "", err
	}
	return artifact.CaseCount, artifact.SHA256, tx.Commit()
}
