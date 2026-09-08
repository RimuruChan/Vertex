package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"

	"github.com/jmoiron/sqlx"
)

// CreateRejudging expands a selector into a batch, bumps every matched
// submission to a new generation and queues fresh judge jobs for them.
//
// The recorded generation is the fence: a member whose current generation has
// moved past the recorded one was taken over by a later rejudging, and this
// batch stops counting it.
func (s *Repository) CreateRejudging(
	ctx context.Context, selector submissiondomain.RejudgeSelector, createdBy string,
) (*submissiondomain.Rejudging, error) {
	if selector.IsEmpty() {
		return nil, submissiondomain.ErrRejudgeEmpty
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	practiceOnly, err := lockRejudgeTarget(ctx, tx, selector.ContestID, selector.ProblemID, createdBy)
	if err != nil {
		return nil, err
	}
	if err := lockRejudgeGeneration(ctx, tx); err != nil {
		return nil, err
	}

	q := s.queries.WithTx(tx.Tx)
	candidates, err := q.FindRejudgingCandidates(ctx, dbgen.FindRejudgingCandidatesParams{
		DomainID: tenancydomain.ID(ctx), ContestFilter: selector.ContestID, ProblemFilter: selector.ProblemID,
		UserFilter: selector.UserID, LanguageFilter: selector.Language, StatusFilter: selector.Status,
		FilterIds: len(selector.SubmissionIDs) > 0, SubmissionIds: selector.SubmissionIDs,
		PracticeOnly: practiceOnly, BatchLimit: submissiondomain.MaxRejudgeBatch,
	})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, submissiondomain.ErrRejudgeEmpty
	}
	// Queue locks precede submission locks, matching worker completion. Recheck
	// the selector and snapshot results only after in-flight completion has settled.
	if _, err := q.LockActiveJudgeJobs(ctx, candidates); err != nil {
		return nil, err
	}
	members, err := q.LockRejudgingSnapshots(ctx, dbgen.LockRejudgingSnapshotsParams{
		DomainID: tenancydomain.ID(ctx), ContestFilter: selector.ContestID, ProblemFilter: selector.ProblemID,
		UserFilter: selector.UserID, LanguageFilter: selector.Language, StatusFilter: selector.Status,
		FilterIds: len(selector.SubmissionIDs) > 0, SubmissionIds: selector.SubmissionIDs,
		PracticeOnly: practiceOnly, CandidateIds: candidates, BatchLimit: submissiondomain.MaxRejudgeBatch,
	})
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, submissiondomain.ErrRejudgeEmpty
	}

	var creator *string
	if createdBy != "" {
		creator = &createdBy
	}
	var contestID, problemID *string
	if selector.ContestID != "" {
		contestID = &selector.ContestID
	}
	if selector.ProblemID != "" {
		problemID = &selector.ProblemID
	}

	row, err := q.CreateRejudging(ctx, dbgen.CreateRejudgingParams{ContestID: contestID, ProblemID: problemID,
		Reason: selector.Reason, TotalCount: len(members), CreatedBy: creator, DomainID: tenancydomain.ID(ctx)})
	if err != nil {
		return nil, err
	}
	batch := submissiondomain.Rejudging{ID: row.ID, ContestID: row.ContestID, ProblemID: row.ProblemID,
		Reason: row.Reason, State: row.State, TotalCount: row.TotalCount, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, FinishedAt: row.FinishedAt}

	problemIDs := make(map[string]struct{})
	for _, item := range members {
		generation, problemID, practice, err := requeueSubmission(ctx, tx, item.ID)
		if err != nil {
			return nil, err
		}
		if practice {
			problemIDs[problemID] = struct{}{}
		}
		if err := q.SaveRejudgingSnapshot(ctx, dbgen.SaveRejudgingSnapshotParams{RejudgingID: batch.ID, SubmissionID: item.ID, Generation: generation,
			PriorStatus: item.Status, PriorScore: item.Score,
			PriorTotalTimeMs: item.TotalTimeMs, PriorPeakMemoryKb: item.PeakMemoryKb,
			PriorCompileResult: item.CompileResult, PriorCaseResults: item.CaseResults, PriorJudgedCases: item.JudgedCases, PriorTotalCases: item.TotalCases,
			PriorJudgedAt: item.JudgedAt, DomainID: tenancydomain.ID(ctx), PriorProblemVersion: item.ProblemVersion}); err != nil {
			return nil, err
		}
	}
	orderedProblemIDs := make([]string, 0, len(problemIDs))
	for problemID := range problemIDs {
		orderedProblemIDs = append(orderedProblemIDs, problemID)
	}
	sort.Strings(orderedProblemIDs)
	for _, problemID := range orderedProblemIDs {
		if err := problempg.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
			return nil, err
		}
	}
	// One notification is enough: the dispatcher cascades waiters as claims
	// succeed, so a batch does not need to wake every worker individually.
	if err := q.NotifyJudgeJob(ctx, batch.ID); err != nil {
		return nil, err
	}
	if err := auditRejudge(ctx, tx, createdBy, "rejudging.create", batch.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &batch, nil
}

