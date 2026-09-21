-- name: NextProblemVersion :one
SELECT (COALESCE(max(version_no),0)+1)::integer FROM problem_versions WHERE problem_id=$1;

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
