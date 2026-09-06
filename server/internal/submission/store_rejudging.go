package submission

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/jmoiron/sqlx"
)

// maxRejudgeBatch bounds one batch. A jury that really wants more can run
// several batches; an unbounded one would flood the judge queue and make the
// contest unjudgeable for everyone else.
const maxRejudgeBatch = 5000

// CreateRejudging expands a selector into a batch, bumps every matched
// submission to a new generation and queues fresh judge jobs for them.
//
// The recorded generation is the fence: a member whose current generation has
// moved past the recorded one was taken over by a later rejudging, and this
// batch stops counting it.
func (s *SubmissionStore) CreateRejudging(
	ctx context.Context, selector RejudgeSelector, createdBy string,
) (*Rejudging, error) {
	if selector.IsEmpty() {
		return nil, ErrRejudgeEmpty
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	where, args := rejudgeConditions(ctx, selector)
	// Lock the matched rows in a stable order so two overlapping batches
	// serialize instead of deadlocking.
	rows, err := tx.QueryContext(ctx,
		`SELECT id, status, score, total_time_ms, peak_memory_kb,
		        compile_result, case_results, judged_cases, total_cases, judged_at
		 FROM submissions
		 WHERE `+where+`
		 ORDER BY submitted_at, id
		 LIMIT `+strconv.Itoa(maxRejudgeBatch)+`
		 FOR UPDATE`, args...)
	if err != nil {
		return nil, err
	}
	type member struct {
		id            string
		status        string
		score         int
		totalTimeMs   int
		peakMemoryKB  int
		compileResult string
		caseResults   []byte
		judgedCases   int
		totalCases    int
		judgedAt      *time.Time
	}
	members := make([]member, 0, 64)
	for rows.Next() {
		var item member
		if err := rows.Scan(
			&item.id, &item.status, &item.score, &item.totalTimeMs, &item.peakMemoryKB,
			&item.compileResult, &item.caseResults, &item.judgedCases, &item.totalCases,
			&item.judgedAt,
		); err != nil {
			rows.Close()
			return nil, err
		}
		members = append(members, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, ErrRejudgeEmpty
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

	var batch Rejudging
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO rejudgings (contest_id, problem_id, reason, total_count, created_by, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, contest_id, problem_id, reason, state, total_count, created_by, created_at, finished_at`,
		contestID, problemID, selector.Reason, len(members), creator, domain.ID(ctx)).Scan(
		&batch.ID, &batch.ContestID, &batch.ProblemID, &batch.Reason, &batch.State,
		&batch.TotalCount, &batch.CreatedBy, &batch.CreatedAt, &batch.FinishedAt); err != nil {
		return nil, err
	}

	problemIDs := make(map[string]struct{})
	for _, item := range members {
		generation, problemID, practice, err := requeueSubmission(ctx, tx, item.id)
		if err != nil {
			return nil, err
		}
		if practice {
			problemIDs[problemID] = struct{}{}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rejudging_submissions
			   (rejudging_id, submission_id, generation, prior_status, prior_score,
			    prior_total_time_ms, prior_peak_memory_kb, prior_compile_result,
			    prior_case_results, prior_judged_cases, prior_total_cases, prior_judged_at, domain_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			batch.ID, item.id, generation, item.status, item.score,
			item.totalTimeMs, item.peakMemoryKB, item.compileResult, item.caseResults,
			item.judgedCases, item.totalCases, item.judgedAt, domain.ID(ctx)); err != nil {
			return nil, err
		}
	}
	orderedProblemIDs := make([]string, 0, len(problemIDs))
	for problemID := range problemIDs {
		orderedProblemIDs = append(orderedProblemIDs, problemID)
	}
	sort.Strings(orderedProblemIDs)
	for _, problemID := range orderedProblemIDs {
		if err := problem.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
			return nil, err
		}
	}
	// One notification is enough: the dispatcher cascades waiters as claims
	// succeed, so a batch does not need to wake every worker individually.
	if err := notifyJudgeJob(ctx, tx, batch.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &batch, nil
}

// requeueSubmission resets one submission to Pending under a new generation
// and queues a job for it. It is the shared core of single and batch rejudge.
func requeueSubmission(ctx context.Context, tx *sqlx.Tx, submissionID string) (int, string, bool, error) {
	// Cancel outstanding work first, matching the claim/result lock order.
	if _, err := tx.ExecContext(ctx,
		`UPDATE judge_jobs SET state = 'cancelled', finished_at = now()
		 WHERE submission_id = $1 AND state IN ('queued', 'running')
		 AND EXISTS (SELECT 1 FROM submissions WHERE id = $1 AND domain_id = $2)`, submissionID, domain.ID(ctx)); err != nil {
		return 0, "", false, err
	}

	var generation int
	var problemID, userID string
	var contestID *string
	if err := tx.QueryRowContext(ctx,
		`UPDATE submissions SET status = 'Pending', judged_at = NULL, score = 0,
		                        total_time_ms = 0, peak_memory_kb = 0,
		                        compile_result = '', case_results = '[]'::jsonb,
		                        judged_cases = 0, total_cases = 0,
		                        judge_generation = judge_generation + 1
		 WHERE id = $1 AND domain_id = $2
		 RETURNING judge_generation, problem_id, user_id, contest_id`, submissionID, domain.ID(ctx)).Scan(
		&generation, &problemID, &userID, &contestID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", false, ErrNotFound
		}
		return 0, "", false, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO judge_jobs (submission_id, generation) VALUES ($1, $2)`,
		submissionID, generation); err != nil {
		return 0, "", false, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM submission_cases WHERE submission_id = $1`, submissionID); err != nil {
		return 0, "", false, err
	}
	// A pending submission must not keep contributing to the standings.
	if contestID != nil && *contestID != "" {
		if err := contest.RebuildCell(ctx, tx, *contestID, userID, problemID); err != nil {
			return 0, "", false, err
		}
	}
	practice := contestID == nil || *contestID == ""
	return generation, problemID, practice, nil
}

// rejudgeConditions builds the selector's WHERE clause. Every value is bound,
// never interpolated.
func rejudgeConditions(ctx context.Context, selector RejudgeSelector) (string, []any) {
	// A batch rejudge is defined over completed results. Pending/running work
	// already has a live generation and cannot be restored safely on cancel.
	clauses := []string{"judged_at IS NOT NULL"}
	args := make([]any, 0, 6)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, strings.Replace(clause, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	add("domain_id = ?", domain.ID(ctx))
	if selector.ContestID != "" {
		add("contest_id = ?::uuid", selector.ContestID)
	}
	if selector.ProblemID != "" {
		add("problem_id = ?::uuid", selector.ProblemID)
	}
	if selector.UserID != "" {
		add("user_id = ?::uuid", selector.UserID)
	}
	if selector.Language != "" {
		add("language = ?", selector.Language)
	}
	if selector.Status != "" {
		add("status = ?", selector.Status)
	}
	if len(selector.SubmissionIDs) > 0 {
		add("id = ANY(?::uuid[])", selector.SubmissionIDs)
	}
	return strings.Join(clauses, " AND "), args
}

const rejudgingColumns = `r.id, r.contest_id, r.problem_id, r.reason, r.state, r.total_count,
	r.created_by, r.created_at, r.finished_at,
	(SELECT count(*) FROM rejudging_submissions AS member
	   JOIN submissions AS sub ON sub.id = member.submission_id
	   WHERE member.rejudging_id = r.id
	     AND (sub.judge_generation > member.generation
	          OR (sub.judge_generation = member.generation AND sub.judged_at IS NOT NULL)))::int,
	(SELECT count(*) FROM rejudging_submissions AS member
	   JOIN submissions AS sub ON sub.id = member.submission_id
	   WHERE member.rejudging_id = r.id
	     AND sub.judge_generation = member.generation AND sub.judged_at IS NOT NULL
	     AND (sub.status <> member.prior_status OR sub.score <> member.prior_score))::int`

func scanRejudging(scanner interface{ Scan(...any) error }) (Rejudging, error) {
	var item Rejudging
	err := scanner.Scan(&item.ID, &item.ContestID, &item.ProblemID, &item.Reason,
		&item.State, &item.TotalCount, &item.CreatedBy, &item.CreatedAt, &item.FinishedAt,
		&item.DoneCount, &item.ChangedCount)
	return item, err
}

// Rejudging reads one batch with live progress.
func (s *SubmissionStore) Rejudging(ctx context.Context, id string) (*Rejudging, error) {
	item, err := scanRejudging(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+rejudgingColumns+` FROM rejudgings AS r WHERE r.id = $1 AND r.domain_id = $2`, id, domain.ID(ctx)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRejudgeNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.settleRejudging(ctx, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListRejudgings returns recent batches, newest first, optionally scoped to a
// contest.
func (s *SubmissionStore) ListRejudgings(ctx context.Context, contestID string, limit int) ([]Rejudging, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+rejudgingColumns+` FROM rejudgings AS r
		 WHERE ($1 = '' OR r.contest_id = $1::uuid) AND r.domain_id = $3
		 ORDER BY r.created_at DESC LIMIT $2`, contestID, limit, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Rejudging{}
	for rows.Next() {
		item, err := scanRejudging(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range list {
		if err := s.settleRejudging(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// settleRejudging flips a running batch to finished once every member has been
// judged. Progress is derived, so this is the only state the batch owns.
func (s *SubmissionStore) settleRejudging(ctx context.Context, item *Rejudging) error {
	if item.State != RejudgingRunning || !item.Finished() {
		return nil
	}
	err := s.db.Pool.QueryRowContext(ctx,
		`UPDATE rejudgings SET state = 'finished', finished_at = now()
		 WHERE id = $1 AND state = 'running' AND domain_id = $2
		 RETURNING state, finished_at`, item.ID, domain.ID(ctx)).Scan(&item.State, &item.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Another reader settled it first; the values we already have are fine.
		return nil
	}
	return err
}

// CancelRejudging stops the queued part of a running batch. Every withdrawn
// member gets its exact pre-batch snapshot back; work already leased to a
// worker is allowed to finish so its fenced result still lands consistently.
func (s *SubmissionStore) CancelRejudging(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM rejudgings WHERE id = $1 AND domain_id = $2 FOR UPDATE`, id, domain.ID(ctx)).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRejudgeNotFound
		}
		return err
	}
	if state != RejudgingRunning {
		return ErrRejudgeClosed
	}
	type restoredSubmission struct {
		id        string
		problemID string
		userID    string
		contestID *string
	}
	rows, err := tx.QueryContext(ctx,
		`WITH cancelled AS (
		   UPDATE judge_jobs AS job
		   SET state = 'cancelled', finished_at = now()
		   FROM rejudging_submissions AS member
		   WHERE member.rejudging_id = $1
		     AND job.submission_id = member.submission_id
		     AND job.generation = member.generation
		     AND job.state = 'queued'
		   RETURNING job.submission_id, job.generation
		 )
		 UPDATE submissions AS sub
		 SET status = member.prior_status,
		     score = member.prior_score,
		     total_time_ms = member.prior_total_time_ms,
		     peak_memory_kb = member.prior_peak_memory_kb,
		     compile_result = member.prior_compile_result,
		     case_results = member.prior_case_results,
		     judged_cases = member.prior_judged_cases,
		     total_cases = member.prior_total_cases,
		     judged_at = member.prior_judged_at
		 FROM rejudging_submissions AS member
		 JOIN cancelled
		   ON cancelled.submission_id = member.submission_id
		  AND cancelled.generation = member.generation
		 WHERE member.rejudging_id = $1
		   AND sub.id = member.submission_id
		   AND sub.judge_generation = member.generation
		 RETURNING sub.id, sub.problem_id, sub.user_id, sub.contest_id`, id)
	if err != nil {
		return err
	}
	restored := make([]restoredSubmission, 0)
	for rows.Next() {
		var item restoredSubmission
		if err := rows.Scan(&item.id, &item.problemID, &item.userID, &item.contestID); err != nil {
			rows.Close()
			return err
		}
		restored = append(restored, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// submission_cases mirrors the JSON snapshot for indexed/operational reads.
	// Restore it as well so cancellation never leaves two representations of a
	// verdict disagreeing.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM submission_cases AS result
		 USING rejudging_submissions AS member, judge_jobs AS job, submissions AS sub
		 WHERE member.rejudging_id = $1
		   AND job.submission_id = member.submission_id
		   AND job.generation = member.generation
		   AND job.state = 'cancelled'
		   AND sub.id = member.submission_id
		   AND sub.judge_generation = member.generation
		   AND result.submission_id = member.submission_id`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO submission_cases
		   (submission_id, case_index, verdict, time_ms, memory_kb, exit_status, checker_output)
		 SELECT member.submission_id,
		        (item.value->>'caseIndex')::integer,
		        item.value->>'verdict',
		        COALESCE((item.value->>'timeMs')::integer, 0),
		        COALESCE((item.value->>'memoryKb')::integer, 0),
		        COALESCE(item.value->>'exitStatus', ''),
		        COALESCE(item.value->>'checkerOutput', '')
		 FROM rejudging_submissions AS member
		 JOIN judge_jobs AS job
		   ON job.submission_id = member.submission_id
		  AND job.generation = member.generation
		  AND job.state = 'cancelled'
		 JOIN submissions AS sub
		   ON sub.id = member.submission_id
		  AND sub.judge_generation = member.generation
		 CROSS JOIN LATERAL jsonb_array_elements(member.prior_case_results) AS item(value)
		 WHERE member.rejudging_id = $1`, id); err != nil {
		return err
	}

	practiceProblems := make(map[string]struct{})
	type cell struct{ contestID, userID, problemID string }
	contestCells := make(map[string]cell)
	for _, item := range restored {
		if item.contestID == nil || *item.contestID == "" {
			practiceProblems[item.problemID] = struct{}{}
			continue
		}
		key := *item.contestID + "\x00" + item.userID + "\x00" + item.problemID
		contestCells[key] = cell{contestID: *item.contestID, userID: item.userID, problemID: item.problemID}
	}
	practiceKeys := make([]string, 0, len(practiceProblems))
	for problemID := range practiceProblems {
		practiceKeys = append(practiceKeys, problemID)
	}
	sort.Strings(practiceKeys)
	for _, problemID := range practiceKeys {
		if err := problem.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
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
		if err := contest.RebuildCell(ctx, tx, item.contestID, item.userID, item.problemID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE rejudgings SET state = 'cancelled', finished_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// RejudgingChanges lists the members whose verdict moved, which is the review
// a jury performs before announcing a corrected scoreboard.
func (s *SubmissionStore) RejudgingChanges(ctx context.Context, id string, limit int) ([]RejudgingChange, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT member.submission_id, u.username, p.title,
		        member.prior_status, member.prior_score,
		        sub.status, sub.score,
		        (sub.judge_generation = member.generation AND sub.judged_at IS NOT NULL)
		 FROM rejudging_submissions AS member
		 JOIN submissions AS sub ON sub.id = member.submission_id
		 JOIN users AS u ON u.id = sub.user_id
		 JOIN problems AS p ON p.id = sub.problem_id
		 WHERE member.rejudging_id = $1 AND member.domain_id = $3
		   AND (sub.status <> member.prior_status OR sub.score <> member.prior_score)
		 ORDER BY sub.submitted_at
		 LIMIT $2`, id, limit, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []RejudgingChange{}
	for rows.Next() {
		var item RejudgingChange
		if err := rows.Scan(&item.SubmissionID, &item.Username, &item.ProblemTitle,
			&item.PriorStatus, &item.PriorScore, &item.Status, &item.Score, &item.Judged); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}
