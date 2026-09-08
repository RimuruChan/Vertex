-- name: BumpPackageRevision :one
UPDATE problems SET package_revision = package_revision + 1, data_revision=data_revision+CASE WHEN sqlc.arg(data_changed)::boolean THEN 1 ELSE 0 END
		 WHERE id = $1 AND domain_id = $2
		 RETURNING package_revision;

-- name: ListProblemStatements :many
SELECT problem_id, language, name, legend, input_format, output_format,
		        notes, tutorial, scoring, updated_at
		 FROM problem_statements WHERE problem_id = $1
		 AND EXISTS (SELECT 1 FROM problems WHERE id = $1 AND domain_id = $2)
		 ORDER BY language;

-- name: UpsertProblemStatement :one
INSERT INTO problem_statements
			   (problem_id, language, name, legend, input_format, output_format, notes, tutorial, scoring)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (problem_id, language) DO UPDATE SET
			   name = EXCLUDED.name, legend = EXCLUDED.legend,
			   input_format = EXCLUDED.input_format, output_format = EXCLUDED.output_format,
			   notes = EXCLUDED.notes, tutorial = EXCLUDED.tutorial,
			   scoring = EXCLUDED.scoring, updated_at = now()
			 RETURNING problem_id, language, name, legend, input_format, output_format,
			           notes, tutorial, scoring, updated_at;

-- name: DeleteProblemStatement :execrows
DELETE FROM problem_statements WHERE problem_id = $1 AND language = $2;

-- name: ClearDeletedWorkspaceStatement :exec
UPDATE problem_workspaces SET statement_md='',updated_at=now() WHERE problem_id=$1 AND statement_language=$2;

-- name: DeactivateOtherProblemFiles :exec
UPDATE problem_files SET is_active = FALSE, updated_at = now()
				 WHERE problem_files.problem_id = $1 AND kind = $2 AND is_active AND name <> $3;

-- name: UpsertProblemFile :one
INSERT INTO problem_files
			   (problem_id, kind, name, language, source_code, expected_verdict, is_active)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (problem_id, kind, name) DO UPDATE SET
			   language = EXCLUDED.language, source_code = EXCLUDED.source_code,
			   expected_verdict = EXCLUDED.expected_verdict,
			   is_active = EXCLUDED.is_active, updated_at = now()
			 RETURNING id, problem_id, kind, name, language, source_code,
	expected_verdict, is_active, created_at, updated_at;

-- name: DeleteProblemFile :execrows
DELETE FROM problem_files WHERE problem_files.problem_id = $1 AND problem_files.id = $2;

-- name: CreateProblemTest :one
INSERT INTO problem_tests
			   (problem_id, test_index, group_name, source, input_data, generate_cmd,
			    is_sample, points, description)
			 VALUES ($1,
			   COALESCE((SELECT max(test_index) FROM problem_tests WHERE problem_tests.problem_id = $1), 0) + 1,
			   $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id, problem_id, test_index, group_name, source, input_data,
	generate_cmd, is_sample, points, description;

-- name: UpdateProblemTest :one
UPDATE problem_tests
			 SET group_name = $3, source = $4, input_data = $5, generate_cmd = $6,
			     is_sample = $7, points = $8, description = $9
			 WHERE problem_tests.problem_id = $1 AND problem_tests.id = $2
			 RETURNING id, problem_id, test_index, group_name, source, input_data,
	generate_cmd, is_sample, points, description;

-- name: DeleteProblemTest :one
DELETE FROM problem_tests WHERE problem_tests.problem_id = $1 AND problem_tests.id = $2 RETURNING test_index;

-- name: CloseTestOrderGap :exec
UPDATE problem_tests SET test_index = test_index - 1
			 WHERE problem_tests.problem_id = $1 AND test_index > $2;

-- name: GetTestOrderBounds :one
SELECT test_index,
			        (SELECT count(*)::int FROM problem_tests WHERE problem_tests.problem_id = $1)::integer AS total_tests
			 FROM problem_tests WHERE problem_tests.problem_id = $1 AND problem_tests.id = $2;

-- name: ParkTestForReorder :exec
UPDATE problem_tests
			 SET test_index = (SELECT max(test_index) + 1 FROM problem_tests WHERE problem_tests.problem_id = $1)
			 WHERE problem_tests.problem_id = $1 AND problem_tests.id = $2;

-- name: ShiftTestsRight :exec
UPDATE problem_tests SET test_index = test_index + 1
				 WHERE problem_tests.problem_id = sqlc.arg(problem_id)::uuid AND test_index >= sqlc.arg(range_start)::integer AND test_index < sqlc.arg(range_end)::integer;

-- name: ShiftTestsLeft :exec
UPDATE problem_tests SET test_index = test_index - 1
				 WHERE problem_tests.problem_id = sqlc.arg(problem_id)::uuid AND test_index > sqlc.arg(range_start)::integer AND test_index <= sqlc.arg(range_end)::integer;

-- name: SetTestOrder :exec
UPDATE problem_tests SET test_index = $3 WHERE problem_tests.problem_id = $1 AND problem_tests.id = $2;
