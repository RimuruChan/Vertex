package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/database"
	domain "github.com/RimuruChan/Vertex/server/internal/judge/domain"
	"github.com/RimuruChan/Vertex/server/internal/judge/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
)

const maxJudgeAttempts = 5

// JobRepository owns leased jobs and atomically commits their result, case
// snapshots and counters. Workers never write these tables directly.
type JobRepository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

var _ domain.Repository = (*JobRepository)(nil)

func NewJobRepository(db *database.DB) *JobRepository {
	return &JobRepository{db: db.Pool.DB, queries: dbgen.New(db.Pool.DB)}
}

func (r *JobRepository) Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*domain.Job, error) {
	if err := r.failExhausted(ctx); err != nil {
		return nil, err
	}
	row, err := r.queries.ClaimJudgeJob(ctx, dbgen.ClaimJudgeJobParams{WorkerID: workerID, LeaseMillis: leaseTTL.Milliseconds(), MaxAttempts: maxJudgeAttempts})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// These columns are nullable for queued jobs, but ClaimJudgeJob always fills
	// them. Reject an invalid database result instead of returning a zero lease.
	if !row.WorkerID.Valid || row.LeaseToken == nil || row.LeaseExpiresAt == nil {
		return nil, fmt.Errorf("claimed judge job %s has an incomplete lease", row.ID)
	}
	return &domain.Job{ID: row.ID, SubmissionID: row.SubmissionID, Generation: row.Generation, Attempt: row.Attempt,
		WorkerID: row.WorkerID.String, LeaseToken: *row.LeaseToken, LeaseExpiresAt: *row.LeaseExpiresAt,
		UserID: row.UserID, ProblemID: row.ProblemID, ContestID: row.ContestID, Language: row.Language, SourceCode: row.SourceCode,
		TimeLimitMs: row.TimeLimitMs, MemoryLimitKB: row.MemoryLimitKb, DomainID: row.DomainID, ProblemVersion: row.ProblemVersion,
		Testdata: domain.Testdata{StoragePath: row.TestdataPath, DataVersion: row.ArtifactVersion, SHA256: row.Sha256, CaseCount: row.CaseCount, Checker: row.Checker}}, nil
}

// Heartbeat renews the fenced lease and monotonically advances display progress.
func (r *JobRepository) Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string, judgedCases int, leaseTTL time.Duration) error {
	renewed, err := r.queries.RenewJudgeLease(ctx, dbgen.RenewJudgeLeaseParams{JobID: jobID, Generation: generation,
		LeaseToken: leaseToken, WorkerID: workerID, JudgedCases: judgedCases, LeaseMillis: leaseTTL.Milliseconds()})
	if err != nil {
		return err
	}
	if renewed != 1 {
		return domain.ErrStaleLease
	}
	return nil
}

func (r *JobRepository) Complete(ctx context.Context, result domain.Result) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := r.queries.WithTx(tx)
	target, err := queries.LockJudgeResultTarget(ctx, dbgen.LockJudgeResultTargetParams{JobID: result.JobID, Generation: result.Generation})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrStaleLease
	}
	if err != nil {
		return err
	}
	if target.SubmissionID != result.SubmissionID {
		return domain.ErrStaleLease
	}
	if target.State == "completed" {
		if target.WorkerID != result.WorkerID || target.LeaseToken != result.LeaseToken {
			return domain.ErrStaleLease
		}
		return tx.Commit()
	}
	affected, err := queries.CompleteJudgeJob(ctx, dbgen.CompleteJudgeJobParams{JobID: result.JobID, Generation: result.Generation, LeaseToken: result.LeaseToken, WorkerID: result.WorkerID})
	if err != nil {
		return err
	}
	if affected != 1 {
		return domain.ErrStaleLease
	}
	cases, err := json.Marshal(caseResultSnapshot(result.Cases))
	if err != nil {
		return err
	}
	affected, err = queries.SaveSubmissionResult(ctx, dbgen.SaveSubmissionResultParams{SubmissionID: result.SubmissionID, Generation: result.Generation,
		Status: result.Status, Score: result.Score, TotalTimeMs: result.TotalTimeMs, PeakMemoryKb: result.PeakMemoryKB,
		CompileResult: result.CompileResult, CaseResults: cases, JudgedCases: len(result.Cases)})
	if err != nil {
		return err
	}
	if affected != 1 {
		return domain.ErrStaleLease
	}
	if err := queries.DeleteSubmissionCases(ctx, result.SubmissionID); err != nil {
		return err
	}
	for _, item := range result.Cases {
		if err := queries.InsertSubmissionCase(ctx, dbgen.InsertSubmissionCaseParams{SubmissionID: result.SubmissionID, CaseIndex: item.CaseIndex,
			Verdict: item.Verdict, TimeMs: item.TimeMs, MemoryKb: item.MemoryKB, ExitStatus: item.ExitStatus, CheckerOutput: item.CheckerOutput}); err != nil {
			return err
		}
	}
	if err := rebuildResultCounters(ctx, tx, target.ContestID, target.UserID, target.ProblemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *JobRepository) failExhausted(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := r.queries.WithTx(tx)
	jobs, err := queries.FailExhaustedJudgeJobs(ctx, maxJudgeAttempts)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		target, err := queries.FailSubmissionForExpiredLease(ctx, dbgen.FailSubmissionForExpiredLeaseParams{SubmissionID: job.SubmissionID, Generation: job.Generation, CompileResult: "judge worker lease expired too many times"})
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if err := queries.DeleteSubmissionCases(ctx, job.SubmissionID); err != nil {
			return err
		}
		if err := rebuildResultCounters(ctx, tx, target.ContestID, target.UserID, target.ProblemID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func rebuildResultCounters(ctx context.Context, tx *sql.Tx, contestID *string, userID, problemID string) error {
	if contestID != nil && *contestID != "" {
		return contestpg.RebuildCell(ctx, tx, *contestID, userID, problemID)
	}
	return problempg.RebuildPracticeCounters(ctx, tx, problemID)
}

type persistedCaseResult struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKB      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

func caseResultSnapshot(cases []domain.CaseResult) []persistedCaseResult {
	snapshot := make([]persistedCaseResult, 0, len(cases))
	for _, item := range cases {
		snapshot = append(snapshot, persistedCaseResult{
			CaseIndex: item.CaseIndex, Verdict: item.Verdict, TimeMs: item.TimeMs,
			MemoryKB: item.MemoryKB, ExitStatus: item.ExitStatus, CheckerOutput: item.CheckerOutput,
		})
	}
	return snapshot
}
