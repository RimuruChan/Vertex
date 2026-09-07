package authoring

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
)

const originColumns = `source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by,copied_at`

func (s *PackageStore) Origin(ctx context.Context, id string) (*CopyOrigin, error) {
	if err := checkProblemRead(ctx, s.db.Pool, id); err != nil {
		return nil, err
	}
	var origin CopyOrigin
	err := s.db.Pool.QueryRowxContext(ctx, "SELECT "+originColumns+" FROM problem_origins WHERE problem_id=$1", id).Scan(
		&origin.SourceDomainID, &origin.SourceDomainSlug, &origin.SourceProblemID, &origin.SourceProblemNumber, &origin.SourceVersion, &origin.SourceTitle, &origin.SourceSHA256, &origin.Attribution, &origin.CopiedBy, &origin.CopiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &origin, nil
}

// Copy fixes source authorization and both domains before allocating an
// independent draft. No source ACLs, evaluations or live file references cross.
func (s *PackageStore) Copy(ctx context.Context, input CopyInput, artifacts ArtifactCopier) (_ *CopyResult, err error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	actor := domain.ActorID(ctx)
	var sourceDomainID string
	err = tx.GetContext(ctx, &sourceDomainID, "SELECT id FROM domains WHERE slug=$1", input.SourceDomain)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	scopes, err := domain.LockScopes(ctx, tx, actor, sourceDomainID, domain.ID(ctx))
	if err != nil {
		return nil, packageAccessError(err)
	}
	target := scopes[domain.ID(ctx)]
	if !target.Allows(domain.CreateProblem) {
		return nil, domain.ErrForbidden
	}
	sourceContext := domain.WithScope(ctx, scopes[sourceDomainID])
	var sourceID, sourceNumber string
	key := "id"
	if publicid.IsNumber(input.SourceProblem) {
		key = "public_id"
	}
	err = tx.QueryRowxContext(ctx, "SELECT id,public_id FROM problems WHERE domain_id=$1 AND "+key+"=$2", sourceDomainID, input.SourceProblem).Scan(&sourceID, &sourceNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	access, err := problem.LockAuthorization(sourceContext, tx, sourceID, actor)
	if err != nil {
		return nil, packageAccessError(err)
	}
	if !access.Permissions.Copy {
		return nil, domain.ErrForbidden
	}
	var artifact PackageUpload
	var releaseID int64
	var title string
	err = tx.QueryRowxContext(ctx, "SELECT id,title,testdata_path,sha256,case_count,checker FROM problem_versions WHERE problem_id=$1 AND version_no=$2", sourceID, input.SourceVersion).Scan(&releaseID, &title, &artifact.StoragePath, &artifact.SHA256, &artifact.CaseCount, &artifact.Checker)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var inherited string
	if err := tx.GetContext(ctx, &inherited, "SELECT COALESCE((SELECT attribution FROM problem_origins WHERE problem_id=$1),'')", sourceID); err != nil {
		return nil, err
	}
	attribution := strings.TrimSpace(inherited + "\n\n" + input.Attribution)
	if len(attribution) > 8192 {
		return nil, invalid("combined copy attribution exceeds 8192 bytes")
	}
	result := &CopyResult{DomainID: target.Domain.ID, DomainSlug: target.Domain.Slug}
	err = tx.QueryRowxContext(ctx, `INSERT INTO problems(domain_id,owner_id,author_id,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,visibility,package_revision,data_revision,built_revision)
 SELECT $1,$2,$2,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,'draft',1,1,1
 FROM problem_versions WHERE id=$3 RETURNING id,public_id`, target.Domain.ID, actor, releaseID).Scan(&result.ProblemID, &result.PublicID)
	if err != nil {
		return nil, err
	}
	queries := []string{
		`UPDATE problem_workspaces w SET tags_json=v.tags_json FROM problem_versions v WHERE w.problem_id=$1 AND v.id=$2`,
		`INSERT INTO problem_statements(problem_id,language,name,legend,input_format,output_format,notes,tutorial,scoring)
 SELECT $1,s.language,s.name,s.legend,s.input_format,s.output_format,s.notes,s.tutorial,s.scoring
 FROM problem_versions v CROSS JOIN LATERAL jsonb_to_recordset(v.statements_json) s(language text,name text,legend text,input_format text,output_format text,notes text,tutorial text,scoring text) WHERE v.id=$2`,
		`INSERT INTO problem_files(problem_id,kind,name,language,source_code,expected_verdict,is_active)
 SELECT $1,f.kind,f.name,f.language,f.source_code,f.expected_verdict,f.is_active
 FROM problem_versions v CROSS JOIN LATERAL jsonb_to_recordset(v.files_json) f(kind text,name text,language text,source_code text,expected_verdict text,is_active boolean) WHERE v.id=$2`,
		`INSERT INTO problem_tests(problem_id,test_index,group_name,source,input_data,generate_cmd,is_sample,points,description)
 SELECT $1,t."Index",t."Group",t."Source",t."InputData",t."GenerateCmd",t."IsSample",t."Points",t."Description"
 FROM problem_versions v CROSS JOIN LATERAL jsonb_to_recordset(COALESCE(NULLIF(v.package_json->'Tests','null'::jsonb),'[]'::jsonb)) t("Index" integer,"Group" text,"Source" text,"InputData" text,"GenerateCmd" text,"IsSample" boolean,"Points" integer,"Description" text) WHERE v.id=$2`,
	}
	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query, result.ProblemID, releaseID); err != nil {
			return nil, err
		}
	}
	copied, err := artifacts.Clone(ctx, sourceID, result.ProblemID, artifact)
	if err != nil {
		return nil, err
	}
	committing := false
	defer func() {
		// A failed COMMIT can have an unknown outcome. Keep files in that case;
		// deleting them could break a copy which PostgreSQL already committed.
		if err != nil && !committing && copied.created {
			err = errors.Join(err, artifacts.Remove(result.ProblemID))
		}
	}()
	if _, err := tx.ExecContext(ctx, `INSERT INTO problem_testdata(problem_id,data_version,data_revision,storage_path,sha256,case_count,checker,spj_source,config_json,samples_json)
 SELECT $1,1,1,$3,$4,$5,checker,spj_source,config_json,samples_json FROM problem_versions WHERE id=$2`, result.ProblemID, releaseID, copied.StoragePath, copied.SHA256, copied.CaseCount); err != nil {
		return nil, err
	}
	err = tx.QueryRowxContext(ctx, `INSERT INTO problem_origins(problem_id,source_domain_id,source_domain_slug,source_problem_id,source_problem_number,source_version,source_title,source_sha256,attribution,copied_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+originColumns,
		result.ProblemID, sourceDomainID, input.SourceDomain, sourceID, sourceNumber, input.SourceVersion, title, artifact.SHA256, attribution, actor).Scan(
		&result.Origin.SourceDomainID, &result.Origin.SourceDomainSlug, &result.Origin.SourceProblemID, &result.Origin.SourceProblemNumber, &result.Origin.SourceVersion, &result.Origin.SourceTitle, &result.Origin.SourceSHA256, &result.Origin.Attribution, &result.Origin.CopiedBy, &result.Origin.CopiedAt)
	if err != nil {
		return nil, err
	}
	for _, event := range []struct{ domainID, action, target string }{
		{sourceDomainID, "problem.copy.export", sourceID + ":" + result.ProblemID},
		{target.Domain.ID, "problem.copy.import", result.ProblemID},
	} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)", event.domainID, actor, event.action, event.target); err != nil {
			return nil, err
		}
	}
	committing = true
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
