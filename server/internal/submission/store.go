package submission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/problem"
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
	if err := validateSubmissionTarget(ctx, tx, sub); err != nil {
		return nil, err
	}
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

// validateSubmissionTarget repeats the service's access decision under the
// same transaction that creates the submission and judge job. The row locks
// close the gap where a contest could end, lose a problem, or revoke a
// participant after the service check but before persistence.
func validateSubmissionTarget(ctx context.Context, tx *sqlx.Tx, sub *Submission) error {
	if sub.ContestID == nil || *sub.ContestID == "" {
		var visibility, role string
		err := tx.QueryRowContext(ctx,
			`SELECT problem.visibility, usr.role
			 FROM problems AS problem
			 JOIN users AS usr ON usr.id = $2::uuid
			 WHERE problem.id = $1::uuid
			 FOR SHARE OF problem, usr`, sub.ProblemID, sub.UserID).Scan(&visibility, &role)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if visibility != "public" && role != "admin" {
			return ErrProblemForbidden
		}
		return nil
	}

	var active, privileged bool
	var visibility string
	err := tx.QueryRowContext(ctx,
		`SELECT now() >= contest.begin_at AND now() <= contest.end_at,
		        contest.visibility,
		        usr.role = 'admin' OR COALESCE(contest.created_by = $2::uuid, FALSE)
		 FROM contests AS contest
		 JOIN users AS usr ON usr.id = $2::uuid
		 WHERE contest.id = $1::uuid
		 FOR SHARE OF contest, usr`, *sub.ContestID, sub.UserID).Scan(&active, &visibility, &privileged)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !active {
		return contest.ErrNotActive
	}
	authorized := privileged
	if !authorized {
		var present int
		err = tx.QueryRowContext(ctx,
			`SELECT 1 FROM contest_staff
			 WHERE contest_id = $1::uuid AND user_id = $2::uuid
			 FOR SHARE`, *sub.ContestID, sub.UserID).Scan(&present)
		if err == nil {
			authorized = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if !authorized && visibility == "private" {
		return ErrNotFound
	}
	if !authorized {
		var present int
		err = tx.QueryRowContext(ctx,
			`SELECT 1 FROM contest_participants
			 WHERE contest_id = $1::uuid AND user_id = $2::uuid
			 FOR SHARE`, *sub.ContestID, sub.UserID).Scan(&present)
		if err == nil {
			authorized = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if !authorized {
		return contest.ErrNotParticipant
	}

	// Problem deletion locks the problem before cascading into
	// contest_problems; use the same order to avoid a delete/submit deadlock.
	var present int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM problems WHERE id = $1::uuid FOR SHARE`, sub.ProblemID).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return contest.ErrProblemNotInContest
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM contest_problems
		 WHERE contest_id = $1::uuid AND problem_id = $2::uuid
		 FOR SHARE`, *sub.ContestID, sub.ProblemID).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return contest.ErrProblemNotInContest
	}
	return err
}

// Get 取单条可见提交,join 出用户名与题目标题。
func (s *SubmissionStore) Get(ctx context.Context, id string, viewer Viewer) (*Submission, error) {
	args := []any{id}
	visibility := appendViewerVisibility(&args, viewer)
	row := s.db.Pool.QueryRowContext(ctx,
		`SELECT s.id, s.user_id, u.username, s.problem_id, p.title,
		        s.language, s.source_code, s.status, s.score,
		        s.total_time_ms, s.peak_memory_kb, s.compile_result,
		        s.case_results, s.judged_cases, s.total_cases,
		        s.contest_id, s.submitted_at, s.judged_at
		 FROM submissions s
		 JOIN users u ON u.id = s.user_id
		 JOIN problems p ON p.id = s.problem_id
		 WHERE s.id = $1 AND `+visibility, args...,
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

// Progress reads only fields needed by the polling contract and its access
// policy. It joins the problem solely for visibility and never reads source.
func (s *SubmissionStore) Progress(ctx context.Context, id string, viewer Viewer) (*SubmissionProgress, error) {
	args := []any{id}
	visibility := appendViewerVisibility(&args, viewer)
	var item SubmissionProgress
	var caseResults []byte
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT s.id, s.user_id, s.contest_id, s.status, s.score,
		        s.total_time_ms, s.peak_memory_kb, s.compile_result,
		        s.case_results, s.judged_cases, s.total_cases
		 FROM submissions s
		 JOIN problems p ON p.id = s.problem_id
		 WHERE s.id = $1 AND `+visibility, args...,
	).Scan(&item.ID, &item.UserID, &item.ContestID, &item.Status, &item.Score,
		&item.TotalTimeMs, &item.PeakMemoryKb, &item.CompileResult,
		&caseResults, &item.JudgedCases, &item.TotalCases)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(caseResults, &item.CaseResults); err != nil {
		return nil, err
	}
	return &item, nil
}

// List 分页查询查看者可见的提交(不含源码与逐测试点详情)。
func (s *SubmissionStore) List(ctx context.Context, f Filters, viewer Viewer) ([]Submission, int, error) {
	clauses := []string{}
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
	clauses = append(clauses, appendViewerVisibility(&args, viewer))
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	where := "WHERE " + joinClauses(clauses)

	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		"SELECT count(*) FROM submissions s JOIN users u ON u.id = s.user_id JOIN problems p ON p.id = s.problem_id "+where,
		args...).Scan(&total); err != nil {
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
	          ` + where + fmt.Sprintf(" ORDER BY s.submitted_at DESC, s.id DESC LIMIT $%d OFFSET $%d", limitIdx, offsetIdx)

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

// appendViewerVisibility appends the two bound values used by the shared
// list/detail/progress predicate. Contest submissions stay private to their
// owner and staff while a round is live or its board is frozen/hidden. Once a
// visible board is both finished and unfrozen, readers get only the problem
// metadata the contest itself would reveal to them. Keeping the decision in
// SQL makes totals, pages and detail reads agree.
func appendViewerVisibility(args *[]any, viewer Viewer) string {
	var viewerID any
	if viewer.UserID != "" {
		viewerID = viewer.UserID
	}
	*args = append(*args, viewerID, viewer.Admin)
	viewerIndex, adminIndex := len(*args)-1, len(*args)
	return fmt.Sprintf(`(
		$%[2]d
		OR s.user_id = $%[1]d::uuid
		OR (
			s.contest_id IS NULL
			AND (p.visibility = 'public' OR p.author_id = $%[1]d::uuid)
		)
		OR EXISTS (
			SELECT 1 FROM contests c
			WHERE c.id = s.contest_id
			  AND (
				c.created_by = $%[1]d::uuid
				OR EXISTS (
					SELECT 1 FROM contest_staff staff
					WHERE staff.contest_id = c.id AND staff.user_id = $%[1]d::uuid
				)
				OR (
					c.end_at < now()
					AND c.rankboard_visible
					AND NOT (
						c.freeze_at IS NOT NULL
						AND now() > c.freeze_at
						AND (c.unfreeze_at IS NULL OR now() < c.unfreeze_at)
					)
					AND (
						(c.visibility = 'public' AND (
							p.visibility = 'public'
							OR EXISTS (
								SELECT 1 FROM contest_participants participant
								WHERE participant.contest_id = c.id
								  AND participant.user_id = $%[1]d::uuid
							)
						))
						OR (c.visibility = 'password' AND EXISTS (
							SELECT 1 FROM contest_participants participant
							WHERE participant.contest_id = c.id
							  AND participant.user_id = $%[1]d::uuid
						))
					)
				)
			  )
		)
	)`, viewerIndex, adminIndex)
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

// Rejudge re-queues one submission. It shares requeueSubmission with batch
// rejudging so both paths reset state, fence the generation and repair the
// standings identically.
func (s *SubmissionStore) Rejudge(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, problemID, practice, err := requeueSubmission(ctx, tx, id)
	if err != nil {
		return err
	}
	if practice {
		if err := problem.RebuildPracticeCounters(ctx, tx, problemID); err != nil {
			return err
		}
	}
	if err := notifyJudgeJob(ctx, tx, id); err != nil {
		return err
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
