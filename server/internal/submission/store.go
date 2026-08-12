package submission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jmoiron/sqlx"
)

// SubmissionStore owns submission creation, listing and rejudge transactions.
type SubmissionStore struct{ db *database.DB }

func NewSubmissionStore(db *database.DB) *SubmissionStore { return &SubmissionStore{db: db} }

// 判题状态常量(与 migrations 中的 CHECK 约束一致)。
const (
	StatusPending     = "Pending"
	StatusJudging     = "Judging"
	StatusAccepted    = "Accepted"
	StatusWrongAnswer = "Wrong Answer"
	StatusTLE         = "Time Limit Exceeded"
	StatusMLE         = "Memory Limit Exceeded"
	StatusRE          = "Runtime Error"
	StatusCE          = "Compile Error"
	StatusOLE         = "Output Limit Exceeded"
	StatusSE          = "System Error"
	StatusSkipped     = "Skipped"
)

// Create 插入一条 Pending 提交,返回带 ID 的完整行。
func (s *SubmissionStore) Create(ctx context.Context, sub *Submission) (*Submission, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var contestID *string
	if sub.ContestID != nil {
		contestID = sub.ContestID
	}
	row := tx.QueryRowContext(ctx,
		`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id)
		 VALUES ($1, $2, $3, $4, 'Pending', $5)
		 RETURNING id, user_id, problem_id, language, source_code, status, score,
		           total_time_ms, peak_memory_kb, compile_result, contest_id, submitted_at, judged_at`,
		sub.UserID, sub.ProblemID, sub.Language, sub.SourceCode, contestID,
	)
	created, err := scanSubmission(row)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO judge_jobs (submission_id, generation) VALUES ($1, 1)`, created.ID); err != nil {
		return nil, err
	}
	if err := notifyJudgeJob(ctx, tx, created.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// Get 取单条提交,join 出用户名与题目标题。
func (s *SubmissionStore) Get(ctx context.Context, id string) (*Submission, error) {
	row := s.db.Pool.QueryRowContext(ctx,
		`SELECT s.id, s.user_id, u.username, s.problem_id, p.title,
		        s.language, s.source_code, s.status, s.score,
		        s.total_time_ms, s.peak_memory_kb, s.compile_result,
		        s.case_results, s.judged_cases, s.total_cases,
		        s.contest_id, s.submitted_at, s.judged_at
		 FROM submissions s
		 JOIN users u ON u.id = s.user_id
		 JOIN problems p ON p.id = s.problem_id
		 WHERE s.id = $1`, id,
	)
	var (
		sub         Submission
		caseResults []byte
	)
	err := row.Scan(&sub.ID, &sub.UserID, &sub.Username, &sub.ProblemID, &sub.ProblemTitle,
		&sub.Language, &sub.SourceCode, &sub.Status, &sub.Score,
		&sub.TotalTimeMs, &sub.PeakMemoryKb, &sub.CompileResult,
		&caseResults, &sub.JudgedCases, &sub.TotalCases,
		&sub.ContestID, &sub.SubmittedAt, &sub.JudgedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(caseResults, &sub.CaseResults); err != nil {
		return nil, err
	}
	return &sub, nil
}

// List 分页查询提交列表(不含源码与逐测试点详情)。
func (s *SubmissionStore) List(ctx context.Context, f Filters) ([]Submission, int, error) {
	clauses := []string{"1=1"}
	args := []any{}
	add := func(clause string, val any) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if f.UserID != "" {
		add("(s.user_id::text = $%[1]d OR u.username = $%[1]d)", f.UserID)
	}
	if f.ProblemID != "" {
		add("s.problem_id = $%d", f.ProblemID)
	}
	if f.ContestID != "" {
		add("s.contest_id = $%d", f.ContestID)
	}
	if f.Language != "" {
		add("s.language = $%d", f.Language)
	}
	if f.Status != "" {
		add("s.status = $%d", f.Status)
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	where := "WHERE " + joinClauses(clauses)

	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		"SELECT count(*) FROM submissions s JOIN users u ON u.id = s.user_id "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	limitIdx, offsetIdx := len(args)-1, len(args)
	query := `SELECT s.id, s.user_id, u.username, s.problem_id, p.title,
	                 s.language, s.status, s.score,
	                 s.total_time_ms, s.peak_memory_kb,
	                 s.judged_cases, s.total_cases, s.contest_id, s.submitted_at
	          FROM submissions s
	          JOIN users u ON u.id = s.user_id
	          JOIN problems p ON p.id = s.problem_id
	          ` + where + fmt.Sprintf(" ORDER BY s.submitted_at DESC LIMIT $%d OFFSET $%d", limitIdx, offsetIdx)

	rows, err := s.db.Pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Submission{}
	for rows.Next() {
		var sub Submission
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Username, &sub.ProblemID, &sub.ProblemTitle,
			&sub.Language, &sub.Status, &sub.Score,
			&sub.TotalTimeMs, &sub.PeakMemoryKb,
			&sub.JudgedCases, &sub.TotalCases, &sub.ContestID, &sub.SubmittedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, sub)
	}
	return list, total, rows.Err()
}

func joinClauses(clauses []string) string {
	out := ""
	for i, c := range clauses {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}

// Rejudge 把提交重置回 Pending 并清空旧结果(worker 会重新判定)。
func (s *SubmissionStore) Rejudge(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock jobs before the submission, matching the claim/result lock order.
	if _, err := tx.ExecContext(ctx,
		`UPDATE judge_jobs SET state = 'cancelled', finished_at = now()
		 WHERE submission_id = $1 AND state IN ('queued', 'running')`, id); err != nil {
		return err
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
		 WHERE id = $1
		 RETURNING judge_generation, problem_id, user_id, contest_id`, id).Scan(
		&generation, &problemID, &userID, &contestID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO judge_jobs (submission_id, generation) VALUES ($1, $2)`, id, generation); err != nil {
		return err
	}
	if err := notifyJudgeJob(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM submission_cases WHERE submission_id = $1`, id); err != nil {
		return err
	}
	if err := rebuildProblemCounters(ctx, tx, problemID); err != nil {
		return err
	}
	if contestID != nil && *contestID != "" {
		if err := rebuildContestCell(ctx, tx, *contestID, userID, problemID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubmission(row rowScanner) (*Submission, error) {
	var sub Submission
	err := row.Scan(&sub.ID, &sub.UserID, &sub.ProblemID, &sub.Language, &sub.SourceCode,
		&sub.Status, &sub.Score, &sub.TotalTimeMs, &sub.PeakMemoryKb, &sub.CompileResult,
		&sub.ContestID, &sub.SubmittedAt, &sub.JudgedAt)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

type execContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func notifyJudgeJob(ctx context.Context, tx execContext, submissionID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_notify('vertex_judge_jobs', $1)`, submissionID)
	return err
}

func rebuildProblemCounters(ctx context.Context, tx *sqlx.Tx, problemID string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "problem:"+problemID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE problems SET
		   submission_count = (SELECT count(*) FROM submissions WHERE problem_id = $1 AND judged_at IS NOT NULL),
		   accepted_count = (SELECT count(*) FROM submissions WHERE problem_id = $1 AND status = 'Accepted'),
		   solved_user_count = (SELECT count(DISTINCT user_id) FROM submissions WHERE problem_id = $1 AND status = 'Accepted')
		 WHERE id = $1`, problemID)
	return err
}

func rebuildContestCell(ctx context.Context, tx *sqlx.Tx, contestID, userID, problemID string) error {
	lockKey := contestID + ":" + userID + ":" + problemID
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM contest_submission_cells
		 WHERE contest_id = $1 AND user_id = $2 AND problem_id = $3`,
		contestID, userID, problemID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`WITH contest_window AS (
		   SELECT begin_at, end_at FROM contests WHERE id = $1
		 ), eligible AS (
		   SELECT sub.status, sub.submitted_at, bounds.begin_at,
		          min(sub.submitted_at) FILTER (WHERE sub.status = 'Accepted') OVER () AS first_ac
		   FROM submissions AS sub CROSS JOIN contest_window AS bounds
		   WHERE sub.contest_id = $1 AND sub.user_id = $2 AND sub.problem_id = $3
		     AND sub.submitted_at BETWEEN bounds.begin_at AND bounds.end_at
		     AND sub.status NOT IN ('Pending', 'Judging', 'System Error')
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
