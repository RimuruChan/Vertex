-- name: LockProblemPackageRevision :one
SELECT package_revision FROM problems WHERE id = sqlc.arg(problem_id) AND domain_id = sqlc.arg(domain_id) FOR UPDATE;

-- name: GetActiveBuild :one
SELECT id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM problem_build_jobs
		 WHERE problem_build_jobs.problem_id = sqlc.arg(problem_id) AND state IN ('queued', 'running');

-- name: CreateBuild :one
INSERT INTO problem_build_jobs (problem_id, revision, created_by, data_revision, input_json)
		 VALUES (sqlc.arg(problem_id), sqlc.arg(revision), sqlc.arg(created_by), sqlc.arg(data_revision), sqlc.arg(input_json))
		 RETURNING id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at;

-- name: NotifyBuildJob :exec
SELECT pg_notify('vertex_problem_builds', sqlc.arg(build_id));

-- name: GetBuild :one
SELECT id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM problem_build_jobs WHERE problem_build_jobs.id = sqlc.arg(build_id) AND problem_id = sqlc.arg(problem_id)
		 AND EXISTS (SELECT 1 FROM problems WHERE problems.id = sqlc.arg(problem_id) AND domain_id = sqlc.arg(domain_id));

-- name: GetLatestBuild :one
SELECT id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM problem_build_jobs
		 WHERE problem_build_jobs.problem_id = sqlc.arg(problem_id) AND EXISTS (SELECT 1 FROM problems WHERE problems.id = sqlc.arg(problem_id) AND domain_id = sqlc.arg(domain_id))
		 ORDER BY created_at DESC LIMIT 1;

-- name: GetLatestSuccessfulBuild :one
SELECT id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM problem_build_jobs
		 WHERE problem_build_jobs.problem_id = sqlc.arg(problem_id) AND state = 'succeeded'
		 AND EXISTS (SELECT 1 FROM problems WHERE problems.id = sqlc.arg(problem_id) AND domain_id = sqlc.arg(domain_id))
		 ORDER BY finished_at DESC NULLS LAST LIMIT 1;

-- name: ListBuilds :many
SELECT id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM problem_build_jobs
		 WHERE problem_build_jobs.problem_id = sqlc.arg(problem_id) AND EXISTS (SELECT 1 FROM problems WHERE problems.id = sqlc.arg(problem_id) AND domain_id = sqlc.arg(domain_id))
		 ORDER BY created_at DESC LIMIT sqlc.arg(page_limit)::integer;

-- name: LockBuildForCancellation :one
SELECT id FROM problem_build_jobs WHERE problem_build_jobs.id =sqlc.arg(build_id) AND problem_id=sqlc.arg(problem_id)
	 AND EXISTS(SELECT 1 FROM problems WHERE problems.id =sqlc.arg(problem_id) AND domain_id=sqlc.arg(domain_id)) FOR UPDATE;

-- name: CancelBuild :execrows
UPDATE problem_build_jobs
		 SET state = 'cancelled', stage = 'done', finished_at = now(),
		     lease_expires_at = NULL, error_message = 'cancelled by author'
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND problem_id = sqlc.arg(problem_id) AND state IN ('queued', 'running')
		 AND EXISTS (SELECT 1 FROM problems WHERE problems.id = sqlc.arg(problem_id) AND domain_id = sqlc.arg(domain_id));

-- name: ClaimBuild :one
WITH candidate AS (
		   SELECT id FROM problem_build_jobs
		   WHERE (state = 'queued' AND available_at <= now())
		      OR (state = 'running' AND lease_expires_at < now() AND problem_build_jobs.attempt < sqlc.arg(max_attempts))
		   ORDER BY priority DESC, available_at, created_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 ), claimed AS (
		   UPDATE problem_build_jobs AS job
		   SET state = 'running', stage = 'compile', attempt = job.attempt + 1,
		       worker_id = sqlc.arg(worker_id)::text, lease_token = gen_random_uuid(),
		       lease_expires_at = now() + (sqlc.arg(lease_ms)::bigint * interval '1 millisecond'),
		       started_at = COALESCE(job.started_at, now()), error_message = '',
		       package_path='',package_sha256='',package_cases=0,
		       tests_json='[]',solutions_json='[]',progress_done=0,progress_total=0
		   FROM candidate
		   WHERE job.id = candidate.id
		   RETURNING job.*
		 )
		 SELECT id, problem_id, revision, data_revision, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM claimed;

