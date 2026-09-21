-- name: CurrentReleasedVersion :one
SELECT COALESCE(published_version,0)::integer FROM problems WHERE id=$1;

-- name: PublicationCheckArtifact :one
SELECT package_path,package_sha256,package_cases,package_manifest,toolchain_key,data_hash,check_policy
FROM problem_build_jobs WHERE problem_id=$1 AND id=$2 AND source_tree_hash IS NOT NULL AND state='succeeded';

-- name: ReadCommittedRelease :one
SELECT version_no,source_revision,source_tree_hash,check_id,toolchain_key,statement_language,created_at
FROM problem_versions WHERE problem_id=$1 AND version_no=$2 AND source_revision IS NOT NULL;

-- name: ListCommittedReleases :many
SELECT version_no,source_revision,source_tree_hash,check_id,toolchain_key,statement_language,created_at
FROM problem_versions WHERE problem_id=$1 AND source_revision IS NOT NULL ORDER BY version_no DESC LIMIT 100;

-- name: CreateCommittedRelease :one
INSERT INTO problem_versions(problem_id,version_no,source_revision,source_tree_hash,check_id,toolchain_key,
title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,tags_json,config_json,testdata_path,sha256,case_count,checker,created_by)
VALUES(sqlc.arg(problem_id),sqlc.arg(version_no),sqlc.arg(source_revision),sqlc.arg(tree_hash),sqlc.arg(check_id),sqlc.arg(toolchain_key),
sqlc.arg(title),sqlc.arg(statement_md),sqlc.arg(difficulty),sqlc.arg(source),sqlc.arg(time_limit_ms),sqlc.arg(memory_limit_kb),sqlc.arg(judge_type),sqlc.arg(statement_language),sqlc.arg(tags_json),sqlc.arg(config_json),sqlc.arg(testdata_path),sqlc.arg(sha256),sqlc.arg(case_count),'artifact',sqlc.arg(actor_id))
RETURNING version_no,source_revision,source_tree_hash,check_id,toolchain_key,statement_language,created_at;
