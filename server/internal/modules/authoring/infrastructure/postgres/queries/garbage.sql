-- name: TryGarbageStorageLock :one
SELECT pg_try_advisory_lock(hashtextextended(sqlc.arg(resource_key)::text,0))::boolean;
-- name: ReleaseGarbageStorageLock :exec
SELECT pg_advisory_unlock(hashtextextended(sqlc.arg(resource_key)::text,0));

-- name: GarbageNamespaces :many
SELECT problem_id::text FROM (
 SELECT problem_id FROM problem_content_trees UNION SELECT problem_id FROM problem_blobs UNION SELECT problem_id FROM problem_imports
) candidates WHERE problem_id::text>sqlc.arg(after_id)::text ORDER BY problem_id LIMIT sqlc.arg(page_limit)::integer;

-- name: ExpirePackageImports :execrows
DELETE FROM problem_imports WHERE id IN (
 SELECT id FROM problem_imports WHERE problem_imports.problem_id=sqlc.arg(problem_id) AND expires_at<sqlc.arg(cutoff)::timestamptz ORDER BY expires_at LIMIT 1000
);
-- name: ExpireBlobGrants :execrows
DELETE FROM problem_blob_uploads WHERE ctid IN (
 SELECT ctid FROM problem_blob_uploads WHERE problem_blob_uploads.problem_id=sqlc.arg(problem_id) AND created_at<sqlc.arg(cutoff)::timestamptz ORDER BY created_at LIMIT 1000
);

-- name: PruneAuthoringTrees :execrows
DELETE FROM problem_content_trees WHERE (problem_id,tree_hash) IN (
 SELECT t.problem_id,t.tree_hash FROM problem_content_trees t
 WHERE t.problem_id=sqlc.arg(problem_id) AND t.created_at<sqlc.arg(cutoff)::timestamptz
 AND NOT EXISTS(SELECT 1 FROM problem_commits c WHERE c.problem_id=t.problem_id AND c.tree_hash=t.tree_hash)
 AND NOT EXISTS(SELECT 1 FROM problem_authoring_heads h WHERE h.problem_id=t.problem_id AND (h.tree_hash=t.tree_hash OR h.initial_tree_hash=t.tree_hash))
 AND NOT EXISTS(SELECT 1 FROM problem_working_copies w WHERE w.problem_id=t.problem_id AND w.tree_hash=t.tree_hash)
 AND NOT EXISTS(SELECT 1 FROM problem_merge_sessions m WHERE m.problem_id=t.problem_id AND (m.local_tree=t.tree_hash OR m.remote_tree=t.tree_hash))
 AND NOT EXISTS(SELECT 1 FROM problem_imports i WHERE i.problem_id=t.problem_id AND i.tree_hash=t.tree_hash)
 AND NOT EXISTS(SELECT 1 FROM problem_build_jobs b WHERE b.problem_id=t.problem_id AND b.source_tree_hash=t.tree_hash)
 AND NOT EXISTS(SELECT 1 FROM problem_versions v WHERE v.problem_id=t.problem_id AND v.source_tree_hash=t.tree_hash)
 ORDER BY t.created_at LIMIT 1000
);

-- name: PruneAuthoringBlobs :execrows
DELETE FROM problem_blobs WHERE (problem_id,sha256) IN (
 SELECT b.problem_id,b.sha256 FROM problem_blobs b WHERE b.problem_id=sqlc.arg(problem_id) AND b.created_at<sqlc.arg(cutoff)::timestamptz
 AND NOT EXISTS(SELECT 1 FROM problem_tree_blobs t WHERE t.problem_id=b.problem_id AND t.sha256=b.sha256)
 AND NOT EXISTS(SELECT 1 FROM problem_version_files f WHERE f.problem_id=b.problem_id AND f.blob_sha256=b.sha256)
 AND NOT EXISTS(SELECT 1 FROM problem_blob_uploads u WHERE u.problem_id=b.problem_id AND u.sha256=b.sha256)
 AND NOT EXISTS(SELECT 1 FROM problem_imports i WHERE i.problem_id=b.problem_id AND i.archive_hash=b.sha256)
 AND NOT EXISTS(SELECT 1 FROM problem_merge_sessions m, jsonb_array_elements(CASE WHEN jsonb_typeof(m.result_json#>'{tree,entries}')='array' THEN m.result_json#>'{tree,entries}' ELSE '[]'::jsonb END) e WHERE m.problem_id=b.problem_id AND e#>>'{blob,sha256}'=b.sha256)
 ORDER BY b.created_at LIMIT 1000
);

-- name: RetainedAuthoringBlobs :many
SELECT sha256 FROM problem_blobs WHERE problem_id=$1;
-- name: RetainedAuthoringArtifacts :many
SELECT testdata_path AS storage_path FROM problem_versions WHERE testdata_path<>'' AND testdata_path LIKE sqlc.arg(namespace_id)::text||'/%'
UNION SELECT package_path AS storage_path FROM problem_build_jobs WHERE package_path<>'' AND package_path LIKE sqlc.arg(namespace_id)::text||'/%';
