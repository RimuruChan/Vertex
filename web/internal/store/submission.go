package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vertex-oj/web/internal/model"
)

// SubmissionStore 负责 submissions 表读写与判题队列领取。
type SubmissionStore struct{ db *DB }

func NewSubmissionStore(db *DB) *SubmissionStore { return &SubmissionStore{db: db} }

// 判题状态常量(与 migrations 中的 CHECK 约束一致)。
const (
	StatusPending       = "Pending"
	StatusJudging       = "Judging"
	StatusAccepted      = "Accepted"
	StatusWrongAnswer   = "Wrong Answer"
	StatusTLE           = "Time Limit Exceeded"
	StatusMLE           = "Memory Limit Exceeded"
	StatusRE            = "Runtime Error"
	StatusCE            = "Compile Error"
	StatusOLE           = "Output Limit Exceeded"
	StatusSE            = "System Error"
	StatusSkipped       = "Skipped"
)

// SubmissionFilters 提交列表筛选。
type SubmissionFilters struct {
	UserID    string
	ProblemID string
	ContestID string
	Language  string
	Status    string
	Limit     int
	Offset    int
}

// Create 插入一条 Pending 提交,返回带 ID 的完整行。
func (s *SubmissionStore) Create(ctx context.Context, sub *model.Submission) (*model.Submission, error) {
	var contestID *string
	if sub.ContestID != nil {
		contestID = sub.ContestID
	}
	row := s.db.Pool.QueryRow(ctx,
		`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id)
		 VALUES ($1, $2, $3, $4, 'Pending', $5)
		 RETURNING id, user_id, problem_id, language, source_code, status, score,
		           total_time_ms, peak_memory_kb, compile_result, contest_id, submitted_at, judged_at`,
		sub.UserID, sub.ProblemID, sub.Language, sub.SourceCode, contestID,
	)
	return scanSubmission(row)
}

