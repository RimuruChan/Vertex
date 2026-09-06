package problem

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jmoiron/sqlx"
)

// ProblemAdminStore 负责出题侧的创建/更新(写操作)。
type ProblemAdminStore struct {
	db           *database.DB
	TestdataRoot string // 测试数据卷根目录(<root>/<problemID>/<content-hash>/)
}

func NewProblemAdminStore(db *database.DB, testdataRoot string) *ProblemAdminStore {
	return &ProblemAdminStore{db: db, TestdataRoot: testdataRoot}
}

// Create 创建题目(默认 draft),并可选关联标签。
func (s *ProblemAdminStore) Create(ctx context.Context, authorID string, in *CreateInput) (*Problem, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

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

	var p Problem
	err = tx.QueryRowContext(ctx,
		`INSERT INTO problems (title, statement_md, difficulty, source,
		                      time_limit_ms, memory_limit_kb, visibility, author_id, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, public_id, title, statement_md, difficulty, source,
		           time_limit_ms, memory_limit_kb, visibility, author_id,
		           submission_count, accepted_count, solved_user_count, judge_type,
		           created_at, updated_at`,
		in.Title, in.StatementMD, in.Difficulty, in.Source,
		in.TimeLimitMs, in.MemoryLimitKb, in.Visibility, authorID, domain.ID(ctx),
	).Scan(&p.ID, &p.PublicID, &p.Title, &p.StatementMD, &p.Difficulty, &p.Source,
		&p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility, &p.AuthorID,
		&p.SubmissionCount, &p.AcceptedCount, &p.SolvedUserCount, &p.JudgeType,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := s.setTags(ctx, tx, p.ID, in.Tags); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	p.Tags = in.Tags
	return &p, nil
}

// Update atomically replaces the editable problem fields and tag set.
func (s *ProblemAdminStore) Update(ctx context.Context, id string, in *UpdateInput) (*Problem, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var p Problem
	err = tx.QueryRowContext(ctx,
		`UPDATE problems SET title = $2, statement_md = $3, difficulty = $4,
		                    source = $5, time_limit_ms = $6, memory_limit_kb = $7,
		                    visibility = $8, updated_at = now()
		 WHERE id = $1 AND domain_id = $9
		 RETURNING id, public_id, title, statement_md, difficulty, source,
		           time_limit_ms, memory_limit_kb, visibility, author_id,
		           submission_count, accepted_count, solved_user_count, judge_type,
		           created_at, updated_at`,
		id, in.Title, in.StatementMD, in.Difficulty, in.Source,
		in.TimeLimitMs, in.MemoryLimitKb, in.Visibility, domain.ID(ctx),
	).Scan(&p.ID, &p.PublicID, &p.Title, &p.StatementMD, &p.Difficulty, &p.Source,
		&p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility, &p.AuthorID,
		&p.SubmissionCount, &p.AcceptedCount, &p.SolvedUserCount, &p.JudgeType,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM problem_tags WHERE problem_id = $1`, id); err != nil {
		return nil, err
	}
	if err := s.setTags(ctx, tx, id, in.Tags); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	p.Tags = in.Tags
	return &p, nil
}

// Delete removes the problem row and its testdata directory as one store operation.
func (s *ProblemAdminStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `DELETE FROM problems WHERE id = $1 AND domain_id = $2`, id, domain.ID(ctx))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	dir := filepath.Join(s.TestdataRoot, id)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return tx.Commit()
}

// setTags 在事务内替换题目标签(upsert 标签,重建关联)。
func (s *ProblemAdminStore) setTags(ctx context.Context, tx *sqlx.Tx, problemID string, tags []string) error {
	for _, name := range tags {
		if name == "" {
			continue
		}
		var tagID int64
		err := tx.QueryRowContext(ctx,
			`INSERT INTO tags (name, domain_id) VALUES ($1, $2)
			 ON CONFLICT (domain_id, name) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`, name, domain.ID(ctx)).Scan(&tagID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO problem_tags (problem_id, tag_id, domain_id) VALUES ($1, $2, $3)`,
			problemID, tagID, domain.ID(ctx)); err != nil {
			return err
		}
	}
	return nil
}

// SaveTestdata materializes a content-addressed immutable directory before
// publishing its metadata. Existing jobs can therefore keep using the exact
// storagePath/hash snapshot they claimed while a new version is uploaded.
func (s *ProblemAdminStore) SaveTestdata(ctx context.Context, problemID string, zipData []byte, checker string) (int, string, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback() }()
	var present int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM problems WHERE id = $1 AND domain_id = $2 FOR UPDATE`, problemID, domain.ID(ctx)).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", err
	}
	count, hashText, storagePath, err := materializeTestdata(s.TestdataRoot, problemID, zipData)
	if err != nil {
		return 0, "", err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO problem_testdata (problem_id, data_version, storage_path, sha256, case_count, checker)
		 VALUES ($1, 1, $2, $3, $4, $5)
		 ON CONFLICT (problem_id) DO UPDATE SET
		   data_version = problem_testdata.data_version + 1,
		   storage_path = EXCLUDED.storage_path,
		   sha256 = EXCLUDED.sha256,
		   case_count = EXCLUDED.case_count,
		   checker = EXCLUDED.checker`,
		problemID, storagePath, hashText, count, checker,
	)
	if err != nil {
		return 0, "", err
	}
	return count, hashText, tx.Commit()
}

func materializeTestdata(root, problemID string, zipData []byte) (int, string, string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, "", "", err
	}
	staging, err := os.MkdirTemp(root, ".testdata-upload-")
	if err != nil {
		return 0, "", "", err
	}
	defer os.RemoveAll(staging)

	count, err := extractTestdataZip(zipData, staging)
	if err != nil {
		return 0, "", "", err
	}
	hash, err := dirSHA256(staging)
	if err != nil {
		return 0, "", "", err
	}
	hashText := hex.EncodeToString(hash[:])
	storagePath := path.Join(problemID, hashText)
	target := filepath.Join(root, filepath.FromSlash(storagePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, "", "", err
	}
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if renameErr := os.Rename(staging, target); renameErr != nil {
			// An identical concurrent upload may have won the rename race.
			if _, statErr := os.Stat(target); statErr != nil {
				return 0, "", "", renameErr
			}
		}
	} else if err != nil {
		return 0, "", "", err
	}
	return count, hashText, storagePath, nil
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
