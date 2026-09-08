package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
)

// Execer lets verdict and rejudge repositories share the caller's transaction.
type Execer = dbgen.DBTX

// RebuildCell replaces both jury and public projections from submission facts.
// The advisory lock serializes concurrent results for the same score cell.
func RebuildCell(ctx context.Context, tx Execer, contestID, userID, problemID string) error {
	q := dbgen.New(tx)
	if err := q.LockContestCell(ctx, "contest-cell:"+contestID+":"+userID+":"+problemID); err != nil {
		return err
	}
	settings, err := q.GetContestScoringRules(ctx, dbgen.GetContestScoringRulesParams{ContestID: contestID, ProblemID: problemID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rules := domain.ScoringRules{Format: domain.NormalizeFormat(settings.Rule), BeginAt: settings.BeginAt, EndAt: settings.EndAt, FreezeAt: settings.FreezeAt,
		PenaltyMinutes: settings.PenaltyMinutes, PenalizeCompileError: settings.PenalizeCompileError, MaxPoints: settings.MaxPoints}
	rows, err := q.ListScoredSubmissions(ctx, dbgen.ListScoredSubmissionsParams{ContestID: contestID, UserID: userID, ProblemID: problemID})
	if err != nil {
		return err
	}
	submissions := make([]domain.ScoredSubmission, 0, len(rows))
	for _, row := range rows {
		submissions = append(submissions, domain.ScoredSubmission{SubmittedAt: row.SubmittedAt, Status: row.Status, Score: row.Score})
	}
	cell := domain.ScoreCell(rules, submissions)
	if cell.Attempts == 0 && cell.PublicAttempts == 0 && cell.PendingCount == 0 {
		return q.DeleteContestCell(ctx, dbgen.DeleteContestCellParams{ContestID: contestID, UserID: userID, ProblemID: problemID})
	}
	return q.UpsertContestCell(ctx, dbgen.UpsertContestCellParams{ContestID: contestID, UserID: userID, ProblemID: problemID, Attempts: cell.Attempts, PenaltySec: cell.PenaltySec, Score: cell.Score, SolvedAt: cell.SolvedAt,
		PublicAttempts: cell.PublicAttempts, PublicPenaltySec: cell.PublicPenaltySec, PublicScore: cell.PublicScore, PublicSolvedAt: cell.PublicSolvedAt, PendingCount: cell.PendingCount, LastSubmitAt: cell.LastSubmitAt})
}

// RebuildContest repairs every affected cell within the existing write transaction.
func RebuildContest(ctx context.Context, tx Execer, contestID string) error {
	targets, err := dbgen.New(tx).ListContestScoringTargets(ctx, contestID)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := RebuildCell(ctx, tx, contestID, target.UserID, target.ProblemID); err != nil {
			return err
		}
	}
	return nil
}
