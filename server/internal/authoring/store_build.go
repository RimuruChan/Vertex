package authoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

const (
	maxBuildAttempts = 3
	// Logs and reports are author-facing diagnostics, not judge inputs, so
	// they are truncated rather than rejected when a build gets chatty.
	maxBuildLogBytes    = 256 << 10
	maxSampleTextBytes  = 8 << 10
	maxOutcomeTextBytes = 4 << 10
)

// BuildStore owns the build queue. It reuses the judge queue's fencing model:
// a claim hands out a lease token, and every later write must present the same
// (build, worker, lease token) triple while the lease is still alive.
type BuildStore struct{ db *database.DB }

func NewBuildStore(db *database.DB) *BuildStore { return &BuildStore{db: db} }

const buildColumns = `id, problem_id, revision, state, stage, attempt,
	COALESCE(worker_id, ''), COALESCE(lease_token::text, ''),
	COALESCE(lease_expires_at, to_timestamp(0)),
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at`

func scanBuild(scanner interface{ Scan(...any) error }) (Build, error) {
	var item Build
	var tests, solutions []byte
	err := scanner.Scan(&item.ID, &item.ProblemID, &item.Revision, &item.State, &item.Stage,
		&item.Attempt, &item.WorkerID, &item.LeaseToken, &item.LeaseExpires,
		&item.ProgressDone, &item.ProgressTotal, &item.Log, &item.ErrorMessage,
		&tests, &solutions, &item.PackagePath, &item.PackageSHA256, &item.PackageCases,
		&item.CreatedBy, &item.CreatedAt, &item.StartedAt, &item.FinishedAt)
	if err != nil {
		return item, err
	}
	if err := json.Unmarshal(tests, &item.Tests); err != nil {
		item.Tests = nil
	}
	if err := json.Unmarshal(solutions, &item.Solutions); err != nil {
		item.Solutions = nil
	}
	return item, nil
}

