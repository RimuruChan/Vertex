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

// ClaimNext 用 SKIP LOCKED 原子领取一条 Pending 提交。
func (s *PGStore) ClaimNext(ctx context.Context) (*scheduler.Submission, error) {
	var sub scheduler.Submission
	err := s.Pool.QueryRow(ctx,
		`UPDATE submissions SET status = 'Judging'
		 WHERE id = (
		   SELECT id FROM submissions
		   WHERE status = 'Pending'
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

	// 先读旧状态,决定是否计次(避免 rejudge 重复计数)
	var prevStatus string
	err = tx.QueryRow(ctx, `SELECT status FROM submissions WHERE id = $1`, sub.ID).Scan(&prevStatus)
	if err != nil {
		return err
	}
	isNewJudge := !isFinalStatus(prevStatus)
	wasAccepted := prevStatus == "Accepted"

	_, err = tx.Exec(ctx,
		`UPDATE submissions SET
		   status = $2, score = $3, total_time_ms = $4, peak_memory_kb = $5,
		   compile_result = $6, case_results = $7, judged_at = now()
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

	// 题目计数:仅当从非终态变为终态时递增 submission_count;
	// accepted_count 仅当本次 AC 且旧状态非 AC 时递增。
	if isNewJudge {
		if _, err := tx.Exec(ctx,
			`UPDATE problems SET submission_count = submission_count + 1 WHERE id = $1`, sub.ProblemID); err != nil {
			return err
		}
	}
	if status == "Accepted" && !wasAccepted {
		if _, err := tx.Exec(ctx,
			`UPDATE problems SET accepted_count = accepted_count + 1 WHERE id = $1`, sub.ProblemID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// isFinalStatus 判断状态是否为终态。
func isFinalStatus(status string) bool {
	switch status {
	case "Pending", "Judging":
		return false
	default:
		return true
	}
}

// Requeue 提交失败后退回队列。
func (s *PGStore) Requeue(ctx context.Context, subID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE submissions SET status = 'Pending' WHERE id = $1`, subID)
	return err
}