// CancelRejudging stops the queued part of a running batch. Every withdrawn
// member gets its exact pre-batch snapshot back; work already leased to a
// worker is allowed to finish so its fenced result still lands consistently.
func (s *Repository) CancelRejudging(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	contestID, problemID, err := loadRejudgeParent(ctx, tx, id)
	if err != nil {
		return err
	}
	if _, err := lockRejudgeTarget(ctx, tx, refValue(contestID), refValue(problemID), tenancydomain.ActorID(ctx)); err != nil {
		return err
	}
	if err := lockRejudgeGeneration(ctx, tx); err != nil {
		return err
	}

	q := s.queries.WithTx(tx.Tx)
	state, err := q.LockRejudging(ctx, dbgen.LockRejudgingParams{ID: id, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return submissiondomain.ErrRejudgeNotFound
	}
	if err != nil {
		return err
	}

	if state != submissiondomain.RejudgingRunning {
		return submissiondomain.ErrRejudgeClosed
	}

	restored, err := q.RestoreQueuedRejudgingSubmissions(ctx, id)
	if err != nil {
		return err
	}

	// submission_cases mirrors the JSON snapshot for indexed/operational reads.
	// Restore it as well so cancellation never leaves two representations of a
	// verdict disagreeing.
	if err := q.DeleteCancelledRejudgingCases(ctx, id); err != nil {
		return err
	}
	if err := q.RestoreCancelledRejudgingCases(ctx, id); err != nil {
		return err
	}

	practiceProblems := make(map[string]struct{})
	type cell struct{ contestID, userID, problemID string }
	contestCells := make(map[string]cell)
	for _, item := range restored {
		if item.ContestID == nil || *item.ContestID == "" {
			practiceProblems[item.ProblemID] = struct{}{}
			continue
		}
		key := *item.ContestID + "\x00" + item.UserID + "\x00" + item.ProblemID
		contestCells[key] = cell{contestID: *item.ContestID, userID: item.UserID, problemID: item.ProblemID}
	}
	practiceKeys := make([]string, 0, len(practiceProblems))
	for problemID := range practiceProblems {
		practiceKeys = append(practiceKeys, problemID)
	}
	sort.Strings(practiceKeys)
	for _, problemID := range practiceKeys {
		if err := problempg.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
			return err
		}
	}
	cellKeys := make([]string, 0, len(contestCells))
	for key := range contestCells {
		cellKeys = append(cellKeys, key)
	}
	sort.Strings(cellKeys)
	for _, key := range cellKeys {
		item := contestCells[key]
		if err := contestpg.RebuildCell(ctx, tx, item.contestID, item.userID, item.problemID); err != nil {
			return err
		}
	}
	if err := q.CancelRejudging(ctx, id); err != nil {
		return err
	}
	if err := auditRejudge(ctx, tx, tenancydomain.ActorID(ctx), "rejudging.cancel", id); err != nil {
		return err
	}
	return tx.Commit()
}

func loadRejudgeParent(ctx context.Context, tx *sqlx.Tx, id string) (*string, *string, error) {
	row, err := dbgen.New(tx).GetRejudgingTarget(ctx, dbgen.GetRejudgingTargetParams{ID: id, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, submissiondomain.ErrRejudgeNotFound
	}
	return row.ContestID, row.ProblemID, err
}