// Enqueue queues a build for the problem's current package revision. The
// partial unique index makes "one active build per problem" a database
// invariant, so a double click returns the running build instead of a second.
func (s *BuildStore) Enqueue(ctx context.Context, problemID, createdBy string) (*Build, error) {
	var creator *string
	if createdBy != "" {
		creator = &createdBy
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var revision int
	err = tx.QueryRowContext(ctx,
		`SELECT package_revision FROM problems WHERE id = $1 FOR UPDATE`, problemID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	existing, err := scanBuild(tx.QueryRowContext(ctx,
		`SELECT `+buildColumns+` FROM problem_build_jobs
		 WHERE problem_id = $1 AND state IN ('queued', 'running')`, problemID))
	if err == nil {
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, commitErr
		}
		return &existing, ErrBuildRunning
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	created, err := scanBuild(tx.QueryRowContext(ctx,
		`INSERT INTO problem_build_jobs (problem_id, revision, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING `+buildColumns, problemID, revision, creator))
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_notify('vertex_problem_builds', $1)`, created.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *BuildStore) Get(ctx context.Context, problemID, buildID string) (*Build, error) {
	build, err := scanBuild(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+buildColumns+` FROM problem_build_jobs WHERE id = $1 AND problem_id = $2`,
		buildID, problemID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &build, nil
}

// Latest returns the most recent build of a problem, or nil when the package
// has never been built.
func (s *BuildStore) Latest(ctx context.Context, problemID string) (*Build, error) {
	build, err := scanBuild(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+buildColumns+` FROM problem_build_jobs
		 WHERE problem_id = $1 ORDER BY created_at DESC LIMIT 1`, problemID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &build, nil
}

// LatestSuccessful returns the most recent published build, or nil when the
// package has never built successfully.
func (s *BuildStore) LatestSuccessful(ctx context.Context, problemID string) (*Build, error) {
	build, err := scanBuild(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+buildColumns+` FROM problem_build_jobs
		 WHERE problem_id = $1 AND state = 'succeeded'
		 ORDER BY finished_at DESC NULLS LAST LIMIT 1`, problemID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &build, nil
}

func (s *BuildStore) List(ctx context.Context, problemID string, limit int) ([]Build, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+buildColumns+` FROM problem_build_jobs
		 WHERE problem_id = $1 ORDER BY created_at DESC LIMIT $2`, problemID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Build, 0, limit)
	for rows.Next() {
		item, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Cancel stops a queued or running build. A running worker discovers the
// cancellation when its next fenced write is rejected.
func (s *BuildStore) Cancel(ctx context.Context, problemID, buildID string) error {
	result, err := s.db.Pool.ExecContext(ctx,
		`UPDATE problem_build_jobs
		 SET state = 'cancelled', stage = 'done', finished_at = now(),
		     lease_expires_at = NULL, error_message = 'cancelled by author'
		 WHERE id = $1 AND problem_id = $2 AND state IN ('queued', 'running')`,
		buildID, problemID)
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
	return nil
}

// Claim leases the next queued build and returns it together with the package
// snapshot read in the same transaction, so the worker builds exactly the
// revision recorded on the job row.
func (s *BuildStore) Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*Build, *Package, error) {
	if err := s.failExhausted(ctx); err != nil {
		return nil, nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	build, err := scanBuild(tx.QueryRowContext(ctx,
		`WITH candidate AS (
		   SELECT id FROM problem_build_jobs
		   WHERE (state = 'queued' AND available_at <= now())
		      OR (state = 'running' AND lease_expires_at < now() AND attempt < $3)
		   ORDER BY priority DESC, available_at, created_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 ), claimed AS (
		   UPDATE problem_build_jobs AS job
		   SET state = 'running', stage = 'compile', attempt = job.attempt + 1,
		       worker_id = $1, lease_token = gen_random_uuid(),
		       lease_expires_at = now() + ($2::bigint * interval '1 millisecond'),
		       started_at = COALESCE(job.started_at, now()), error_message = '',
		       revision = (SELECT package_revision FROM problems WHERE id = job.problem_id)
		   FROM candidate
		   WHERE job.id = candidate.id
		   RETURNING job.*
		 )
		 SELECT `+buildColumns+` FROM claimed`,
		workerID, leaseTTL.Milliseconds(), maxBuildAttempts))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	pkg, err := snapshotFrom(ctx, tx, build.ProblemID)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return &build, pkg, nil
}

// Progress renews the lease and publishes advisory stage/progress data. Like
// the judge heartbeat, a malformed progress number costs display accuracy, not
// the lease.
func (s *BuildStore) Progress(ctx context.Context, progress Progress, leaseTTL time.Duration) error {
	var renewed int
	err := s.db.Pool.QueryRowContext(ctx,
		`WITH renewed AS (
		   UPDATE problem_build_jobs
		   SET lease_expires_at = now() + ($4::bigint * interval '1 millisecond'),
		       stage = COALESCE(NULLIF($5::text, ''), stage),
		       progress_done = GREATEST(progress_done, $6),
		       progress_total = GREATEST(progress_total, $7),
		       log = left(log || $8, $9)
		   WHERE id = $1 AND worker_id = $2 AND lease_token = $3::uuid
		     AND state = 'running' AND lease_expires_at >= now()
		   RETURNING 1
		 )
		 SELECT count(*)::int FROM renewed`,
		progress.BuildID, progress.WorkerID, progress.LeaseToken, leaseTTL.Milliseconds(),
		progress.Stage, progress.Done, progress.Total, progress.Log, maxBuildLogBytes,
	).Scan(&renewed)
	if err != nil {
		return err
	}
	if renewed != 1 {
		return ErrStaleLease
	}
	return nil
}

// ResolvePackageTarget returns the server-owned problem ID only while the
// caller still owns a live build lease. Filesystem paths must be derived from
// this value, never from the worker's query string.
func (s *BuildStore) ResolvePackageTarget(ctx context.Context, buildID, workerID, leaseToken string) (string, error) {
	var problemID string
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT problem_id FROM problem_build_jobs
		 WHERE id = $1 AND worker_id = $2 AND lease_token = $3::uuid
		   AND state = 'running' AND lease_expires_at >= now()`,
		buildID, workerID, leaseToken).Scan(&problemID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrStaleLease
	}
	if err != nil {
		return "", err
	}
	return problemID, nil
}

// RecordPackage stores the artifact a worker just materialized. It repeats
// both lease and problem checks to close the race between target resolution
// and filesystem work. Only a successful Complete flips problem_testdata.
func (s *BuildStore) RecordPackage(
	ctx context.Context, buildID, problemID, workerID, leaseToken string, upload PackageUpload,
) error {
	if path.Dir(upload.StoragePath) != problemID || path.Base(upload.StoragePath) != upload.SHA256 {
		return ErrPackageTarget
	}
	result, err := s.db.Pool.ExecContext(ctx,
		`UPDATE problem_build_jobs
		 SET package_path = $5, package_sha256 = $6, package_cases = $7, stage = 'package'
		 WHERE id = $1 AND problem_id = $2 AND worker_id = $3 AND lease_token = $4::uuid
		   AND state = 'running' AND lease_expires_at >= now()`,
		buildID, problemID, workerID, leaseToken,
		upload.StoragePath, upload.SHA256, upload.CaseCount)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrStaleLease
	}
	return nil
}

