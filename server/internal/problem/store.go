package problem

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// ProblemStore 负责 problems 表读写。
type ProblemStore struct{ db *database.DB }

func NewProblemStore(db *database.DB) *ProblemStore { return &ProblemStore{db: db} }

// List 分页查询题目(public 可见性 + 标签 + 难度 + 关键字),带标签聚合。
func (s *ProblemStore) List(ctx context.Context, f Filters) ([]Problem, int, error) {
	clauses := []string{"1=1"}
	args := []any{}
	add := func(clause string, val any) {
		args = append(args, val)
		clauses = append(clauses, strings.Replace(clause, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if f.Visibility != "" {
		add("p.visibility = ?", f.Visibility)
	}
	if f.Difficulty > 0 {
		add("p.difficulty = ?", f.Difficulty)
	}
	if f.Keyword != "" {
		args = append(args, "%"+f.Keyword+"%")
		clauses = append(clauses, "(p.title ILIKE $"+strconv.Itoa(len(args))+" OR p.source ILIKE $"+strconv.Itoa(len(args))+")")
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}

	// 标签过滤:EXISTS 子查询(避免 join 重复)
	if f.Tag != "" {
		args = append(args, f.Tag)
		clauses = append(clauses, `EXISTS (
			SELECT 1 FROM problem_tags pt JOIN tags t ON t.id = pt.tag_id
			WHERE pt.problem_id = p.id AND t.name = $`+strconv.Itoa(len(args))+`)`)
	}

	// 个人进度过滤:只对已登录查看者生效,走 idx_submissions_user_problem_status。
	if f.ViewerID != "" && f.Status != "" {
		args = append(args, f.ViewerID)
		viewer := "$" + strconv.Itoa(len(args))
		solved := `EXISTS (SELECT 1 FROM submissions s
			WHERE s.user_id = ` + viewer + `::uuid AND s.problem_id = p.id
			  AND s.contest_id IS NULL AND s.status = 'Accepted')`
		attempted := `EXISTS (SELECT 1 FROM submissions s
			WHERE s.user_id = ` + viewer + `::uuid AND s.problem_id = p.id
			  AND s.contest_id IS NULL)`
		switch f.Status {
		case UserStatusSolved:
			clauses = append(clauses, solved)
		case UserStatusAttempted:
			clauses = append(clauses, attempted+" AND NOT "+solved)
		case UserStatusNone:
			clauses = append(clauses, "NOT "+attempted)
		}
	}

	where := "WHERE " + joinClauses(clauses)

	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		"SELECT count(*) FROM problems p "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	limitIdx, offsetIdx := len(args)-1, len(args)

	query := `SELECT p.id, p.title, p.difficulty, p.source,
	                 p.time_limit_ms, p.memory_limit_kb, p.visibility,
	                 p.author_id, p.submission_count, p.accepted_count,
	                 p.solved_user_count, p.judge_type, p.created_at, p.updated_at
	          FROM problems p
	          ` + where + fmt.Sprintf(" ORDER BY p.created_at DESC, p.id DESC LIMIT $%d OFFSET $%d", limitIdx, offsetIdx)

	rows, err := s.db.Pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Problem{}
	for rows.Next() {
		p := Problem{}
		if err := rows.Scan(&p.ID, &p.Title, &p.Difficulty, &p.Source,
			&p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility,
			&p.AuthorID, &p.SubmissionCount, &p.AcceptedCount,
			&p.SolvedUserCount, &p.JudgeType, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// 批量取标签
	if len(list) > 0 {
		if err := s.fillTags(ctx, list); err != nil {
			return nil, 0, err
		}
	}
	return list, total, nil
}

// Get 取单道题目(含标签),不校验可见性(调用方判断)。
func (s *ProblemStore) Get(ctx context.Context, id string) (*Problem, error) {
	var p Problem
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT p.id, p.title, p.statement_md, p.difficulty, p.source,
		        p.time_limit_ms, p.memory_limit_kb, p.visibility,
		        p.author_id, p.submission_count, p.accepted_count,
		        p.solved_user_count, p.judge_type, p.created_at, p.updated_at
		 FROM problems p WHERE p.id = $1`, id,
	).Scan(&p.ID, &p.Title, &p.StatementMD, &p.Difficulty, &p.Source,
		&p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility,
		&p.AuthorID, &p.SubmissionCount, &p.AcceptedCount,
		&p.SolvedUserCount, &p.JudgeType, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// fillTags writes into the slice it is given, so the result has to be read
	// back out of that slice rather than from the local copy that seeded it.
	single := []Problem{p}
	if err := s.fillTags(ctx, single); err != nil {
		return nil, err
	}
	return &single[0], nil
}

// Testdata 取题目测试数据元信息(判题 worker 用)。
func (s *ProblemStore) Testdata(ctx context.Context, problemID string) (*TestdataInfo, error) {
	var td TestdataInfo
	var cfg []byte
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT problem_id, data_version, storage_path, sha256, case_count, checker, spj_source, config_json
		 FROM problem_testdata WHERE problem_id = $1`, problemID,
	).Scan(&td.ProblemID, &td.DataVersion, &td.StoragePath, &td.SHA256, &td.CaseCount,
		&td.Checker, &td.SPJSource, &cfg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cfg, &td.Config)
	return &td, nil
}

// UserStatuses 一次查出查看者在给定题目上的进度,避免每行一次子查询。
// viewerID 为空或没有提交记录的题目不会出现在返回 map 中(调用方按 none 处理)。
func (s *ProblemStore) UserStatuses(ctx context.Context, viewerID string, problemIDs []string) (map[string]string, error) {
	if viewerID == "" || len(problemIDs) == 0 {
		return map[string]string{}, nil
	}
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT problem_id, bool_or(status = 'Accepted') AS solved
		 FROM submissions
		 WHERE user_id = $1::uuid AND problem_id = ANY($2) AND contest_id IS NULL
		 GROUP BY problem_id`, viewerID, problemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statuses := make(map[string]string, len(problemIDs))
	for rows.Next() {
		var problemID string
		var solved bool
		if err := rows.Scan(&problemID, &solved); err != nil {
			return nil, err
		}
		if solved {
			statuses[problemID] = UserStatusSolved
		} else {
			statuses[problemID] = UserStatusAttempted
		}
	}
	return statuses, rows.Err()
}

// Tags 列出 public 题目上出现过的标签,按题目数量倒序。
func (s *ProblemStore) Tags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT t.name, count(*)::int AS problem_count
		 FROM tags t
		 JOIN problem_tags pt ON pt.tag_id = t.id
		 JOIN problems p ON p.id = pt.problem_id AND p.visibility = 'public'
		 GROUP BY t.name
		 ORDER BY problem_count DESC, t.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []Tag{}
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.ProblemCount); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *ProblemStore) fillTags(ctx context.Context, problems []Problem) error {
	ids := make([]string, 0, len(problems))
	idToIdx := map[string]int{}
	for i, p := range problems {
		ids = append(ids, p.ID)
		idToIdx[p.ID] = i
	}

	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT pt.problem_id, t.name
		 FROM problem_tags pt JOIN tags t ON t.id = pt.tag_id
		 WHERE pt.problem_id = ANY($1) ORDER BY t.name`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid, name string
		if err := rows.Scan(&pid, &name); err != nil {
			return err
		}
		if idx, ok := idToIdx[pid]; ok {
			problems[idx].Tags = append(problems[idx].Tags, name)
		}
	}
	return rows.Err()
}

func joinClauses(clauses []string) string {
	return strings.Join(clauses, " AND ")
}
