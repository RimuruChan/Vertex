-- name: FindActiveCheck :one
SELECT id FROM problem_build_jobs WHERE problem_build_jobs.problem_id=sqlc.arg(problem_id) AND source_tree_hash=sqlc.arg(tree_hash)
AND created_by=sqlc.arg(actor_id) AND state IN ('queued','running');

-- name: EnqueueMaterialCheck :one
INSERT INTO problem_build_jobs(problem_id,input_json,source_tree_hash,source_revision,data_hash,check_policy,created_by)
VALUES(sqlc.arg(problem_id),sqlc.arg(input_json),sqlc.arg(tree_hash),sqlc.narg(source_revision),sqlc.arg(data_hash),sqlc.arg(check_policy),sqlc.arg(actor_id))
RETURNING id;

-- name: ReadMaterialCheck :one
SELECT id,source_tree_hash,source_revision,
COALESCE((SELECT min(c.revision) FROM problem_commits c WHERE c.problem_id=problem_build_jobs.problem_id AND c.tree_hash=source_tree_hash),0)::bigint AS matching_revision,
data_hash,check_policy,toolchain_key,state,stage,progress_done,progress_total,log,error_message,
tests_json,solutions_json,validation_json,package_cases,created_at,started_at,finished_at
FROM problem_build_jobs WHERE problem_build_jobs.problem_id=sqlc.arg(problem_id) AND problem_build_jobs.id=sqlc.arg(id) AND source_tree_hash IS NOT NULL
AND (source_revision IS NOT NULL OR created_by=sqlc.arg(actor_id) OR EXISTS(SELECT 1 FROM problem_commits c WHERE c.problem_id=problem_build_jobs.problem_id AND c.tree_hash=source_tree_hash));

-- name: ListMaterialChecks :many
SELECT id,source_tree_hash,source_revision,
COALESCE((SELECT min(c.revision) FROM problem_commits c WHERE c.problem_id=problem_build_jobs.problem_id AND c.tree_hash=source_tree_hash),0)::bigint AS matching_revision,
data_hash,check_policy,toolchain_key,state,stage,progress_done,progress_total,
package_cases,created_at,started_at,finished_at
FROM problem_build_jobs WHERE problem_build_jobs.problem_id=sqlc.arg(problem_id) AND source_tree_hash IS NOT NULL
AND (source_revision IS NOT NULL OR created_by=sqlc.arg(actor_id) OR EXISTS(SELECT 1 FROM problem_commits c WHERE c.problem_id=problem_build_jobs.problem_id AND c.tree_hash=source_tree_hash))
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit)::integer;

-- name: ResolveCheckContent :one
SELECT b.problem_id,o.sha256,o.byte_size FROM problem_build_jobs b
JOIN problem_tree_blobs t ON t.problem_id=b.problem_id AND t.tree_hash=b.source_tree_hash
JOIN problem_blobs o ON o.problem_id=t.problem_id AND o.sha256=t.sha256
WHERE b.id=sqlc.arg(build_id) AND o.sha256=sqlc.arg(sha256) AND b.worker_id=sqlc.arg(worker_id)
AND b.lease_token=sqlc.arg(lease_token) AND b.state='running' AND b.lease_expires_at>clock_timestamp();

-- name: CancelMaterialCheck :exec
UPDATE problem_build_jobs SET state='cancelled',stage='done',finished_at=clock_timestamp(),lease_expires_at=NULL
WHERE problem_build_jobs.problem_id=sqlc.arg(problem_id) AND problem_build_jobs.id=sqlc.arg(id) AND source_tree_hash IS NOT NULL AND state IN ('queued','running')
AND (source_revision IS NOT NULL OR created_by=sqlc.arg(actor_id) OR EXISTS(SELECT 1 FROM problem_commits c WHERE c.problem_id=problem_build_jobs.problem_id AND c.tree_hash=source_tree_hash));

-- name: RecordCheckToolchain :execrows
UPDATE problem_build_jobs SET toolchain_key=sqlc.arg(toolchain_key)
WHERE id=sqlc.arg(id) AND worker_id=sqlc.arg(worker_id) AND lease_token=sqlc.arg(lease_token)
AND source_tree_hash IS NOT NULL AND state='running' AND lease_expires_at>clock_timestamp();

-- name: ReadCheckBinding :one
SELECT source_tree_hash,source_revision,data_hash,check_policy FROM problem_build_jobs WHERE id=$1;
