-- name: GetWorkspaceStatementLanguage :one
SELECT statement_language FROM problem_workspaces WHERE problem_id = $1;

-- name: SaveRenderedStatement :exec
UPDATE problem_workspaces SET statement_md = sqlc.arg(statement_md)::text, title = COALESCE(NULLIF(sqlc.arg(statement_name)::text, ''), title), updated_at = now()
		 WHERE problem_id = sqlc.arg(problem_id)::uuid;

-- name: GetProblemStatement :one
SELECT problem_id, language, name, legend, input_format, output_format,
		        notes, tutorial, scoring, updated_at
		 FROM problem_statements WHERE problem_id = $1 AND language = $2;

-- name: GetBuiltSamples :one
SELECT samples_json FROM problem_testdata WHERE problem_id=$1;
