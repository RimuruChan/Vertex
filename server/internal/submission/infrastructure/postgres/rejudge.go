package postgres

import (
	"context"
	"database/sql"
	"errors"

	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

// Rejudge re-queues one submission. It shares requeueSubmission with batch
// rejudging so both paths reset state, fence the generation and repair the
// standings identically.
func (s *Repository) Rejudge(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx.Tx)
	target, err := q.GetSubmissionRejudgeTarget(ctx, dbgen.GetSubmissionRejudgeTargetParams{SubmissionID: id, DomainID: tenancydomain.ID(ctx)})

	if errors.Is(err, sql.ErrNoRows) {
		return submissiondomain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := lockRejudgeTarget(ctx, tx, refValue(target.ContestID), target.ProblemID, tenancydomain.ActorID(ctx)); err != nil {
		return err
	}
	if err := lockRejudgeGeneration(ctx, tx); err != nil {
		return err
	}

	_, problemID, practice, err := requeueSubmission(ctx, tx, id)
	if err != nil {
		return err
	}
	if practice {
		if err := problempg.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
			return err
		}
	}
	if err := q.NotifyJudgeJob(ctx, id); err != nil {
		return err
	}
	if err := auditRejudge(ctx, tx, tenancydomain.ActorID(ctx), "submission.rejudge", id); err != nil {
		return err
	}
	return tx.Commit()
}

func refValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
