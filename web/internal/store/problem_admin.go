package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/vertex-oj/web/internal/model"
)

// ProblemAdminStore 负责出题侧的创建/更新(写操作)。
type ProblemAdminStore struct {
	db           *DB
	TestdataRoot string // 测试数据卷根目录(<root>/<problemID>/)
}

func NewProblemAdminStore(db *DB, testdataRoot string) *ProblemAdminStore {
	return &ProblemAdminStore{db: db, TestdataRoot: testdataRoot}
}

// CreateProblemInput 创建题目的输入。
type CreateProblemInput struct {
	Title         string   `json:"title"`
	StatementMD   string   `json:"statementMd"`
	Difficulty    int      `json:"difficulty"`
	Source        string   `json:"source"`
	TimeLimitMs   int      `json:"timeLimitMs"`
	MemoryLimitKb int      `json:"memoryLimitKb"`
	Visibility    string   `json:"visibility"` // draft | private | public
	Tags          []string `json:"tags"`
}

// Create 创建题目(默认 draft),并可选关联标签。
func (s *ProblemAdminStore) Create(ctx context.Context, authorID string, in *CreateProblemInput) (*model.Problem, error) {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 默认值兜底
	if in.TimeLimitMs <= 0 {
		in.TimeLimitMs = 1000
	}
	if in.MemoryLimitKb <= 0 {
		in.MemoryLimitKb = 262144
	}
	if in.Visibility == "" {
		in.Visibility = "draft"
	}

	var p model.Problem
	err = tx.QueryRow(ctx,
		`INSERT INTO problems (title, statement_md, difficulty, source,
		                      time_limit_ms, memory_limit_kb, visibility, author_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, title, statement_md, difficulty, source,
		           time_limit_ms, memory_limit_kb, visibility, author_id,
		           submission_count, accepted_count, solved_user_count, judge_type,
		           created_at, updated_at`,
		in.Title, in.StatementMD, in.Difficulty, in.Source,
		in.TimeLimitMs, in.MemoryLimitKb, in.Visibility, authorID,
	).Scan(&p.ID, &p.Title, &p.StatementMD, &p.Difficulty, &p.Source,
		&p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility, &p.AuthorID,
		&p.SubmissionCount, &p.AcceptedCount, &p.SolvedUserCount, &p.JudgeType,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := s.setTags(ctx, tx, p.ID, in.Tags); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	p.Tags = in.Tags
	return &p, nil
}

// UpdateInput 更新题目的输入(全部字段全量更新)。
type UpdateInput struct {
	Title         string   `json:"title"`
	StatementMD   string   `json:"statementMd"`
	Difficulty    int      `json:"difficulty"`
	Source        string   `json:"source"`
	TimeLimitMs   int      `json:"timeLimitMs"`
	MemoryLimitKb int      `json:"memoryLimitKb"`
	Visibility    string   `json:"visibility"`
	Tags          []string `json:"tags"`
}

// Update 全量更新题目(出题工作流:MVP 直接改线上题;problem_versions 留待 v1)。
func (s *ProblemAdminStore) Update(ctx context.Context, id string, in *UpdateInput) (*model.Problem, error) {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var p model.Problem
	err = tx.QueryRow(ctx,
		`UPDATE problems SET title = $2, statement_md = $3, difficulty = $4,
		                    source = $5, time_limit_ms = $6, memory_limit_kb = $7,
		                    visibility = $8, updated_at = now()
		 WHERE id = $1
		 RETURNING id, title, statement_md, difficulty, source,
		           time_limit_ms, memory_limit_kb, visibility, author_id,
		           submission_count, accepted_count, solved_user_count, judge_type,
		           created_at, updated_at`,
		id, in.Title, in.StatementMD, in.Difficulty, in.Source,
		in.TimeLimitMs, in.MemoryLimitKb, in.Visibility,
	).Scan(&p.ID, &p.Title, &p.StatementMD, &p.Difficulty, &p.Source,
		&p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility, &p.AuthorID,
		&p.SubmissionCount, &p.AcceptedCount, &p.SolvedUserCount, &p.JudgeType,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM problem_tags WHERE problem_id = $1`, id); err != nil {
		return nil, err
	}
	if err := s.setTags(ctx, tx, id, in.Tags); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	p.Tags = in.Tags
	return &p, nil
}

// Delete 删除题目及其测试数据文件。
func (s *ProblemAdminStore) Delete(ctx context.Context, id string) error {
	// 删除测试数据目录(先删文件,DB 行由级联删除)
	dir := filepath.Join(s.TestdataRoot, id)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM problems WHERE id = $1`, id)
	return err
}

// setTags 在事务内替换题目标签(upsert 标签,重建关联)。
func (s *ProblemAdminStore) setTags(ctx context.Context, tx pgx.Tx, problemID string, tags []string) error {
	for _, name := range tags {
		if name == "" {
			continue
		}
		var tagID int64
		err := tx.QueryRow(ctx,
			`INSERT INTO tags (name) VALUES ($1)
			 ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`, name).Scan(&tagID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO problem_tags (problem_id, tag_id) VALUES ($1, $2)`,
			problemID, tagID); err != nil {
			return err
		}
	}
	return nil
}

// SaveTestdata 保存测试数据 zip:解压到卷目录,写入 problem_testdata 元信息。
// zip 需含 1.in/1.out, 2.in/2.out ...(可含子目录,按文件名匹配)。
// 返回 case 数。
func (s *ProblemAdminStore) SaveTestdata(ctx context.Context, problemID string, zipData []byte, checker string) (int, string, error) {
	dir := filepath.Join(s.TestdataRoot, problemID)
	if err := os.RemoveAll(dir); err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, "", err
	}

	count, err := extractTestdataZip(zipData, dir)
	if err != nil {
		return 0, "", err
	}

	// 计算数据目录 sha256
	hash, err := dirSHA256(dir)
	if err != nil {
		return 0, "", err
	}

	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO problem_testdata (problem_id, data_version, storage_path, sha256, case_count, checker)
		 VALUES ($1, 1, $2, $3, $4, $5)
		 ON CONFLICT (problem_id) DO UPDATE SET
		   data_version = problem_testdata.data_version + 1,
		   storage_path = EXCLUDED.storage_path,
		   sha256 = EXCLUDED.sha256,
		   case_count = EXCLUDED.case_count,
		   checker = EXCLUDED.checker`,
		problemID, problemID, hex.EncodeToString(hash[:]), count, checker,
	)
	if err != nil {
		return 0, "", err
	}
	return count, hex.EncodeToString(hash[:]), nil
}

func dirSHA256(dir string) ([32]byte, error) {
	var h [32]byte
	entries, err := os.ReadDir(dir)
	if err != nil {
		return h, err
	}
	hasher := sha256.New()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return h, err
		}
		hasher.Write([]byte(e.Name()))
		hasher.Write(data)
	}
	copy(h[:], hasher.Sum(nil))
	return h, nil
}
