package postgres

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/database"
	domain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

type Repository struct {
	*Queries
	db      *database.DB
	queries *dbgen.Queries
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{Queries: NewQueries(db), db: db, queries: dbgen.New(db.Pool.DB)}
}

// Create rechecks the target under the same transaction as insertion and job
// enqueue. Schema triggers pin the approved problem version for this generation.
func (r *Repository) Create(ctx context.Context, input *domain.Submission) (*domain.Submission, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := validateSubmissionTarget(ctx, tx, input); err != nil {
		return nil, err
	}
	q := r.queries.WithTx(tx.Tx)
	row, err := q.CreateSubmission(ctx, dbgen.CreateSubmissionParams{UserID: input.UserID, ProblemID: input.ProblemID,
		Language: input.Language, SourceCode: input.SourceCode, ContestID: input.ContestID, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, err
	}
	if err := q.EnqueueInitialJudgeJob(ctx, row.ID); err != nil {
		return nil, err
	}
	if err := q.NotifyJudgeJob(ctx, row.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &domain.Submission{ID: row.ID, PublicID: row.PublicID, UserID: row.UserID, ProblemID: row.ProblemID,
		Language: row.Language, SourceCode: row.SourceCode, Status: row.Status, Score: row.Score,
		TotalTimeMs: row.TotalTimeMs, PeakMemoryKb: row.PeakMemoryKb, CompileResult: row.CompileResult,
		ContestID: row.ContestID, SubmittedAt: row.SubmittedAt, JudgedAt: row.JudgedAt, ProblemVersion: row.ProblemVersion}, nil
}
