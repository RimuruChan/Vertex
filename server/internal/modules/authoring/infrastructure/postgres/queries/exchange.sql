-- name: RegisterImportBlobs :exec
INSERT INTO problem_blobs(problem_id,sha256,byte_size)
SELECT sqlc.arg(problem_id)::uuid,unnest(sqlc.arg(hashes)::text[]),unnest(sqlc.arg(sizes)::bigint[])
ON CONFLICT DO NOTHING;

-- name: GrantImportBlobs :exec
INSERT INTO problem_blob_uploads(problem_id,sha256,actor_id)
SELECT sqlc.arg(problem_id)::uuid,unnest(sqlc.arg(hashes)::text[]),sqlc.arg(actor_id)::uuid
ON CONFLICT(problem_id,sha256,actor_id) DO UPDATE SET created_at=now();

-- name: CreatePackageImport :one
INSERT INTO problem_imports(problem_id,actor_id,base_etag,archive_hash,tree_hash,plan_json)
VALUES($1,$2,$3,$4,$5,$6) RETURNING id,base_etag,plan_json,expires_at,applied_at;

-- name: ReadPackageImport :one
SELECT id,base_etag,plan_json,expires_at,applied_at FROM problem_imports WHERE problem_id=$1 AND actor_id=$2 AND id=$3;

-- name: ApplyPackageImport :execrows
UPDATE problem_imports SET applied_at=now() WHERE problem_id=$1 AND actor_id=$2 AND id=$3 AND applied_at IS NULL AND expires_at>now();
-- name: FindExportCheck :one
SELECT package_path,package_manifest,toolchain_key,data_hash,check_policy
FROM problem_build_jobs WHERE problem_build_jobs.problem_id=sqlc.arg(problem_id) AND data_hash=sqlc.arg(data_hash)
AND check_policy=sqlc.arg(check_policy) AND state='succeeded' AND source_tree_hash IS NOT NULL
AND (created_by=sqlc.arg(actor_id) OR source_revision IS NOT NULL OR EXISTS(
 SELECT 1 FROM problem_commits c WHERE c.problem_id=problem_build_jobs.problem_id AND c.tree_hash=source_tree_hash))
ORDER BY finished_at DESC,id DESC LIMIT 1;