-- name: GetBuildInput :one
SELECT b.input_json,p.domain_id FROM problem_build_jobs b JOIN problems p ON p.id=b.problem_id WHERE b.id=sqlc.arg(build_id);

-- name: RenewBuildLease :one
WITH renewed AS (
		   UPDATE problem_build_jobs
		   SET lease_expires_at = now() + (sqlc.arg(lease_ms)::bigint * interval '1 millisecond'),
		       stage = COALESCE(NULLIF(sqlc.arg(stage)::text, ''), stage),
		       progress_done = GREATEST(progress_done, sqlc.arg(progress_done)),
		       progress_total = GREATEST(progress_total, sqlc.arg(progress_total)),
		       log = left(log || sqlc.arg(log), sqlc.arg(log_limit))
		   WHERE problem_build_jobs.id = sqlc.arg(build_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		     AND state = 'running' AND lease_expires_at >= now()
		   RETURNING 1
		 )
		 SELECT count(*)::int FROM renewed;

-- name: GetBuildUploadTarget :one
SELECT problem_id FROM problem_build_jobs
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		   AND state = 'running' AND lease_expires_at >= now();

-- name: RecordBuildArtifact :execrows
UPDATE problem_build_jobs
		 SET package_path = sqlc.arg(package_path), package_sha256 = sqlc.arg(package_sha256), package_cases = sqlc.arg(package_cases), stage = 'package'
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND problem_id = sqlc.arg(problem_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		   AND state = 'running' AND lease_expires_at >= now();

-- name: LockBuildForCompletion :one
SELECT state, problem_id, package_path, package_sha256, package_cases, revision, data_revision
		 FROM problem_build_jobs WHERE problem_build_jobs.id = sqlc.arg(build_id) FOR UPDATE;

-- name: CompleteBuild :execrows
UPDATE problem_build_jobs
		 SET state = sqlc.arg(state), stage = 'done', finished_at = now(), lease_expires_at = NULL,
		     log = left(log || sqlc.arg(log), sqlc.arg(log_limit)), error_message = left(sqlc.arg(error_message), 4096),
		     tests_json = sqlc.arg(tests_json), solutions_json = sqlc.arg(solutions_json),
		     progress_done = GREATEST(progress_done, progress_total)
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		   AND state = 'running' AND lease_expires_at >= now() AND problem_id = sqlc.arg(problem_id);

-- name: LockProblemDataRevision :one
SELECT data_revision FROM problems WHERE id=sqlc.arg(problem_id) FOR UPDATE;

-- name: SaveBuiltTestdata :exec
INSERT INTO problem_testdata
		   (problem_id, data_version, storage_path, sha256, case_count, checker, config_json, data_revision, build_id, samples_json)
		 VALUES (sqlc.arg(problem_id), 1, sqlc.arg(storage_path), sqlc.arg(sha256), sqlc.arg(case_count), sqlc.arg(checker), sqlc.arg(config_json), sqlc.arg(data_revision), sqlc.arg(build_id), sqlc.arg(samples_json))
		 ON CONFLICT (problem_id) DO UPDATE SET
		   data_version = problem_testdata.data_version + 1,
		   storage_path = EXCLUDED.storage_path, sha256 = EXCLUDED.sha256,
		   case_count = EXCLUDED.case_count, checker = EXCLUDED.checker,
		   config_json = EXCLUDED.config_json, spj_source='',
		   data_revision=EXCLUDED.data_revision,build_id=EXCLUDED.build_id,samples_json=EXCLUDED.samples_json;

-- name: MarkProblemBuilt :exec
UPDATE problems SET built_revision = sqlc.arg(built_revision), last_built_at = now()
		 WHERE id = sqlc.arg(problem_id);

-- name: FailExhaustedBuilds :exec
UPDATE problem_build_jobs
		 SET state = 'dead', stage = 'done', finished_at = now(), lease_expires_at = NULL,
		     error_message = 'build worker lease expired too many times'
		 WHERE state = 'running' AND lease_expires_at < now() AND attempt >= sqlc.arg(max_attempts);
