-- name: ListProblemReleases :many
SELECT version_no,workspace_revision,artifact_version,statement_language,sha256,case_count,created_at FROM problem_versions WHERE problem_id=$1 ORDER BY version_no DESC LIMIT 100;

-- name: GetPublicationWorkspace :one
SELECT p.package_revision,p.data_revision,COALESCE(p.published_version,0),w.title,w.statement_md,w.difficulty,w.source,w.time_limit_ms,w.memory_limit_kb,w.judge_type,w.statement_language,w.tags_json
	 FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id WHERE p.id=$1;

-- name: GetPublicationTestdata :one
SELECT data_version,data_revision,storage_path,sha256,case_count,checker,spj_source,config_json,samples_json
	 FROM problem_testdata WHERE problem_id=$1;

-- name: GetProblemRelease :one
SELECT version_no,workspace_revision,artifact_version,statement_language,sha256,case_count,created_at FROM problem_versions WHERE problem_id=$1 AND version_no=$2;

-- name: SnapshotProblemStatements :one
SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY language),'[]'::jsonb)::jsonb FROM problem_statements s WHERE problem_id=$1;

-- name: SnapshotProblemFiles :one
SELECT COALESCE(jsonb_agg(to_jsonb(f) ORDER BY kind,name),'[]'::jsonb)::jsonb FROM problem_files f WHERE problem_id=$1;

-- name: NextProblemVersion :one
SELECT (COALESCE(max(version_no),0)+1)::integer FROM problem_versions WHERE problem_id=$1;

-- name: CreateProblemRelease :one
INSERT INTO problem_versions
	 (problem_id,version_no,workspace_revision,data_revision,artifact_version,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,tags_json,statements_json,package_json,config_json,testdata_path,sha256,case_count,checker,spj_source,created_by,files_json,samples_json)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25) RETURNING version_no,workspace_revision,artifact_version,statement_language,sha256,case_count,created_at;

-- name: PublishProblem :exec
UPDATE problems SET title=$2,statement_md=$3,difficulty=$4,source=$5,time_limit_ms=$6,memory_limit_kb=$7,judge_type=$8,statement_language=$9,published_version=$10,updated_at=now() WHERE id=$1;

-- name: DeletePublishedProblemTags :exec
DELETE FROM problem_tags WHERE problem_id=$1;

-- name: EnsurePublishedTags :exec
INSERT INTO tags(domain_id,name) SELECT $1,value FROM jsonb_array_elements_text(sqlc.arg(tags_json)::jsonb) WHERE value<>'' ON CONFLICT(domain_id,name) DO NOTHING;

-- name: InsertPublishedProblemTags :exec
INSERT INTO problem_tags(domain_id,problem_id,tag_id) SELECT $1,$2,t.id FROM tags t WHERE t.domain_id=$1 AND t.name IN (SELECT value FROM jsonb_array_elements_text(sqlc.arg(tags_json)::jsonb));

-- name: RecordProblemPublication :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,'problem.publish',$3);
