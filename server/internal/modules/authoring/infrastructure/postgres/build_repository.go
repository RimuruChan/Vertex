package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// BuildRepository stores sealed input and accepts later writes only from the
// current worker with its live lease token.
type BuildRepository struct {
	db      *database.DB
	queries *dbgen.Queries
}

func NewBuildRepository(db *database.DB) *BuildRepository {
	return &BuildRepository{db: db, queries: dbgen.New(db.Pool.DB)}
}

var _ domain.WorkerBuildRepository = (*BuildRepository)(nil)

// Claim retries the original sealed input, never the current editable package.
func (r *BuildRepository) Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*domain.Build, *domain.CheckInput, error) {
	if err := r.failExhausted(ctx); err != nil {
		return nil, nil, err
	}
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	q := r.queries.WithTx(tx.Tx)
	row, err := q.ClaimBuild(ctx, dbgen.ClaimBuildParams{WorkerID: workerID, LeaseMs: leaseTTL.Milliseconds(), MaxAttempts: maxBuildAttempts, AcceptsChecks: domain.AcceptsCheckProtocol(ctx)})
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
	var pkg domain.CheckInput
	if err := json.Unmarshal(input.InputJson, &pkg); err != nil {
		return nil, nil, err
	}
	if pkg.Check == nil || pkg.ProblemID != row.ProblemID || pkg.DomainID != input.DomainID {
		return nil, nil, domain.ErrPackageTarget
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	item := buildFromRecord(row)
	return &item, &pkg, nil
}

func buildFromRecord(row dbgen.ClaimBuildRow) domain.Build {
	item := domain.Build{ProblemNumber: row.ProblemNumber, ID: row.ID, ProblemID: row.ProblemID,
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