// Complete finishes a fenced build. On success it publishes the recorded
// artifact into problem_testdata and re-renders the public statement in the
// same transaction, so readers never observe testdata and statement disagreeing.
func (s *BuildStore) Complete(ctx context.Context, result BuildResult, checker string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state, problemID, packagePath, packageSHA string
	var packageCases, revision int
	err = tx.QueryRowContext(ctx,
		`SELECT state, problem_id, package_path, package_sha256, package_cases, revision
		 FROM problem_build_jobs WHERE id = $1 FOR UPDATE`, result.BuildID).Scan(
		&state, &problemID, &packagePath, &packageSHA, &packageCases, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleLease
	}
	if err != nil {
		return err
	}
	if state != BuildRunning {
		return ErrStaleLease
	}

	publish := result.Success
	if publish && (packagePath == "" || packageCases <= 0) {
		publish = false
		result.ErrorMessage = "构建声明成功但没有上传测试数据"
	}
	finalState := BuildFailed
	if publish {
		finalState = BuildSucceeded
	}
	tests, err := json.Marshal(boundedTestOutcomes(result.Tests))
	if err != nil {
		return err
	}
	solutions, err := json.Marshal(boundedSolutionOutcomes(result.Solutions))
	if err != nil {
		return err
	}

	command, err := tx.ExecContext(ctx,
		`UPDATE problem_build_jobs
		 SET state = $5, stage = 'done', finished_at = now(), lease_expires_at = NULL,
		     log = left(log || $6, $9), error_message = left($7, 4096),
		     tests_json = $8, solutions_json = $10,
		     progress_done = GREATEST(progress_done, progress_total)
		 WHERE id = $1 AND worker_id = $2 AND lease_token = $3::uuid
		   AND state = 'running' AND lease_expires_at >= now() AND problem_id = $4`,
		result.BuildID, result.WorkerID, result.LeaseToken, problemID, finalState,
		result.Log, result.ErrorMessage, tests, maxBuildLogBytes, solutions)
	if err != nil {
		return err
	}
	affected, err := command.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrStaleLease
	}

	if !publish {
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO problem_testdata
		   (problem_id, data_version, storage_path, sha256, case_count, checker, config_json)
		 VALUES ($1, 1, $2, $3, $4, $5, $6)
		 ON CONFLICT (problem_id) DO UPDATE SET
		   data_version = problem_testdata.data_version + 1,
		   storage_path = EXCLUDED.storage_path, sha256 = EXCLUDED.sha256,
		   case_count = EXCLUDED.case_count, checker = EXCLUDED.checker,
		   config_json = EXCLUDED.config_json`,
		problemID, packagePath, packageSHA, packageCases, checker,
		testManifest(result.Tests)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE problems SET built_revision = $2, last_built_at = now(), updated_at = now()
		 WHERE id = $1`, problemID, revision); err != nil {
		return err
	}
	if err := renderStatementTx(ctx, tx, problemID, result.Tests); err != nil {
		return err
	}
	return tx.Commit()
}

// failExhausted retires builds whose worker died more times than the retry
// budget allows, so the queue cannot spin forever on a poisoned package.
func (s *BuildStore) failExhausted(ctx context.Context) error {
	_, err := s.db.Pool.ExecContext(ctx,
		`UPDATE problem_build_jobs
		 SET state = 'dead', stage = 'done', finished_at = now(), lease_expires_at = NULL,
		     error_message = 'build worker lease expired too many times'
		 WHERE state = 'running' AND lease_expires_at < now() AND attempt >= $1`,
		maxBuildAttempts)
	return err
}

// testManifest is the per-test metadata the judge side keeps alongside the
// data snapshot: groups, points and which tests are samples.
func testManifest(tests []TestOutcome) []byte {
	type manifestTest struct {
		Index    int    `json:"index"`
		Group    string `json:"group,omitempty"`
		Points   int    `json:"points"`
		IsSample bool   `json:"sample"`
	}
	entries := make([]manifestTest, 0, len(tests))
	for _, item := range tests {
		entries = append(entries, manifestTest{
			Index: item.Index, Group: item.Group, Points: item.Points, IsSample: item.IsSample,
		})
	}
	payload, err := json.Marshal(map[string]any{"tests": entries})
	if err != nil {
		return []byte(`{}`)
	}
	return payload
}

func boundedTestOutcomes(tests []TestOutcome) []TestOutcome {
	result := make([]TestOutcome, 0, len(tests))
	for _, item := range tests {
		item.Message = truncate(item.Message, maxOutcomeTextBytes)
		item.InputHead = truncate(item.InputHead, maxSampleTextBytes)
		item.AnswerHead = truncate(item.AnswerHead, maxSampleTextBytes)
		result = append(result, item)
	}
	return result
}

func boundedSolutionOutcomes(solutions []SolutionOutcome) []SolutionOutcome {
	result := make([]SolutionOutcome, 0, len(solutions))
	for _, item := range solutions {
		item.Message = truncate(item.Message, maxOutcomeTextBytes)
		result = append(result, item)
	}
	return result
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n…(truncated)"
}
