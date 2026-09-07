package authoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

const releaseColumns = `version_no,workspace_revision,artifact_version,statement_language,sha256,case_count,created_at`

func scanRelease(row interface{ Scan(...any) error }) (Release, error) {
	var release Release
	err := row.Scan(&release.Version, &release.Revision, &release.ArtifactVersion, &release.Language, &release.SHA256, &release.CaseCount, &release.CreatedAt)
	return release, err
}

func (s *PackageStore) Releases(ctx context.Context, id string) ([]Release, error) {
	if err := checkProblemRead(ctx, s.db.Pool, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT "+releaseColumns+" FROM problem_versions WHERE problem_id=$1 ORDER BY version_no DESC LIMIT 100", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Release{}
	for rows.Next() {
		item, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PackageStore) Samples(ctx context.Context, id string) ([]TestOutcome, error) {
	if err := checkProblemRead(ctx, s.db.Pool, id); err != nil {
		return nil, err
	}
	return lastBuiltSamples(ctx, s.db.Pool, id)
}

// Publish pins the exact revision and candidate reviewed by an authorized
// publisher. A repeated request for the current release is idempotent.
func (s *PackageStore) Publish(ctx context.Context, id string, input PublishInput) (*Release, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := problem.LockAccess(ctx, tx, id, domain.ActorID(ctx))
	if err != nil {
		return nil, packageAccessError(err)
	}
	if !access.Permissions.Publish {
		return nil, domain.ErrForbidden
	}
	var revision, dataRevision, currentVersion, difficulty, timeLimit, memoryLimit int
	var title, markdown, source, judgeType, language string
	var tags []byte
	err = tx.QueryRowxContext(ctx, `SELECT p.package_revision,p.data_revision,COALESCE(p.published_version,0),w.title,w.statement_md,w.difficulty,w.source,w.time_limit_ms,w.memory_limit_kb,w.judge_type,w.statement_language,w.tags_json
	 FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id WHERE p.id=$1`, id).Scan(&revision, &dataRevision, &currentVersion, &title, &markdown, &difficulty, &source, &timeLimit, &memoryLimit, &judgeType, &language, &tags)
	if err != nil {
		return nil, err
	}
	if input.Revision != revision {
		return nil, ErrRevisionConflict
	}
	defaultLanguage := language
	if input.Language != "" {
		language = input.Language
	}
	var artifactVersion, artifactRevision, cases int
	var storagePath, hash, checker, spj string
	var config, samples []byte
	err = tx.QueryRowxContext(ctx, `SELECT data_version,data_revision,storage_path,sha256,case_count,checker,spj_source,config_json,samples_json
	 FROM problem_testdata WHERE problem_id=$1`, id).Scan(&artifactVersion, &artifactRevision, &storagePath, &hash, &cases, &checker, &spj, &config, &samples)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotPublished
	}
	if err != nil {
		return nil, err
	}
	if artifactVersion != input.ArtifactVersion || artifactRevision != dataRevision {
		return nil, ErrRevisionConflict
	}
	if storagePath == "" || hash == "" || cases <= 0 {
		return nil, ErrNotPublished
	}
	if currentVersion > 0 {
		current, err := scanRelease(tx.QueryRowxContext(ctx, "SELECT "+releaseColumns+" FROM problem_versions WHERE problem_id=$1 AND version_no=$2", id, currentVersion))
		if err != nil {
			return nil, err
		}
		if current.Revision == revision && current.ArtifactVersion == artifactVersion && current.Language == language {
			return &current, nil
		}
	}
	var outcomes []TestOutcome
	if err := json.Unmarshal(samples, &outcomes); err != nil {
		return nil, err
	}
	statement, err := statementFrom(ctx, tx, id, language)
	if err == nil {
		markdown = RenderStatement(*statement, SamplesFromOutcomes(outcomes))
		if statement.Name != "" {
			title = statement.Name
		}
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	} else if language != defaultLanguage {
		return nil, invalid("所选语言尚无已保存题面")
	}
	if strings.TrimSpace(markdown) == "" {
		return nil, invalid("请先保存完整题面，再发布版本")
	}
	var statements []byte
	if err := tx.GetContext(ctx, &statements, "SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY language),'[]'::jsonb) FROM problem_statements s WHERE problem_id=$1", id); err != nil {
		return nil, err
	}
	var files []byte
	if err := tx.GetContext(ctx, &files, "SELECT COALESCE(jsonb_agg(to_jsonb(f) ORDER BY kind,name),'[]'::jsonb) FROM problem_files f WHERE problem_id=$1", id); err != nil {
		return nil, err
	}
	pkg, err := snapshotFrom(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	packageJSON, err := json.Marshal(pkg)
	if err != nil {
		return nil, err
	}
	var version int
	if err := tx.GetContext(ctx, &version, "SELECT COALESCE(max(version_no),0)+1 FROM problem_versions WHERE problem_id=$1", id); err != nil {
		return nil, err
	}
	release, err := scanRelease(tx.QueryRowxContext(ctx, `INSERT INTO problem_versions
	 (problem_id,version_no,workspace_revision,data_revision,artifact_version,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language,tags_json,statements_json,package_json,config_json,testdata_path,sha256,case_count,checker,spj_source,created_by,files_json,samples_json)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25) RETURNING `+releaseColumns,
		id, version, revision, dataRevision, artifactVersion, title, markdown, difficulty, source, timeLimit, memoryLimit, judgeType, language, tags, statements, packageJSON, config, storagePath, hash, cases, checker, spj, access.Scope.UserID, files, samples))
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE problems SET title=$2,statement_md=$3,difficulty=$4,source=$5,time_limit_ms=$6,memory_limit_kb=$7,judge_type=$8,statement_language=$9,published_version=$10,updated_at=now() WHERE id=$1`,
		id, title, markdown, difficulty, source, timeLimit, memoryLimit, judgeType, language, version); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM problem_tags WHERE problem_id=$1", id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tags(domain_id,name) SELECT $1,value FROM jsonb_array_elements_text($2::jsonb) WHERE value<>'' ON CONFLICT(domain_id,name) DO NOTHING`, access.Scope.Domain.ID, tags); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO problem_tags(domain_id,problem_id,tag_id) SELECT $1,$2,t.id FROM tags t WHERE t.domain_id=$1 AND t.name IN (SELECT value FROM jsonb_array_elements_text($3::jsonb))`, access.Scope.Domain.ID, id, tags); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,'problem.publish',$3)", access.Scope.Domain.ID, access.Scope.UserID, id+":"+strconv.Itoa(version)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &release, nil
}
