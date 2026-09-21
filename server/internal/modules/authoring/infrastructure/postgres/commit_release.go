package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (repo *RevisionRepository) PublishCommit(ctx context.Context, id string, input domain.CommitPublication) (*domain.CommitRelease, error) {
	if input.Revision <= 0 || input.ExpectedVersion < 0 || input.CheckID == "" {
		return nil, domain.InvalidInput("a commit, successful check and expected current release are required")
	}
	var result *domain.CommitRelease
	err := repo.transactionFor(ctx, id, publishWorkbench, func(q *dbgen.Queries, actor string) error {
		commit, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: input.Revision})
		if err != nil {
			return err
		}
		tree, err := loadTree(ctx, q, id, commit.TreeHash)
		if err != nil {
			return err
		}
		report, snapshot, err := repo.inspectTree(ctx, id, tree)
		if err != nil {
			return err
		}
		if !report.CanBuild {
			return domain.ErrNotBuildable
		}
		if len(snapshot.Metadata.Requirements) > 0 {
			return domain.InvalidInput("resolve imported compatibility requirements before publication")
		}
		if len(report.PublicationIssues) > 0 {
			return domain.InvalidInput(report.PublicationIssues[0].Message)
		}
		language := input.Language
		if language == "" {
			language = snapshot.Metadata.StatementLanguage
		}
		current, err := q.CurrentReleasedVersion(ctx, id)
		if err != nil {
			return err
		}
		if current > 0 {
			prior, err := q.ReadCommittedRelease(ctx, dbgen.ReadCommittedReleaseParams{ProblemID: id, VersionNo: current})
			if err == nil && prior.SourceRevision.Int64 == input.Revision && prior.CheckID != nil && *prior.CheckID == input.CheckID && prior.StatementLanguage == language {
				result = committedRelease(prior)
				return nil
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if current != input.ExpectedVersion {
			return domain.ErrRevisionConflict
		}
		check, err := readCheck(ctx, q, id, actor, input.CheckID)
		if err != nil {
			return err
		}
		if check.State != domain.BuildSucceeded || check.DataHash != snapshot.DataHash || check.PolicyVersion != domain.CheckPolicyVersion || !toolchainKeyPattern.MatchString(check.ToolchainKey) || !validationResultsMatch(snapshot.Validation, check.Validation) {
			return domain.ErrNotPublished
		}
		artifact, err := q.PublicationCheckArtifact(ctx, dbgen.PublicationCheckArtifactParams{ProblemID: id, ID: input.CheckID})
		if err != nil {
			return err
		}
		var manifest domain.CheckArtifact
		if err := json.Unmarshal(artifact.PackageManifest, &manifest); err != nil {
			return err
		}
		if manifest.ToolchainKey != check.ToolchainKey || manifest.Snapshot.DataHash != snapshot.DataHash || len(manifest.Tests) != len(snapshot.Tests) || artifact.PackageCases != len(snapshot.Tests) {
			return domain.ErrNotPublished
		}
		if repo.artifacts == nil {
			return domain.InvalidInput("publication artifact storage is not configured")
		}
		presentation, err := repo.presentation(ctx, q, id, actor, language, artifact.PackagePath, tree, snapshot, manifest)
		if err != nil {
			return err
		}
		markdown := presentation.markdown
		tags, err := json.Marshal(snapshot.Metadata.Tags)
		if err != nil {
			return err
		}
		version, err := q.NextProblemVersion(ctx, id)
		if err != nil {
			return err
		}
		meta := snapshot.Metadata
		row, err := q.CreateCommittedRelease(ctx, dbgen.CreateCommittedReleaseParams{ProblemID: id, VersionNo: version, SourceRevision: sql.NullInt64{Int64: input.Revision, Valid: true}, TreeHash: sql.NullString{String: commit.TreeHash, Valid: true}, CheckID: &input.CheckID, ToolchainKey: check.ToolchainKey, Title: meta.Title, StatementMd: markdown, Difficulty: meta.Difficulty, Source: meta.Source, TimeLimitMs: meta.TimeLimitMs, MemoryLimitKb: meta.MemoryLimitKB, JudgeType: meta.JudgeType, StatementLanguage: language, TagsJson: tags, ConfigJson: artifact.PackageManifest, TestdataPath: artifact.PackagePath, Sha256: artifact.PackageSha256, CaseCount: artifact.PackageCases, ActorID: &actor})
		if err != nil {
			return err
		}
		for _, file := range presentation.files {
			file.VersionNo = version
			if err := q.CreatePublishedFile(ctx, file); err != nil {
				return err
			}
		}
		if err := q.PublishProblem(ctx, dbgen.PublishProblemParams{ID: id, Title: meta.Title, StatementMd: markdown, Difficulty: meta.Difficulty, Source: meta.Source, TimeLimitMs: meta.TimeLimitMs, MemoryLimitKb: meta.MemoryLimitKB, JudgeType: meta.JudgeType, StatementLanguage: language, PublishedVersion: sql.NullInt32{Int32: int32(version), Valid: true}}); err != nil {
			return err
		}
		if err := q.DeletePublishedProblemTags(ctx, id); err != nil {
			return err
		}
		if err := q.EnsurePublishedTags(ctx, dbgen.EnsurePublishedTagsParams{DomainID: tenancy.ID(ctx), TagsJson: tags}); err != nil {
			return err
		}
		if err := q.InsertPublishedProblemTags(ctx, dbgen.InsertPublishedProblemTagsParams{DomainID: tenancy.ID(ctx), ProblemID: id, TagsJson: tags}); err != nil {
			return err
		}
		if err := q.RecordProblemPublication(ctx, dbgen.RecordProblemPublicationParams{DomainID: tenancy.ID(ctx), ActorID: &actor, Target: id + ":" + strconv.Itoa(version)}); err != nil {
			return err
		}
		result = committedRelease(dbgen.ReadCommittedReleaseRow(row))
		return nil
	})
	return result, err
}

func (repo *RevisionRepository) CommitReleases(ctx context.Context, id string) ([]domain.CommitRelease, error) {
	result := []domain.CommitRelease{}
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, _ string) error {
		rows, err := q.ListCommittedReleases(ctx, id)
		if err != nil {
			return err
		}
		for _, row := range rows {
			result = append(result, *committedRelease(dbgen.ReadCommittedReleaseRow(row)))
		}
		return nil
	})
	return result, err
}
func committedRelease(row dbgen.ReadCommittedReleaseRow) *domain.CommitRelease {
	return &domain.CommitRelease{Version: row.VersionNo, Revision: row.SourceRevision.Int64, TreeHash: row.SourceTreeHash.String, CheckID: *row.CheckID, ToolchainKey: row.ToolchainKey, Language: row.StatementLanguage, CreatedAt: row.CreatedAt}
}
