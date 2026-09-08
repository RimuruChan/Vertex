-- name: GetProblemOrigin :one
SELECT source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by,copied_at FROM problem_origins WHERE problem_id=$1;

-- name: GetCopySourceDomain :one
SELECT id FROM domains WHERE slug=$1;

-- name: GetCopySourceVersion :one
SELECT id,title,testdata_path,sha256,case_count,checker FROM problem_versions WHERE problem_id=$1 AND version_no=$2;

-- name: GetInheritedAttribution :one
SELECT COALESCE((SELECT attribution FROM problem_origins WHERE problem_id=$1),'')::text;

-- name: CreateCopiedProblem :one
INSERT INTO problems(domain_id,owner_id,author_id,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,visibility,package_revision,data_revision,built_revision)
 SELECT $1,$2,$2,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,'draft',1,1,1
 FROM problem_versions WHERE problem_versions.id=$3 RETURNING id,public_id;

-- name: CopyVersionTestdata :exec
INSERT INTO problem_testdata(problem_id,data_version,data_revision,storage_path,sha256,case_count,checker,spj_source,config_json,samples_json)
 SELECT $1,1,1,$3,$4,$5,checker,spj_source,config_json,samples_json FROM problem_versions WHERE id=$2;

-- name: SaveProblemOrigin :one
INSERT INTO problem_origins(problem_id,source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by,copied_at;

-- name: RecordProblemCopyAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4);


-- name: CopyWorkspaceTags :exec
UPDATE problem_workspaces w SET tags_json=v.tags_json FROM problem_versions v WHERE w.problem_id=sqlc.arg(problem_id)::uuid AND v.id=sqlc.arg(version_id)::bigint;

-- name: CopyVersionStatements :exec
INSERT INTO problem_statements(problem_id,language,name,legend,input_format,output_format,notes,tutorial,scoring)
 SELECT sqlc.arg(problem_id)::uuid,s.language,s.name,s.legend,s.input_format,s.output_format,s.notes,s.tutorial,s.scoring
 FROM problem_versions v CROSS JOIN LATERAL jsonb_to_recordset(v.statements_json) s(language text,name text,legend text,input_format text,output_format text,notes text,tutorial text,scoring text) WHERE v.id=sqlc.arg(version_id)::bigint;

-- name: CopyVersionFiles :exec
INSERT INTO problem_files(problem_id,kind,name,language,source_code,expected_verdict,is_active)
 SELECT sqlc.arg(problem_id)::uuid,f.kind,f.name,f.language,f.source_code,f.expected_verdict,f.is_active
 FROM problem_versions v CROSS JOIN LATERAL jsonb_to_recordset(v.files_json) f(kind text,name text,language text,source_code text,expected_verdict text,is_active boolean) WHERE v.id=sqlc.arg(version_id)::bigint;

-- name: CopyVersionTests :exec
INSERT INTO problem_tests(problem_id,test_index,group_name,source,input_data,generate_cmd,is_sample,points,description)
 SELECT sqlc.arg(problem_id)::uuid,t."Index",t."Group",t."Source",t."InputData",t."GenerateCmd",t."IsSample",t."Points",t."Description"
 FROM problem_versions v CROSS JOIN LATERAL jsonb_to_recordset(COALESCE(NULLIF(v.package_json->'Tests','null'::jsonb),'[]'::jsonb)) t("Index" integer,"Group" text,"Source" text,"InputData" text,"GenerateCmd" text,"IsSample" boolean,"Points" integer,"Description" text) WHERE v.id=sqlc.arg(version_id)::bigint;

-- name: ResolveCopySourceID :one
SELECT id,public_id FROM problems WHERE domain_id=sqlc.arg(domain_id)::uuid AND id=sqlc.arg(problem_id)::uuid;

-- name: ResolveCopySourceNumber :one
SELECT id,public_id FROM problems WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(problem_number)::bigint;
