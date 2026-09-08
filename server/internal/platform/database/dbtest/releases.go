package dbtest

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// PublishedProblems makes explicit, immutable fixture releases for tests of
// other domains. Publication tests must use the real authoring workflow.
// It never updates an existing release or changes resource access visibility.
func PublishedProblems(ctx context.Context, db *database.DB, ids ...string) error {
	if ids == nil {
		ids = []string{}
	}
	tx, err := db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO problem_versions(problem_id,version_no,workspace_revision,data_revision,artifact_version,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,tags_json,testdata_path,sha256,case_count,checker,spj_source,config_json,created_by)
	 SELECT p.id,1,p.package_revision,p.data_revision,COALESCE(td.data_version,1),w.title,COALESCE(NULLIF(w.statement_md,''),'Fixture statement'),w.difficulty,w.source,w.time_limit_ms,w.memory_limit_kb,w.judge_type,w.statement_language,
	 CASE WHEN w.tags_json<>'[]'::jsonb THEN w.tags_json ELSE COALESCE((SELECT jsonb_agg(t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb) END,
	 COALESCE(NULLIF(td.storage_path,''),p.id::text||'/fixture'),COALESCE(NULLIF(td.sha256,''),'fixture'),GREATEST(COALESCE(td.case_count,1),1),COALESCE(td.checker,'diff'),COALESCE(td.spj_source,''),COALESCE(td.config_json,'{}'::jsonb),p.owner_id
	 FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id LEFT JOIN problem_testdata td ON td.problem_id=p.id
	 WHERE p.published_version IS NULL AND (cardinality($1::uuid[])=0 OR p.id=ANY($1::uuid[])) ON CONFLICT(problem_id,version_no) DO NOTHING`, ids)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE problems p SET published_version=1,title=v.title,statement_md=v.statement_md,difficulty=v.difficulty,source=v.source,time_limit_ms=v.time_limit_ms,memory_limit_kb=v.memory_limit_kb
	 FROM problem_versions v WHERE v.problem_id=p.id AND v.version_no=1 AND p.published_version IS NULL AND (cardinality($1::uuid[])=0 OR p.id=ANY($1::uuid[]))`, ids)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tags(domain_id,name) SELECT DISTINCT p.domain_id,t.value FROM problems p JOIN problem_versions v ON v.problem_id=p.id AND v.version_no=1 CROSS JOIN LATERAL jsonb_array_elements_text(v.tags_json) t WHERE (cardinality($1::uuid[])=0 OR p.id=ANY($1::uuid[])) ON CONFLICT(domain_id,name) DO NOTHING`, ids); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO problem_tags(domain_id,problem_id,tag_id) SELECT p.domain_id,p.id,t.id FROM problems p JOIN problem_versions v ON v.problem_id=p.id AND v.version_no=1 JOIN tags t ON t.domain_id=p.domain_id AND v.tags_json @> jsonb_build_array(t.name) WHERE (cardinality($1::uuid[])=0 OR p.id=ANY($1::uuid[])) ON CONFLICT DO NOTHING`, ids); err != nil {
		return err
	}
	return tx.Commit()
}
