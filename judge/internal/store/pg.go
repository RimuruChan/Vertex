package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vertex-oj/judge/internal/executor"
	"github.com/vertex-oj/judge/internal/scheduler"
)

// PGStore 基于 Postgres 的判题 worker 数据访问。
type PGStore struct {
	Pool *pgxpool.Pool
	// TestdataRoot 测试数据卷根目录(与 web 共享),问题数据在 <root>/<problemID>/
	TestdataRoot string
}

func New(pool *pgxpool.Pool, testdataRoot string) *PGStore {
	return &PGStore{Pool: pool, TestdataRoot: testdataRoot}
}

// ClaimNext 用 SKIP LOCKED 原子领取 Pending 提交，并回收崩溃 Worker 遗留的过期租约。
func (s *PGStore) ClaimNext(ctx context.Context) (*scheduler.Submission, error) {
	var sub scheduler.Submission
	err := s.Pool.QueryRow(ctx,
		`UPDATE submissions SET status = 'Judging', judge_started_at = now()
		 WHERE id = (
		   SELECT id FROM submissions
		   WHERE status = 'Pending'
		      OR (status = 'Judging' AND judge_started_at < now() - interval '10 minutes')
		   ORDER BY submitted_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 )
		 RETURNING id, user_id, problem_id, language, source_code, contest_id`,
	).Scan(&sub.ID, &sub.UserID, &sub.ProblemID, &sub.Language, &sub.SourceCode, &sub.ContestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// GetProblemLimits 取题目限值。
func (s *PGStore) GetProblemLimits(ctx context.Context, problemID string) (*scheduler.ProblemLimits, error) {
	var l scheduler.ProblemLimits
	err := s.Pool.QueryRow(ctx,
		`SELECT time_limit_ms, memory_limit_kb FROM problems WHERE id = $1`, problemID,
	).Scan(&l.TimeLimitMs, &l.MemLimitKB)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("problem %s not found", problemID)
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// GetTestdataDir 返回测试数据目录。目录结构:<root>/<problemID>/1.in, 1.out, ...
// 目录缺失时尝试从 DB 的 storage_path 解析。
func (s *PGStore) GetTestdataDir(ctx context.Context, problemID string) (*scheduler.Testdata, error) {
	var (
		storagePath string
		caseCount   int
	)
	err := s.Pool.QueryRow(ctx,
		`SELECT storage_path, case_count FROM problem_testdata WHERE problem_id = $1`, problemID,
	).Scan(&storagePath, &caseCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("testdata for problem %s not found", problemID)
	}
	if err != nil {
		return nil, err
	}

	dir := ""
	if storagePath != "" {
		if filepath.IsAbs(storagePath) {
			dir = storagePath
		} else {
			dir = filepath.Join(s.TestdataRoot, storagePath)
		}
	} else {
		dir = filepath.Join(s.TestdataRoot, problemID)
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("testdata dir %s missing: %w", dir, err)
	}
	return &scheduler.Testdata{Dir: dir, CaseCount: caseCount}, nil
}

// MarkResult 写最终判定结果(同一事务,幂等)。
func (s *PGStore) MarkResult(ctx context.Context, sub *scheduler.Submission, status string, score int,
	totalTime int64, peakMem int, compileResult string, cases []executor.CaseResult) error {

	caseJSON, _ := json.Marshal(cases)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`UPDATE submissions SET
		   status = $2, score = $3, total_time_ms = $4, peak_memory_kb = $5,
		   compile_result = $6, case_results = $7, judged_at = now(), judge_started_at = NULL
		 WHERE id = $1`,
		sub.ID, status, score, totalTime, peakMem, compileResult, string(caseJSON))
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM submission_cases WHERE submission_id = $1`, sub.ID); err != nil {
		return err
	}
	for _, c := range cases {
		if _, err := tx.Exec(ctx,
			`INSERT INTO submission_cases (submission_id, case_index, verdict, time_ms, memory_kb, exit_status, checker_output)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sub.ID, c.CaseIndex, c.Verdict, c.TimeMs, c.MemoryKb, c.ExitStatus, c.CheckerOutput,
		); err != nil {
			return err
		}
	}

	// 从 submissions 事实表重算计数。这样 rejudge 改判不会重复累加，
	// solved_user_count 也始终与当前 Accepted 用户集合一致。
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "problem:"+sub.ProblemID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE problems SET
		   submission_count = (SELECT count(*) FROM submissions WHERE problem_id = $1 AND judged_at IS NOT NULL),
		   accepted_count = (SELECT count(*) FROM submissions WHERE problem_id = $1 AND status = 'Accepted'),
		   solved_user_count = (SELECT count(DISTINCT user_id) FROM submissions WHERE problem_id = $1 AND status = 'Accepted')
		 WHERE id = $1`, sub.ProblemID); err != nil {
		return err
	}

	// 比赛积分格同样从当前提交事实重建。advisory lock 串行化同一用户/题目的
	// 并发完成，确保 rejudge、改判和重复回写都得到同一个结果。
	if sub.ContestID != nil && *sub.ContestID != "" {
		if err := rebuildContestCell(ctx, tx, *sub.ContestID, sub.UserID, sub.ProblemID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func rebuildContestCell(ctx context.Context, tx pgx.Tx, contestID, userID, problemID string) error {
	lockKey := contestID + ":" + userID + ":" + problemID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM contest_submission_cells
		 WHERE contest_id = $1 AND user_id = $2 AND problem_id = $3`,
		contestID, userID, problemID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`WITH contest_window AS (
		   SELECT begin_at, end_at FROM contests WHERE id = $1
		 ), eligible AS (
		   SELECT s.status, s.submitted_at, cw.begin_at,
		          min(s.submitted_at) FILTER (WHERE s.status = 'Accepted') OVER () AS first_ac
		   FROM submissions s CROSS JOIN contest_window cw
		   WHERE s.contest_id = $1 AND s.user_id = $2 AND s.problem_id = $3
		     AND s.submitted_at BETWEEN cw.begin_at AND cw.end_at
		     AND s.status NOT IN ('Pending', 'Judging', 'System Error')
		 ), aggregate AS (
		   SELECT count(*) FILTER (WHERE first_ac IS NULL OR submitted_at <= first_ac)::int AS attempts,
		          min(first_ac) AS solved_at,
		          count(*) FILTER (
		            WHERE status <> 'Accepted' AND (first_ac IS NULL OR submitted_at < first_ac)
		          )::int AS failed_attempts,
		          min(begin_at) AS begin_at
		   FROM eligible
		 )
		 INSERT INTO contest_submission_cells
		   (contest_id, user_id, problem_id, attempts, penalty_sec, solved_at, pending_count)
		 SELECT $1, $2, $3, attempts,
		        CASE WHEN solved_at IS NULL THEN 0
		             ELSE extract(epoch FROM (solved_at - begin_at))::int + failed_attempts * 1200 END,
		        solved_at, 0
		 FROM aggregate WHERE attempts > 0`,
		contestID, userID, problemID)
	return err
}

// Requeue 提交失败后退回队列。
func (s *PGStore) Requeue(ctx context.Context, subID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE submissions SET status = 'Pending', judge_started_at = NULL WHERE id = $1`, subID)
	return err
}