// Get 取单条提交,join 出用户名与题目标题。
func (s *SubmissionStore) Get(ctx context.Context, id string) (*model.Submission, error) {
	row := s.db.Pool.QueryRow(ctx,
		`SELECT s.id, s.user_id, u.username, s.problem_id, p.title,
		        s.language, s.source_code, s.status, s.score,
		        s.total_time_ms, s.peak_memory_kb, s.compile_result,
		        s.case_results, s.contest_id, s.submitted_at, s.judged_at
		 FROM submissions s
		 JOIN users u ON u.id = s.user_id
		 JOIN problems p ON p.id = s.problem_id
		 WHERE s.id = $1`, id,
	)
	var (
		sub         model.Submission
		caseResults []byte
	)
	err := row.Scan(&sub.ID, &sub.UserID, &sub.Username, &sub.ProblemID, &sub.ProblemTitle,
		&sub.Language, &sub.SourceCode, &sub.Status, &sub.Score,
		&sub.TotalTimeMs, &sub.PeakMemoryKb, &sub.CompileResult,
		&caseResults, &sub.ContestID, &sub.SubmittedAt, &sub.JudgedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := jsonUnmarshal(caseResults, &sub.CaseResults); err != nil {
		return nil, err
	}
	return &sub, nil
}

// List 分页查询提交列表(不含源码与逐测试点详情)。
func (s *SubmissionStore) List(ctx context.Context, f SubmissionFilters) ([]model.Submission, int, error) {
	clauses := []string{"1=1"}
	args := []any{}
	add := func(clause string, val any) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if f.UserID != "" {
		add("s.user_id = $%d", f.UserID)
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
	if err := s.db.Pool.QueryRow(ctx, "SELECT count(*) FROM submissions s "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	limitIdx, offsetIdx := len(args)-1, len(args)
	query := `SELECT s.id, s.user_id, u.username, s.problem_id, p.title,
	                 s.language, s.status, s.score,
	                 s.total_time_ms, s.peak_memory_kb, s.contest_id, s.submitted_at
	          FROM submissions s
	          JOIN users u ON u.id = s.user_id
	          JOIN problems p ON p.id = s.problem_id
	          ` + where + fmt.Sprintf(" ORDER BY s.submitted_at DESC LIMIT $%d OFFSET $%d", limitIdx, offsetIdx)

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []model.Submission{}
	for rows.Next() {
		var sub model.Submission
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Username, &sub.ProblemID, &sub.ProblemTitle,
			&sub.Language, &sub.Status, &sub.Score,
			&sub.TotalTimeMs, &sub.PeakMemoryKb, &sub.ContestID, &sub.SubmittedAt); err != nil {
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

// NextPending 用 SKIP LOCKED 原子领取一条 Pending 提交。
// 返回 nil 表示暂无待判提交。查询超时由 ctx 控制,避免 worker 空转占用连接。
func (s *SubmissionStore) NextPending(ctx context.Context) (*model.Submission, error) {
	row := s.db.Pool.QueryRow(ctx,
		`UPDATE submissions
		 SET status = 'Judging'
		 WHERE id = (
		   SELECT id FROM submissions
		   WHERE status = 'Pending'
		   ORDER BY submitted_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 )
		 RETURNING id, user_id, problem_id, language, source_code, status, score,
		           total_time_ms, peak_memory_kb, compile_result, contest_id, submitted_at, judged_at`,
	)
	sub, err := scanSubmission(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// SetJudgingFailed 领取后系统异常时把提交退回 Pending(允许重试)。
func (s *SubmissionStore) SetJudgingFailed(ctx context.Context, id string, reason string) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE submissions SET status = 'Pending' WHERE id = $1`, id)
	return err
}

// MarkJudged 写入最终判定结果与逐测试点数据(同一事务)。
func (s *SubmissionStore) MarkJudged(ctx context.Context, sub *model.Submission) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	caseJSON, err := jsonMarshal(sub.CaseResults)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE submissions SET
		   status = $2, score = $3, total_time_ms = $4, peak_memory_kb = $5,
		   compile_result = $6, case_results = $7, judged_at = now()
		 WHERE id = $1`,
		sub.ID, sub.Status, sub.Score, sub.TotalTimeMs, sub.PeakMemoryKb,
		sub.CompileResult, string(caseJSON),
	)
	if err != nil {
		return err
	}

	// 全量重写逐测试点行(幂等;rejudge 时覆盖旧结果)
	if _, err := tx.Exec(ctx, `DELETE FROM submission_cases WHERE submission_id = $1`, sub.ID); err != nil {
		return err
	}
	for _, c := range sub.CaseResults {
		if _, err := tx.Exec(ctx,
			`INSERT INTO submission_cases (submission_id, case_index, verdict, time_ms, memory_kb, exit_status, checker_output)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sub.ID, c.CaseIndex, c.Verdict, c.TimeMs, c.MemoryKb, c.ExitStatus, c.CheckerOutput,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// UpdateCounters 在判定完成后更新题目/用户的解题计数。
func (s *SubmissionStore) UpdateCounters(ctx context.Context, problemID string, accepted bool) error {
	if accepted {
		_, err := s.db.Pool.Exec(ctx,
			`UPDATE problems SET submission_count = submission_count + 1,
			                      accepted_count = accepted_count + 1
			 WHERE id = $1`, problemID)
		return err
	}
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE problems SET submission_count = submission_count + 1 WHERE id = $1`, problemID)
	return err
}

// Rejudge 把提交重置回 Pending 并清空旧结果(worker 会重新判定)。
func (s *SubmissionStore) Rejudge(ctx context.Context, id string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE submissions SET status = 'Pending', judged_at = NULL, score = 0,
		                        total_time_ms = 0, peak_memory_kb = 0,
		                        compile_result = '', case_results = '[]'::jsonb
		 WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM submission_cases WHERE submission_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanSubmission(row pgx.Row) (*model.Submission, error) {
	var (
		sub         model.Submission
		caseResults []byte
	)
	err := row.Scan(&sub.ID, &sub.UserID, &sub.ProblemID, &sub.Language, &sub.SourceCode,
		&sub.Status, &sub.Score, &sub.TotalTimeMs, &sub.PeakMemoryKb, &sub.CompileResult,
		&sub.ContestID, &sub.SubmittedAt, &sub.JudgedAt)
	if err != nil {
		return nil, err
	}
	if len(caseResults) > 0 {
		if err := jsonUnmarshal(caseResults, &sub.CaseResults); err != nil {
			return nil, err
		}
	}
	return &sub, nil
}
