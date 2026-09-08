-- name: GetProblemFile :one
SELECT id, problem_id, kind, name, language, source_code,
	expected_verdict, is_active, created_at, updated_at FROM problem_files WHERE problem_files.problem_id = $1 AND problem_files.id = $2
		 AND EXISTS (SELECT 1 FROM problems WHERE problems.id = $1 AND domain_id = $3);

-- name: GetProblemTest :one
SELECT id, problem_id, test_index, group_name, source, input_data,
	generate_cmd, is_sample, points, description FROM problem_tests WHERE problem_tests.problem_id =$1 AND id=$2;

-- name: GetPackageSnapshotHeader :one
SELECT p.id,w.title,w.time_limit_ms,w.memory_limit_kb,w.judge_type,p.package_revision,p.domain_id,p.data_revision
		 FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id WHERE p.id=$1;

-- name: GetPackageMetadata :one
SELECT problem.id, problem.public_id, w.title, problem.visibility, w.judge_type,
		        w.statement_language, w.time_limit_ms, w.memory_limit_kb,
		        problem.package_revision, COALESCE(testdata.data_revision,0)::integer AS built_revision, problem.last_built_at,
		        COALESCE(testdata.case_count, 0), COALESCE(testdata.checker, ''),
		        COALESCE(testdata.data_version, 0), COALESCE(testdata.sha256, ''), problem.data_revision AS data_revision,
		        COALESCE(problem.published_version,0), COALESCE(v.workspace_revision,-1)::integer AS published_revision,COALESCE(v.artifact_version,0)::integer AS published_artifact_version
		 FROM problems AS problem
		 JOIN problem_workspaces w ON w.problem_id=problem.id
		 LEFT JOIN problem_testdata AS testdata ON testdata.problem_id = problem.id
		 LEFT JOIN problem_versions v ON v.problem_id=problem.id AND v.version_no=problem.published_version
		 WHERE problem.id = $1 AND problem.domain_id = $2;


-- name: ListPackageFiles :many
SELECT id, problem_id, kind, name, language,
 CASE WHEN sqlc.arg(include_source)::boolean THEN source_code ELSE '' END::text AS source_code,
 expected_verdict, is_active, created_at, updated_at
FROM problem_files WHERE problem_id = sqlc.arg(problem_id)::uuid
ORDER BY kind, is_active DESC, name;

-- name: ListPackageTests :many
SELECT id, problem_id, test_index, group_name, source,
 CASE WHEN sqlc.arg(include_input)::boolean THEN input_data ELSE left(input_data,512) END::text AS input_data,
 generate_cmd, is_sample, points, description
FROM problem_tests WHERE problem_id = sqlc.arg(problem_id)::uuid ORDER BY test_index;
