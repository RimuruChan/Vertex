package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/database"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

func (s *PackageRepository) Releases(ctx context.Context, id string) ([]authoringdomain.Release, error) {
	if err := s.checkRead(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListProblemReleases(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]authoringdomain.Release, 0, len(rows))
	for _, row := range rows {
		items = append(items, releaseFromRecord(dbgen.GetProblemReleaseRow(row)))
	}
	return items, nil
}

func (s *PackageRepository) Samples(ctx context.Context, id string) ([]authoringdomain.TestOutcome, error) {
	if err := s.checkRead(ctx, id); err != nil {
		return nil, err
	}
	return loadCandidateSamples(ctx, s.db.Pool, id)
}

// Publish pins the exact revision and candidate reviewed by an authorized
// publisher. A repeated request for the current release is idempotent.
func (s *PackageRepository) Publish(ctx context.Context, id string, input authoringdomain.PublishInput) (*authoringdomain.Release, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := problempg.LockAccess(ctx, tx, id, tenancydomain.ActorID(ctx))
	if err != nil {
		return nil, packageReadError(err)
	}
	if !access.Permissions.Publish {
		return nil, tenancydomain.ErrForbidden
	}
	q := s.queries.WithTx(tx.Tx)
	workspace, err := q.GetPublicationWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	title, markdown, language := workspace.Title, workspace.StatementMd, workspace.StatementLanguage

	if input.Revision != workspace.PackageRevision {
		return nil, authoringdomain.ErrRevisionConflict
	}
	defaultLanguage := language
	if input.Language != "" {
		language = input.Language
	}
	artifact, err := q.GetPublicationTestdata(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, authoringdomain.ErrNotPublished
	}
	if err != nil {
		return nil, err
	}
	if artifact.DataVersion != input.ArtifactVersion || artifact.DataRevision != workspace.DataRevision {
		return nil, authoringdomain.ErrRevisionConflict
	}
	if artifact.StoragePath == "" || artifact.Sha256 == "" || artifact.CaseCount <= 0 {
		return nil, authoringdomain.ErrNotPublished
	}
	if workspace.PublishedVersion > 0 {
		row, err := q.GetProblemRelease(ctx, dbgen.GetProblemReleaseParams{ProblemID: id, VersionNo: workspace.PublishedVersion})
		if err != nil {
			return nil, err
		}
		current := releaseFromRecord(row)

		if current.Revision == workspace.PackageRevision && current.ArtifactVersion == artifact.DataVersion && current.Language == language {
			return &current, nil
		}
	}
	var outcomes []authoringdomain.TestOutcome
	if err := json.Unmarshal(artifact.SamplesJson, &outcomes); err != nil {
		return nil, err
	}
	statement, err := loadStatement(ctx, tx, id, language)
	if err == nil {
		markdown = authoringdomain.RenderStatement(*statement, authoringdomain.SamplesFromOutcomes(outcomes))
		if statement.Name != "" {
			title = statement.Name
		}
	} else if !errors.Is(err, authoringdomain.ErrNotFound) {
		return nil, err
	} else if language != defaultLanguage {
		return nil, authoringdomain.InvalidInput("所选语言尚无已保存题面")
	}
	if strings.TrimSpace(markdown) == "" {
		return nil, authoringdomain.InvalidInput("请先保存完整题面，再发布版本")
	}
	statements, err := q.SnapshotProblemStatements(ctx, id)
	if err != nil {
		return nil, err
	}
	files, err := q.SnapshotProblemFiles(ctx, id)
	if err != nil {
		return nil, err
	}
	pkg, err := loadPackageSnapshot(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	packageJSON, err := json.Marshal(pkg)
	if err != nil {
		return nil, err
	}
	version, err := q.NextProblemVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	row,
		err := q.CreateProblemRelease(ctx,
		dbgen.CreateProblemReleaseParams{ProblemID: id,
			VersionNo:         version,
			WorkspaceRevision: workspace.PackageRevision,
			DataRevision:      workspace.DataRevision,
			ArtifactVersion:   artifact.DataVersion,
			Title:             title,
			StatementMd:       markdown,
			Difficulty:        workspace.Difficulty,
			Source:            workspace.Source,
			TimeLimitMs:       workspace.TimeLimitMs,
			MemoryLimitKb:     workspace.MemoryLimitKb,
			JudgeType:         workspace.JudgeType,
			StatementLanguage: language,
			TagsJson:          json.RawMessage(workspace.TagsJson),
			StatementsJson:    json.RawMessage(statements),
			PackageJson:       json.RawMessage(packageJSON),
			ConfigJson:        json.RawMessage(artifact.ConfigJson),
			TestdataPath:      artifact.StoragePath,
			Sha256:            artifact.Sha256,
			CaseCount:         artifact.CaseCount,
			Checker:           artifact.Checker,
			SpjSource:         artifact.SpjSource,
			CreatedBy:         database.Ptr(access.Scope.UserID),
			FilesJson:         json.RawMessage(files),
			SamplesJson:       json.RawMessage(artifact.SamplesJson)})
	if err != nil {
		return nil, err
	}
	release := releaseFromRecord(dbgen.GetProblemReleaseRow(row))
	if err := q.PublishProblem(ctx,
		dbgen.PublishProblemParams{ID: id,
			Title:             title,
			StatementMd:       markdown,
			Difficulty:        workspace.Difficulty,
			Source:            workspace.Source,
			TimeLimitMs:       workspace.TimeLimitMs,
			MemoryLimitKb:     workspace.MemoryLimitKb,
			JudgeType:         workspace.JudgeType,
			StatementLanguage: language,
			PublishedVersion: sql.NullInt32{Int32: int32(version),
				Valid: true}}); err != nil {
		return nil, err
	}
	if err := q.DeletePublishedProblemTags(ctx, id); err != nil {
		return nil, err
	}
	if err := q.EnsurePublishedTags(ctx, dbgen.EnsurePublishedTagsParams{DomainID: access.Scope.Domain.ID, TagsJson: json.RawMessage(workspace.TagsJson)}); err != nil {
		return nil, err
	}
	if err := q.InsertPublishedProblemTags(ctx, dbgen.InsertPublishedProblemTagsParams{DomainID: access.Scope.Domain.ID, ProblemID: id, TagsJson: json.RawMessage(workspace.TagsJson)}); err != nil {
		return nil, err
	}
	if err := q.RecordProblemPublication(ctx, dbgen.RecordProblemPublicationParams{DomainID: access.Scope.Domain.ID, ActorID: database.Ptr(access.Scope.UserID), Target: id + ":" + strconv.Itoa(version)}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &release, nil
}

func releaseFromRecord(row dbgen.GetProblemReleaseRow) authoringdomain.Release {
	return authoringdomain.Release{Version: row.VersionNo, Revision: row.WorkspaceRevision, ArtifactVersion: row.ArtifactVersion,
		Language: row.StatementLanguage, SHA256: row.Sha256, CaseCount: row.CaseCount, CreatedAt: row.CreatedAt}
}
