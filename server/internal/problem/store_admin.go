package problem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
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

	scope, err := domain.LockScope(ctx, tx, authorID)
	if err != nil {
		return nil, accessError(err)
	}
	if !scope.Allows(domain.CreateProblem) {
		return nil, domain.ErrForbidden
	}

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
		                      time_limit_ms, memory_limit_kb, visibility, author_id, owner_id, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9)
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

	tags, err := json.Marshal(in.Tags)
	if err != nil {
		return nil, err
	}
	if in.Tags == nil {
		tags = []byte("[]")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE problem_workspaces SET tags_json=$2 WHERE problem_id=$1", p.ID, tags); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	p.Tags = in.Tags
	p.OwnerID, p.DomainID = authorID, scope.Domain.ID
	p.Permissions = EffectivePermissions(scope, authorID, p.Visibility, AccessOwner)
	return &p, nil
}

// Update changes the working copy. Access visibility is a separate owner action;
// the published title, statement, limits and tags remain untouched.
func (s *ProblemAdminStore) Update(ctx context.Context, id string, in *UpdateInput) (*Problem, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if !access.Permissions.Edit || (in.Visibility != access.Visibility && !access.Permissions.Publish) {
		return nil, domain.ErrForbidden
	}
	tags, err := json.Marshal(in.Tags)
	if err != nil {
		return nil, err
	}
	if in.Tags == nil {
		tags = []byte("[]")
	}
	var changed, dataChanged bool
	err = tx.QueryRowxContext(ctx, `SELECT (title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,tags_json) IS DISTINCT FROM ($2,$3,$4,$5,$6,$7,$8::jsonb),
 (time_limit_ms,memory_limit_kb) IS DISTINCT FROM ($6,$7) FROM problem_workspaces WHERE problem_id=$1`,
		id, in.Title, in.StatementMD, in.Difficulty, in.Source, in.TimeLimitMs, in.MemoryLimitKb, tags).Scan(&changed, &dataChanged)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE problem_workspaces SET title=$2,statement_md=$3,difficulty=$4,source=$5,time_limit_ms=$6,memory_limit_kb=$7,tags_json=$8,updated_at=now() WHERE problem_id=$1`,
		id, in.Title, in.StatementMD, in.Difficulty, in.Source, in.TimeLimitMs, in.MemoryLimitKb, tags); err != nil {
		return nil, err
	}
	if changed {
		if _, err := tx.ExecContext(ctx, `UPDATE problem_statements SET name=$2,updated_at=now() WHERE problem_id=$1 AND language=(SELECT statement_language FROM problem_workspaces WHERE problem_id=$1) AND name IS DISTINCT FROM $2`, id, in.Title); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE problems SET visibility=$2,
 package_revision=package_revision+CASE WHEN $3 THEN 1 ELSE 0 END,
 data_revision=data_revision+CASE WHEN $4 THEN 1 ELSE 0 END,
 updated_at=CASE WHEN visibility<>$2 THEN now() ELSE updated_at END WHERE id=$1`, id, in.Visibility, changed, dataChanged); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	p, err := NewProblemStore(s.db).GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Permissions = EffectivePermissions(access.Scope, access.OwnerID, in.Visibility, access.Role)
	return p, nil
}

// Delete removes the problem row and its testdata directory as one store operation.
func (s *ProblemAdminStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	access, err := LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Delete {
		return domain.ErrForbidden
	}
	var referenced bool
	if err := tx.GetContext(ctx, &referenced, `SELECT EXISTS(SELECT 1 FROM submissions WHERE problem_id=$1) OR EXISTS(SELECT 1 FROM contest_problems WHERE problem_id=$1)`, id); err != nil {
		return err
	}
	if referenced {
		return ErrReferenced
	}
	if err := recordAccessAudit(ctx, tx, access, "problem.delete", ""); err != nil {
		return err
	}
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

// SaveTestdata imports candidate data, never a publication.
func (s *ProblemAdminStore) SaveTestdata(ctx context.Context, problemID string, zipData []byte, checker string) (int, string, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback() }()
	access, err := LockAccess(ctx, tx, problemID, domain.ActorID(ctx))
	if err != nil {
		return 0, "", err
	}
	if !access.Permissions.Edit {
		return 0, "", domain.ErrForbidden
	}
	var dataRevision int
	if err := tx.QueryRowxContext(ctx, "UPDATE problems SET package_revision=package_revision+1,data_revision=data_revision+1 WHERE id=$1 RETURNING data_revision", problemID).Scan(&dataRevision); err != nil {
		return 0, "", err
	}
	count, hashText, storagePath, err := materializeTestdata(s.TestdataRoot, problemID, zipData)
	if err != nil {
		return 0, "", err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO problem_testdata (problem_id, data_version, storage_path, sha256, case_count, checker, data_revision)
		 VALUES ($1, 1, $2, $3, $4, $5, $6)
		 ON CONFLICT (problem_id) DO UPDATE SET
		   data_version = problem_testdata.data_version + 1,
		   storage_path = EXCLUDED.storage_path,
		   sha256 = EXCLUDED.sha256,
		   case_count = EXCLUDED.case_count,
		   checker = EXCLUDED.checker, data_revision=EXCLUDED.data_revision,
		   build_id=NULL,samples_json='[]',config_json='{}',spj_source=''`,
		problemID, storagePath, hashText, count, checker, dataRevision,
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
