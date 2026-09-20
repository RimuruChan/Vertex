-- name: NotifyBuildJob :exec
SELECT pg_notify('vertex_problem_builds', sqlc.arg(build_id));

-- name: ClaimBuild :one
WITH candidate AS (
		   SELECT id FROM problem_build_jobs
		   WHERE ((state = 'queued' AND available_at <= now())
		      OR (state = 'running' AND lease_expires_at < clock_timestamp() AND problem_build_jobs.attempt < sqlc.arg(max_attempts)))
		      AND source_tree_hash IS NOT NULL AND sqlc.arg(accepts_checks)::boolean
		   ORDER BY priority DESC, available_at, created_at
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 ), claimed AS (
		   UPDATE problem_build_jobs AS job
		   SET state = 'running', stage = 'compile', attempt = job.attempt + 1,
		       worker_id = sqlc.arg(worker_id)::text, lease_token = gen_random_uuid(),
		       lease_expires_at = clock_timestamp() + (sqlc.arg(lease_ms)::bigint * interval '1 millisecond'),
		       started_at = COALESCE(job.started_at, now()), error_message = '',
		       package_path='',package_sha256='',package_cases=0,package_manifest='{}',toolchain_key='',
		       tests_json='[]',solutions_json='[]',validation_json='[]',progress_done=0,progress_total=0
		   FROM candidate
		   WHERE job.id = candidate.id
		   RETURNING job.*
		 )
		 SELECT id, problem_id, (SELECT pnum.public_id::text FROM problems pnum WHERE pnum.id=problem_build_jobs.problem_id)::text AS problem_number, state, stage, attempt,
	COALESCE(worker_id, '')::text AS worker_id, COALESCE(lease_token::text, '')::text AS lease_token,
	COALESCE(lease_expires_at, TIMESTAMPTZ 'epoch')::timestamptz AS lease_expires_at,
	progress_done, progress_total, log, error_message, tests_json, solutions_json,
	package_path, package_sha256, package_cases, created_by, created_at, started_at, finished_at FROM claimed AS problem_build_jobs;

-- name: GetBuildInput :one
SELECT b.input_json,p.domain_id FROM problem_build_jobs b JOIN problems p ON p.id=b.problem_id WHERE b.id=sqlc.arg(build_id);

-- name: RenewBuildLease :one
WITH renewed AS (
		   UPDATE problem_build_jobs
		   SET lease_expires_at = clock_timestamp() + (sqlc.arg(lease_ms)::bigint * interval '1 millisecond'),
		       stage = COALESCE(NULLIF(sqlc.arg(stage)::text, ''), stage),
		       progress_done = GREATEST(progress_done, sqlc.arg(progress_done)),
		       progress_total = GREATEST(progress_total, sqlc.arg(progress_total)),
		       log = left(log || sqlc.arg(log), sqlc.arg(log_limit))
		   WHERE problem_build_jobs.id = sqlc.arg(build_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		     AND state = 'running' AND lease_expires_at > clock_timestamp()
		   RETURNING 1
		 )
		 SELECT count(*)::int FROM renewed;

-- name: GetBuildUploadTarget :one
SELECT problem_id FROM problem_build_jobs
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		   AND state = 'running' AND lease_expires_at > clock_timestamp();

-- name: RecordBuildArtifact :execrows
UPDATE problem_build_jobs
		 SET package_path = sqlc.arg(package_path), package_sha256 = sqlc.arg(package_sha256), package_cases = sqlc.arg(package_cases), package_manifest=sqlc.arg(package_manifest), stage = 'package'
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND problem_id = sqlc.arg(problem_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		   AND state = 'running' AND lease_expires_at > clock_timestamp();

-- name: LockBuildForCompletion :one
SELECT state, problem_id, package_path, package_sha256, package_cases, package_manifest
		 FROM problem_build_jobs WHERE problem_build_jobs.id = sqlc.arg(build_id) FOR UPDATE;

-- name: CompleteBuild :execrows
UPDATE problem_build_jobs
		 SET state = sqlc.arg(state), stage = 'done', finished_at = clock_timestamp(), lease_expires_at = NULL,
		     log = left(log || sqlc.arg(log), sqlc.arg(log_limit)), error_message = left(sqlc.arg(error_message), 4096),
		     tests_json = sqlc.arg(tests_json), solutions_json = sqlc.arg(solutions_json), validation_json=sqlc.arg(validation_json),
		     progress_done = GREATEST(progress_done, progress_total)
		 WHERE problem_build_jobs.id = sqlc.arg(build_id) AND worker_id = sqlc.arg(worker_id)::text AND lease_token = sqlc.arg(lease_token)::uuid
		   AND state = 'running' AND lease_expires_at > clock_timestamp() AND problem_id = sqlc.arg(problem_id);

-- name: FailExhaustedBuilds :exec
UPDATE problem_build_jobs
		 SET state = 'dead', stage = 'done', finished_at = clock_timestamp(), lease_expires_at = NULL,
		     error_message = 'build worker lease expired too many times'
		 WHERE state = 'running' AND lease_expires_at < clock_timestamp() AND attempt >= sqlc.arg(max_attempts);
