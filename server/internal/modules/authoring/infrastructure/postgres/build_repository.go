package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// BuildRepository stores sealed input and accepts later writes only from the
// current worker with its live lease token.
type BuildRepository struct {
	db       *database.DB
	queries  *dbgen.Queries
	packages *PackageQueries
}

func NewBuildRepository(db *database.DB) *BuildRepository {
	return &BuildRepository{db: db, queries: dbgen.New(db.Pool.DB), packages: NewPackageQueries(db)}
}

var _ domain.BuildRepository = (*BuildRepository)(nil)

func (r *BuildRepository) Enqueue(ctx context.Context, problemID, createdBy string) (*domain.Build, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := problempg.LockAccess(ctx, tx, problemID, createdBy)
	if err != nil {
		return nil, packageReadError(err)
	}
	if !access.Permissions.Edit {
		return nil, tenancy.ErrForbidden
	}
	q := r.queries.WithTx(tx.Tx)
	revision, err := q.LockProblemPackageRevision(ctx, dbgen.LockProblemPackageRevisionParams{ProblemID: problemID, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, packageReadError(err)
	}
	existing, err := q.GetActiveBuild(ctx, problemID)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		item := buildFromRecord(dbgen.GetBuildRow(existing))
		return &item, domain.ErrBuildRunning
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	pkg, err := loadPackageSnapshot(ctx, tx, problemID)
	if err != nil {
		return nil, err
	}
	input, err := json.Marshal(pkg)
	if err != nil {
		return nil, err
	}
	var creator *string
	if createdBy != "" {
		creator = &createdBy
	}
	row, err := q.CreateBuild(ctx, dbgen.CreateBuildParams{ProblemID: problemID, Revision: revision, CreatedBy: creator, DataRevision: pkg.DataRevision, InputJson: input})
	if err != nil {
		return nil, err
	}
	if err := q.NotifyBuildJob(ctx, row.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item := buildFromRecord(dbgen.GetBuildRow(row))
	return &item, nil
}

func (r *BuildRepository) Get(ctx context.Context, problemID, buildID string) (*domain.Build, error) {
	if err := r.packages.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	row, err := r.queries.GetBuild(ctx, dbgen.GetBuildParams{BuildID: buildID, ProblemID: problemID, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, packageReadError(err)
	}
	item := buildFromRecord(row)
	return &item, nil
}

func (r *BuildRepository) Latest(ctx context.Context, problemID string) (*domain.Build, error) {
	if err := r.packages.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	row, err := r.queries.GetLatestBuild(ctx, dbgen.GetLatestBuildParams{ProblemID: problemID, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item := buildFromRecord(dbgen.GetBuildRow(row))
	return &item, nil
}

func (r *BuildRepository) LatestSuccessful(ctx context.Context, problemID string) (*domain.Build, error) {
	if err := r.packages.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	row, err := r.queries.GetLatestSuccessfulBuild(ctx, dbgen.GetLatestSuccessfulBuildParams{ProblemID: problemID, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item := buildFromRecord(dbgen.GetBuildRow(row))
	return &item, nil
}

func (r *BuildRepository) List(ctx context.Context, problemID string, limit int) ([]domain.Build, error) {
	if err := r.packages.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.queries.ListBuilds(ctx, dbgen.ListBuildsParams{ProblemID: problemID, DomainID: tenancy.ID(ctx), PageLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	items := make([]domain.Build, 0, len(rows))
	for _, row := range rows {
		items = append(items, buildFromRecord(dbgen.GetBuildRow(row)))
	}
	return items, nil
}

// Claim retries the original sealed input, never the current editable package.
func (r *BuildRepository) Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*domain.Build, *domain.Package, error) {
	if err := r.failExhausted(ctx); err != nil {
		return nil, nil, err
	}
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	q := r.queries.WithTx(tx.Tx)
	row, err := q.ClaimBuild(ctx, dbgen.ClaimBuildParams{WorkerID: workerID, LeaseMs: leaseTTL.Milliseconds(), MaxAttempts: maxBuildAttempts})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	input, err := q.GetBuildInput(ctx, row.ID)
	if err != nil {
		return nil, nil, err
	}
	var pkg domain.Package
	if err := json.Unmarshal(input.InputJson, &pkg); err != nil {
		return nil, nil, err
	}
	if pkg.ProblemID != row.ProblemID || pkg.Revision != row.Revision || pkg.DataRevision != row.DataRevision || pkg.DomainID != input.DomainID {
		return nil, nil, domain.ErrPackageTarget
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	item := buildFromRecord(dbgen.GetBuildRow(row))
	return &item, &pkg, nil
}

func buildFromRecord(row dbgen.GetBuildRow) domain.Build {
	item := domain.Build{ID: row.ID, ProblemID: row.ProblemID, Revision: row.Revision, DataRevision: row.DataRevision,
		State: row.State, Stage: row.Stage, Attempt: row.Attempt, WorkerID: row.WorkerID, LeaseToken: row.LeaseToken,
		LeaseExpires: row.LeaseExpiresAt, ProgressDone: row.ProgressDone, ProgressTotal: row.ProgressTotal,
		Log: row.Log, ErrorMessage: row.ErrorMessage, PackagePath: row.PackagePath, PackageSHA256: row.PackageSha256,
		PackageCases: row.PackageCases, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt}
	if err := json.Unmarshal(row.TestsJson, &item.Tests); err != nil {
		item.Tests = nil
	}
	if err := json.Unmarshal(row.SolutionsJson, &item.Solutions); err != nil {
		item.Solutions = nil
	}
	return item
}
