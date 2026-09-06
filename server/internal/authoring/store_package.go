package authoring

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jmoiron/sqlx"
)

// PackageStore owns every editable part of a problem package. Each mutation
// bumps problems.package_revision inside the same transaction, which is what
// lets the UI say "testdata is stale" without a second source of truth.
type PackageStore struct{ db *database.DB }

func NewPackageStore(db *database.DB) *PackageStore { return &PackageStore{db: db} }

// bumpRevision marks the package dirty. Callers run it inside their own
// transaction so an edit and its revision move together.
func bumpRevision(ctx context.Context, tx *sqlx.Tx, problemID string) (int, error) {
	var revision int
	err := tx.QueryRowContext(ctx,
		`UPDATE problems SET package_revision = package_revision + 1, updated_at = now()
		 WHERE id = $1
		 RETURNING package_revision`, problemID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return revision, err
}

// queryer is satisfied by both *sqlx.DB and *sqlx.Tx so package reads can run
// either standalone or inside the build-claim transaction that pins them.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// execQueryer additionally writes; the statement renderer needs it because it
// reads the package and rewrites the published Markdown atomically.
type execQueryer interface {
	queryer
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *PackageStore) withTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- statements ----------

func (s *PackageStore) Statements(ctx context.Context, problemID string) ([]Statement, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT problem_id, language, name, legend, input_format, output_format,
		        notes, tutorial, scoring, updated_at
		 FROM problem_statements WHERE problem_id = $1 ORDER BY language`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Statement, 0, 2)
	for rows.Next() {
		var item Statement
		if err := rows.Scan(&item.ProblemID, &item.Language, &item.Name, &item.Legend,
			&item.InputFormat, &item.OutputFormat, &item.Notes, &item.Tutorial,
			&item.Scoring, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *PackageStore) SaveStatement(ctx context.Context, statement Statement) (*Statement, error) {
	var saved Statement
	err := s.withTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, statement.ProblemID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO problem_statements
			   (problem_id, language, name, legend, input_format, output_format, notes, tutorial, scoring)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (problem_id, language) DO UPDATE SET
			   name = EXCLUDED.name, legend = EXCLUDED.legend,
			   input_format = EXCLUDED.input_format, output_format = EXCLUDED.output_format,
			   notes = EXCLUDED.notes, tutorial = EXCLUDED.tutorial,
			   scoring = EXCLUDED.scoring, updated_at = now()
			 RETURNING problem_id, language, name, legend, input_format, output_format,
			           notes, tutorial, scoring, updated_at`,
			statement.ProblemID, statement.Language, statement.Name, statement.Legend,
			statement.InputFormat, statement.OutputFormat, statement.Notes,
			statement.Tutorial, statement.Scoring,
		).Scan(&saved.ProblemID, &saved.Language, &saved.Name, &saved.Legend,
			&saved.InputFormat, &saved.OutputFormat, &saved.Notes, &saved.Tutorial,
			&saved.Scoring, &saved.UpdatedAt); err != nil {
			return err
		}
		// Statement edits refresh the public Markdown immediately, reusing the
		// samples the last successful build produced. Test data itself still
		// only changes through a build.
		samples, err := lastBuiltSamples(ctx, tx, statement.ProblemID)
		if err != nil {
			return err
		}
		return renderStatementTx(ctx, tx, statement.ProblemID, samples)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *PackageStore) DeleteStatement(ctx context.Context, problemID, language string) error {
	return s.withTx(ctx, func(tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx,
			`DELETE FROM problem_statements WHERE problem_id = $1 AND language = $2`,
			problemID, language)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		_, err = bumpRevision(ctx, tx, problemID)
		return err
	})
}

// ---------- files ----------

const fileColumns = `id, problem_id, kind, name, language, source_code,
	expected_verdict, is_active, created_at, updated_at`

