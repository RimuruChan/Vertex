-- name: ReadCopyRelease :one
SELECT v.id,v.title,v.source_revision,v.source_tree_hash,v.testdata_path,v.sha256,v.case_count,v.config_json,
v.toolchain_key,b.data_hash,b.check_policy
FROM problem_versions v JOIN problem_build_jobs b ON b.id=v.check_id AND b.problem_id=v.problem_id
WHERE v.problem_id=$1 AND v.version_no=$2 AND v.source_revision IS NOT NULL AND v.source_tree_hash IS NOT NULL AND b.state='succeeded' AND v.checker='artifact';

-- name: CreateRevisionCopy :one
INSERT INTO problems(domain_id,owner_id,author_id,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,visibility)
SELECT $1,$2,$2,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,'private'
FROM problem_versions WHERE problem_versions.id=$3 RETURNING id,public_id;

-- name: CreateCopiedCheck :one
INSERT INTO problem_build_jobs(problem_id,input_json,source_tree_hash,data_hash,check_policy,toolchain_key,state,stage,
progress_done,progress_total,log,tests_json,solutions_json,validation_json,package_path,package_sha256,package_cases,package_manifest,created_by,finished_at)
VALUES(sqlc.arg(problem_id),sqlc.arg(input_json),sqlc.arg(tree_hash),sqlc.arg(data_hash),sqlc.arg(check_policy),sqlc.arg(toolchain_key),'succeeded','copied',
sqlc.arg(case_count),sqlc.arg(case_count),sqlc.arg(log),sqlc.arg(tests_json),'[]',sqlc.arg(validation_json),sqlc.arg(package_path),sqlc.arg(package_sha256),sqlc.arg(case_count),sqlc.arg(package_manifest),sqlc.arg(actor_id),now()) RETURNING id;
