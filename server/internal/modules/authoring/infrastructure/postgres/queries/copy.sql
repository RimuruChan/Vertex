-- name: GetProblemOrigin :one
SELECT source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by,copied_at FROM problem_origins WHERE problem_id=$1;

-- name: GetCopySourceDomain :one
SELECT id FROM domains WHERE slug=$1;

-- name: GetInheritedAttribution :one
SELECT COALESCE((SELECT attribution FROM problem_origins WHERE problem_id=$1),'')::text;

-- name: SaveProblemOrigin :one
INSERT INTO problem_origins(problem_id,source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by,copied_at;

-- name: RecordProblemCopyAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4);


-- name: ResolveCopySourceNumber :one
SELECT id,public_id FROM problems WHERE domain_id=sqlc.arg(domain_id)::uuid AND public_id=sqlc.arg(problem_number)::bigint;
