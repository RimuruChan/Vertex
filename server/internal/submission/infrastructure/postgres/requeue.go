package postgres

import (
	"context"
	"database/sql"
	"errors"

	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/jmoiron/sqlx"
)

// requeueSubmission resets one submission to Pending under a new generation
// and queues a job for it. It is the shared core of single and batch rejudge.
func requeueSubmission(ctx context.Context, tx *sqlx.Tx, submissionID string) (int, string, bool, error) {
	q := dbgen.New(tx)
	// Cancel outstanding work first, matching the claim/result lock order.
	if err := q.CancelActiveJudgeJobs(ctx, dbgen.CancelActiveJudgeJobsParams{SubmissionID: submissionID, DomainID: tenancydomain.ID(ctx)}); err != nil {
		return 0, "", false, err
	}
	submission, err := q.ResetSubmissionForRejudge(ctx, dbgen.ResetSubmissionForRejudgeParams{SubmissionID: submissionID, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, submissiondomain.ErrNotFound
	}
	if err != nil {
		return 0, "", false, err
	}
	if err := q.EnqueueJudgeGeneration(ctx, dbgen.EnqueueJudgeGenerationParams{SubmissionID: submissionID, Generation: submission.JudgeGeneration}); err != nil {
		return 0, "", false, err
	}
	if err := q.DeleteSubmissionCases(ctx, submissionID); err != nil {
		return 0, "", false, err
	}
	// A pending submission must not keep contributing to the standings.
	practice := submission.ContestID == nil || *submission.ContestID == ""
	if !practice {
		if err := contestpg.RebuildCell(ctx, tx, *submission.ContestID, submission.UserID, submission.ProblemID); err != nil {
			return 0, "", false, err
		}
	}
	return submission.JudgeGeneration, submission.ProblemID, practice, nil
}
