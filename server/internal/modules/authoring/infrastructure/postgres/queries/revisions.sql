-- name: RegisterAuthoringBlob :exec
INSERT INTO problem_blobs(problem_id,sha256,byte_size) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: GrantAuthoringBlobUpload :exec
INSERT INTO problem_blob_uploads(problem_id,sha256,actor_id) VALUES($1,$2,$3)
ON CONFLICT(problem_id,sha256,actor_id) DO UPDATE SET created_at=now();

-- name: ReadAuthoringBlob :one
SELECT b.sha256,b.byte_size FROM problem_blobs b
WHERE b.problem_id=sqlc.arg(problem_id) AND b.sha256=sqlc.arg(sha256)
AND (
    EXISTS(SELECT 1 FROM problem_blob_uploads u WHERE u.problem_id=b.problem_id AND u.sha256=b.sha256 AND u.actor_id=sqlc.arg(actor_id))
    OR EXISTS(SELECT 1 FROM problem_version_files f WHERE f.problem_id=b.problem_id AND f.blob_sha256=b.sha256)
    OR EXISTS(SELECT 1 FROM problem_tree_blobs tb JOIN problem_commits c USING(problem_id,tree_hash) WHERE tb.problem_id=b.problem_id AND tb.sha256=b.sha256)
    OR EXISTS(SELECT 1 FROM problem_tree_blobs tb JOIN problem_working_copies w USING(problem_id,tree_hash) WHERE tb.problem_id=b.problem_id AND tb.sha256=b.sha256 AND w.actor_id=sqlc.arg(actor_id))
    OR EXISTS(SELECT 1 FROM problem_tree_blobs tb JOIN problem_build_jobs j ON j.problem_id=tb.problem_id AND j.source_tree_hash=tb.tree_hash WHERE tb.problem_id=b.problem_id AND tb.sha256=b.sha256 AND j.created_by=sqlc.arg(actor_id))
    OR EXISTS(SELECT 1 FROM problem_merge_sessions m, jsonb_array_elements(CASE WHEN jsonb_typeof(m.result_json#>'{tree,entries}')='array' THEN m.result_json#>'{tree,entries}' ELSE '[]'::jsonb END) e WHERE m.problem_id=b.problem_id AND m.actor_id=sqlc.arg(actor_id) AND e#>>'{blob,sha256}'=b.sha256)
);

-- name: InsertAuthoringTree :exec
INSERT INTO problem_content_trees(problem_id,tree_hash,manifest,summary) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: InsertAuthoringTreeBlobs :exec
INSERT INTO problem_tree_blobs(problem_id,tree_hash,sha256)
SELECT sqlc.arg(problem_id)::uuid,sqlc.arg(tree_hash)::text,unnest(sqlc.arg(digests)::text[])
ON CONFLICT DO NOTHING;

-- name: AvailableAuthoringBlobs :many
SELECT b.sha256,b.byte_size FROM problem_blobs b
WHERE b.problem_id=sqlc.arg(problem_id) AND b.sha256=ANY(sqlc.arg(digests)::text[])
AND (
    EXISTS(SELECT 1 FROM problem_blob_uploads u WHERE u.problem_id=b.problem_id AND u.sha256=b.sha256 AND u.actor_id=sqlc.arg(actor_id))
    OR EXISTS(SELECT 1 FROM problem_tree_blobs tb JOIN problem_commits c USING(problem_id,tree_hash) WHERE tb.problem_id=b.problem_id AND tb.sha256=b.sha256)
    OR EXISTS(SELECT 1 FROM problem_tree_blobs tb JOIN problem_working_copies w USING(problem_id,tree_hash) WHERE tb.problem_id=b.problem_id AND tb.sha256=b.sha256 AND w.actor_id=sqlc.arg(actor_id))
    OR EXISTS(SELECT 1 FROM problem_tree_blobs tb JOIN problem_build_jobs j ON j.problem_id=tb.problem_id AND j.source_tree_hash=tb.tree_hash WHERE tb.problem_id=b.problem_id AND tb.sha256=b.sha256 AND j.created_by=sqlc.arg(actor_id))
    OR EXISTS(SELECT 1 FROM problem_merge_sessions m, jsonb_array_elements(CASE WHEN jsonb_typeof(m.result_json#>'{tree,entries}')='array' THEN m.result_json#>'{tree,entries}' ELSE '[]'::jsonb END) e WHERE m.problem_id=b.problem_id AND m.actor_id=sqlc.arg(actor_id) AND e#>>'{blob,sha256}'=b.sha256)
);

-- name: UpdateMergeDraft :execrows
UPDATE problem_merge_sessions SET result_json=sqlc.arg(result_json),etag=gen_random_uuid(),updated_at=now()
WHERE problem_id=sqlc.arg(problem_id) AND actor_id=sqlc.arg(actor_id) AND id=sqlc.arg(id) AND etag=sqlc.arg(etag);

-- name: ReadAuthoringTree :one
SELECT manifest FROM problem_content_trees WHERE problem_id=$1 AND tree_hash=$2;

-- name: InitializeAuthoringHead :exec
INSERT INTO problem_authoring_heads(problem_id,tree_hash,initial_tree_hash)
VALUES(sqlc.arg(problem_id),sqlc.arg(tree_hash),sqlc.arg(tree_hash)) ON CONFLICT DO NOTHING;

-- name: InitialProblemMaterial :one
SELECT title,statement_md,statement_language,source,judge_type,time_limit_ms,memory_limit_kb,difficulty FROM problems WHERE id=$1;

-- name: ReadInitialAuthoringTree :one
SELECT initial_tree_hash FROM problem_authoring_heads WHERE problem_id=$1;

-- name: ReadAuthoringHead :one
SELECT revision,tree_hash FROM problem_authoring_heads WHERE problem_id=$1;

-- name: StartWorkingCopy :exec
INSERT INTO problem_working_copies(problem_id,actor_id,base_revision,tree_hash)
SELECT h.problem_id,sqlc.arg(actor_id),h.revision,h.tree_hash FROM problem_authoring_heads h WHERE h.problem_id=sqlc.arg(problem_id)
ON CONFLICT DO NOTHING;

-- name: ReadWorkingCopy :one
SELECT w.base_revision,w.tree_hash,w.etag,w.updated_at,h.revision AS head_revision,
       COALESCE(m.id::text,'')::text AS merge_id
FROM problem_working_copies w JOIN problem_authoring_heads h USING(problem_id)
LEFT JOIN problem_merge_sessions m ON m.problem_id=w.problem_id AND m.actor_id=w.actor_id
WHERE w.problem_id=$1 AND w.actor_id=$2;

-- name: ReplaceWorkingCopy :execrows
UPDATE problem_working_copies SET tree_hash=sqlc.arg(tree_hash),base_revision=sqlc.narg(base_revision),etag=gen_random_uuid(),updated_at=now()
WHERE problem_id=sqlc.arg(problem_id) AND actor_id=sqlc.arg(actor_id) AND etag=sqlc.arg(etag);

-- name: InsertContentCommit :one
INSERT INTO problem_commits(problem_id,revision,parent_revision,tree_hash,author_id,message,request_id,request_etag)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: ReadContentCommit :one
SELECT * FROM problem_commits WHERE problem_id=$1 AND revision=$2;

-- name: FindContentCommitRequest :one
SELECT * FROM problem_commits WHERE problem_id=sqlc.arg(problem_id) AND author_id=sqlc.arg(actor_id) AND request_id=sqlc.arg(request_id);

-- name: ListContentCommits :many
SELECT * FROM problem_commits WHERE problem_id=sqlc.arg(problem_id)
AND (sqlc.arg(before_revision)::bigint=0 OR revision<sqlc.arg(before_revision))
ORDER BY revision DESC LIMIT sqlc.arg(page_limit)::integer;

-- name: AdvanceAuthoringHead :exec
UPDATE problem_authoring_heads SET revision=$2,tree_hash=$3 WHERE problem_id=$1;

-- name: SaveMergeSession :one
INSERT INTO problem_merge_sessions(problem_id,actor_id,copy_etag,base_revision,remote_revision,local_tree,remote_tree,result_json)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(problem_id,actor_id) DO UPDATE SET id=gen_random_uuid(),etag=gen_random_uuid(),copy_etag=EXCLUDED.copy_etag,
base_revision=EXCLUDED.base_revision,remote_revision=EXCLUDED.remote_revision,local_tree=EXCLUDED.local_tree,
remote_tree=EXCLUDED.remote_tree,result_json=EXCLUDED.result_json,updated_at=now()
RETURNING *;

-- name: ReadMergeSession :one
SELECT * FROM problem_merge_sessions WHERE problem_id=$1 AND actor_id=$2;

-- name: DeleteMergeSession :exec
DELETE FROM problem_merge_sessions WHERE problem_id=$1 AND actor_id=$2;