func scanFile(scanner interface{ Scan(...any) error }) (File, error) {
	var item File
	err := scanner.Scan(&item.ID, &item.ProblemID, &item.Kind, &item.Name, &item.Language,
		&item.SourceCode, &item.ExpectedVerdict, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// Files lists package sources. Passing includeSource=false keeps list
// responses small; the workspace fetches one file at a time for editing.
func (s *PackageStore) Files(ctx context.Context, problemID string, includeSource bool) ([]File, error) {
	return filesFrom(ctx, s.db.Pool, problemID, includeSource)
}

func filesFrom(ctx context.Context, q queryer, problemID string, includeSource bool) ([]File, error) {
	source := "source_code"
	if !includeSource {
		source = "''"
	}
	rows, err := q.QueryContext(ctx,
		`SELECT id, problem_id, kind, name, language, `+source+`,
		        expected_verdict, is_active, created_at, updated_at
		 FROM problem_files WHERE problem_id = $1
		 ORDER BY kind, is_active DESC, name`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]File, 0, 8)
	for rows.Next() {
		item, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *PackageStore) File(ctx context.Context, problemID string, id int64) (*File, error) {
	item, err := scanFile(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+fileColumns+` FROM problem_files WHERE problem_id = $1 AND id = $2`,
		problemID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// SaveFile upserts by (problem_id, kind, name). Activating a file clears the
// previous active one of the same kind first, so the partial unique index can
// never see two winners.
func (s *PackageStore) SaveFile(ctx context.Context, file File) (*File, error) {
	var saved File
	err := s.withTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, file.ProblemID); err != nil {
			return err
		}
		if file.IsActive {
			if _, err := tx.ExecContext(ctx,
				`UPDATE problem_files SET is_active = FALSE, updated_at = now()
				 WHERE problem_id = $1 AND kind = $2 AND is_active AND name <> $3`,
				file.ProblemID, file.Kind, file.Name); err != nil {
				return err
			}
		}
		var err error
		saved, err = scanFile(tx.QueryRowContext(ctx,
			`INSERT INTO problem_files
			   (problem_id, kind, name, language, source_code, expected_verdict, is_active)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (problem_id, kind, name) DO UPDATE SET
			   language = EXCLUDED.language, source_code = EXCLUDED.source_code,
			   expected_verdict = EXCLUDED.expected_verdict,
			   is_active = EXCLUDED.is_active, updated_at = now()
			 RETURNING `+fileColumns,
			file.ProblemID, file.Kind, file.Name, file.Language, file.SourceCode,
			file.ExpectedVerdict, file.IsActive))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *PackageStore) DeleteFile(ctx context.Context, problemID string, id int64) error {
	return s.withTx(ctx, func(tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx,
			`DELETE FROM problem_files WHERE problem_id = $1 AND id = $2`, problemID, id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		_, err = bumpRevision(ctx, tx, problemID)
		return err
	})
}

// ---------- tests ----------

const testColumns = `id, problem_id, test_index, group_name, source, input_data,
	generate_cmd, is_sample, points, description`

func scanTest(scanner interface{ Scan(...any) error }) (Test, error) {
	var item Test
	err := scanner.Scan(&item.ID, &item.ProblemID, &item.Index, &item.Group, &item.Source,
		&item.InputData, &item.GenerateCmd, &item.IsSample, &item.Points, &item.Description)
	return item, err
}

func (s *PackageStore) Tests(ctx context.Context, problemID string, includeInput bool) ([]Test, error) {
	return testsFrom(ctx, s.db.Pool, problemID, includeInput)
}

func testsFrom(ctx context.Context, q queryer, problemID string, includeInput bool) ([]Test, error) {
	input := "input_data"
	if !includeInput {
		input = "left(input_data, 512)"
	}
	rows, err := q.QueryContext(ctx,
		`SELECT id, problem_id, test_index, group_name, source, `+input+`,
		        generate_cmd, is_sample, points, description
		 FROM problem_tests WHERE problem_id = $1 ORDER BY test_index`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Test, 0, 16)
	for rows.Next() {
		item, err := scanTest(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// CreateTest appends a test at the end of the current plan.
func (s *PackageStore) CreateTest(ctx context.Context, test Test) (*Test, error) {
	var saved Test
	err := s.withTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, test.ProblemID); err != nil {
			return err
		}
		var err error
		saved, err = scanTest(tx.QueryRowContext(ctx,
			`INSERT INTO problem_tests
			   (problem_id, test_index, group_name, source, input_data, generate_cmd,
			    is_sample, points, description)
			 VALUES ($1,
			   COALESCE((SELECT max(test_index) FROM problem_tests WHERE problem_id = $1), 0) + 1,
			   $2, $3, $4, $5, $6, $7, $8)
			 RETURNING `+testColumns,
			test.ProblemID, test.Group, test.Source, test.InputData, test.GenerateCmd,
			test.IsSample, test.Points, test.Description))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *PackageStore) UpdateTest(ctx context.Context, test Test) (*Test, error) {
	var saved Test
	err := s.withTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, test.ProblemID); err != nil {
			return err
		}
		var err error
		saved, err = scanTest(tx.QueryRowContext(ctx,
			`UPDATE problem_tests
			 SET group_name = $3, source = $4, input_data = $5, generate_cmd = $6,
			     is_sample = $7, points = $8, description = $9
			 WHERE problem_id = $1 AND id = $2
			 RETURNING `+testColumns,
			test.ProblemID, test.ID, test.Group, test.Source, test.InputData,
			test.GenerateCmd, test.IsSample, test.Points, test.Description))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// DeleteTest removes a test and closes the gap so indexes stay 1..N, which is
// the contract the judge testdata layout depends on.
func (s *PackageStore) DeleteTest(ctx context.Context, problemID string, id int64) error {
	return s.withTx(ctx, func(tx *sqlx.Tx) error {
		var removedIndex int
		err := tx.QueryRowContext(ctx,
			`DELETE FROM problem_tests WHERE problem_id = $1 AND id = $2 RETURNING test_index`,
			problemID, id).Scan(&removedIndex)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE problem_tests SET test_index = test_index - 1
			 WHERE problem_id = $1 AND test_index > $2`, problemID, removedIndex); err != nil {
			return err
		}
		_, err = bumpRevision(ctx, tx, problemID)
		return err
	})
}

// ReorderTest moves one test to a new position. The two-step update through a
// temporary negative index avoids tripping the (problem_id, test_index) unique
// constraint mid-shift.
func (s *PackageStore) ReorderTest(ctx context.Context, problemID string, id int64, target int) error {
	return s.withTx(ctx, func(tx *sqlx.Tx) error {
		var current, total int
		err := tx.QueryRowContext(ctx,
			`SELECT test_index,
			        (SELECT count(*)::int FROM problem_tests WHERE problem_id = $1)
			 FROM problem_tests WHERE problem_id = $1 AND id = $2`, problemID, id).Scan(&current, &total)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if target < 1 {
			target = 1
		}
		if target > total {
			target = total
		}
		if target == current {
			return nil
		}
		// Park the moved row above every existing index while the others
		// shift. A negative placeholder would be simpler but violates the
		// test_index > 0 constraint.
		if _, err := tx.ExecContext(ctx,
			`UPDATE problem_tests
			 SET test_index = (SELECT max(test_index) + 1 FROM problem_tests WHERE problem_id = $1)
			 WHERE problem_id = $1 AND id = $2`,
			problemID, id); err != nil {
			return err
		}
		if target < current {
			_, err = tx.ExecContext(ctx,
				`UPDATE problem_tests SET test_index = test_index + 1
				 WHERE problem_id = $1 AND test_index >= $2 AND test_index < $3`,
				problemID, target, current)
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE problem_tests SET test_index = test_index - 1
				 WHERE problem_id = $1 AND test_index > $2 AND test_index <= $3`,
				problemID, current, target)
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE problem_tests SET test_index = $3 WHERE problem_id = $1 AND id = $2`,
			problemID, id, target); err != nil {
			return err
		}
		_, err = bumpRevision(ctx, tx, problemID)
		return err
	})
}

// ---------- package snapshot ----------

// Snapshot assembles the immutable build input. It is read outside a
// transaction only after the build row pinned the revision, and the build
// completion re-checks that revision before publishing.
func (s *PackageStore) Snapshot(ctx context.Context, problemID string) (*Package, error) {
	return snapshotFrom(ctx, s.db.Pool, problemID)
}

func snapshotFrom(ctx context.Context, q queryer, problemID string) (*Package, error) {
	var pkg Package
	err := q.QueryRowContext(ctx,
		`SELECT id, title, time_limit_ms, memory_limit_kb, judge_type, package_revision
		 FROM problems WHERE id = $1`, problemID).Scan(
		&pkg.ProblemID, &pkg.Title, &pkg.TimeLimitMs, &pkg.MemoryLimitKB,
		&pkg.JudgeType, &pkg.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	files, err := filesFrom(ctx, q, problemID, true)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		switch file.Kind {
		case KindChecker:
			if file.IsActive {
				pkg.Checker = cloneFile(file)
			}
		case KindValidator:
			if file.IsActive {
				pkg.Validator = cloneFile(file)
			}
		case KindInteractor:
			if file.IsActive {
				pkg.Interactor = cloneFile(file)
			}
		case KindGenerator:
			pkg.Generators = append(pkg.Generators, file)
		case KindSolution:
			pkg.Solutions = append(pkg.Solutions, file)
		}
	}

	tests, err := testsFrom(ctx, q, problemID, true)
	if err != nil {
		return nil, err
	}
	pkg.Tests = tests
	return &pkg, nil
}

func cloneFile(file File) *File { return &file }

// Meta reports the problem-level facts the authoring workspace needs without
// loading any source code or test data.
func (s *PackageStore) Meta(ctx context.Context, problemID string) (*PackageMeta, error) {
	var meta PackageMeta
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT problem.id, problem.title, problem.visibility, problem.judge_type,
		        problem.statement_language, problem.time_limit_ms, problem.memory_limit_kb,
		        problem.package_revision, problem.built_revision, problem.last_built_at,
		        COALESCE(testdata.case_count, 0), COALESCE(testdata.checker, ''),
		        COALESCE(testdata.data_version, 0), COALESCE(testdata.sha256, '')
		 FROM problems AS problem
		 LEFT JOIN problem_testdata AS testdata ON testdata.problem_id = problem.id
		 WHERE problem.id = $1`, problemID).Scan(
		&meta.ProblemID, &meta.Title, &meta.Visibility, &meta.JudgeType,
		&meta.StatementLanguage, &meta.TimeLimitMs, &meta.MemoryLimitKB,
		&meta.PackageRevision, &meta.BuiltRevision, &meta.LastBuiltAt,
		&meta.TestdataCases, &meta.TestdataChecker, &meta.TestdataVersion, &meta.TestdataSHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// TouchRevision marks the package dirty for edits that live on the problems
// row itself (limits, judge type) rather than in a package table.
func (s *PackageStore) TouchRevision(ctx context.Context, problemID string) error {
	return s.withTx(ctx, func(tx *sqlx.Tx) error {
		_, err := bumpRevision(ctx, tx, problemID)
		return err
	})
}
