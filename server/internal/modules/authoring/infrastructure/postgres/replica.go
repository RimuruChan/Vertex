package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
)

func (repo *RevisionRepository) CopyRelease(ctx context.Context, input domain.CopyInput) (*domain.CopyResult, error) {
	input, err := domain.NormalizeCopy(input)
	if err != nil {
		return nil, err
	}
	number, err := strconv.ParseInt(input.SourceProblem, 10, 64)
	if err != nil || number < 1 {
		return nil, domain.InvalidInput("source problem number required")
	}
	copier, ok := repo.artifacts.(domain.CheckedArtifactCopier)
	if !ok {
		return nil, domain.InvalidInput("checked artifact copy storage is not configured")
	}
	tx, err := repo.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	actor := tenancy.ActorID(ctx)
	q := dbgen.New(tx)
	sourceDomainID, err := q.GetCopySourceDomain(ctx, input.SourceDomain)
	if err != nil {
		return nil, packageReadError(err)
	}
	scopes, err := tenancypg.LockScopes(ctx, tx, actor, sourceDomainID, tenancy.ID(ctx))
	if err != nil {
		return nil, packageReadError(err)
	}
	target := scopes[tenancy.ID(ctx)]
	if !target.Allows(tenancy.CreateProblem) {
		return nil, tenancy.ErrForbidden
	}
	source, err := q.ResolveCopySourceNumber(ctx, dbgen.ResolveCopySourceNumberParams{DomainID: sourceDomainID, ProblemNumber: number})
	if err != nil {
		return nil, packageReadError(err)
	}
	sourceCtx := tenancy.WithScope(ctx, scopes[sourceDomainID])
	access, err := problempg.LockAuthorization(sourceCtx, tx, source.ID, actor)
	if err != nil {
		return nil, packageReadError(err)
	}
	if !access.Permissions.Copy {
		return nil, tenancy.ErrForbidden
	}
	release, err := q.ReadCopyRelease(ctx, dbgen.ReadCopyReleaseParams{ProblemID: source.ID, VersionNo: input.SourceVersion})
	if err != nil {
		return nil, packageReadError(err)
	}
	tree, err := loadTree(ctx, q, source.ID, release.SourceTreeHash.String)
	if err != nil {
		return nil, err
	}
	inspection, snapshot, err := repo.inspectTree(ctx, source.ID, tree)
	if err != nil {
		return nil, err
	}
	if !inspection.CanBuild || snapshot.DataHash != release.DataHash || snapshot.PolicyVersion != release.CheckPolicy {
		return nil, domain.ErrNotBuildable
	}
	var manifest domain.CheckArtifact
	if err := json.Unmarshal(release.ConfigJson, &manifest); err != nil {
		return nil, err
	}
	if manifest.Snapshot.DataHash != snapshot.DataHash || manifest.ToolchainKey != release.ToolchainKey {
		return nil, domain.ErrNotPublished
	}
	inherited, err := q.GetInheritedAttribution(ctx, source.ID)
	if err != nil {
		return nil, err
	}
	attribution := strings.TrimSpace(inherited + "\n\n" + input.Attribution)
	if len(attribution) > 8192 {
		return nil, domain.InvalidInput("combined copy attribution exceeds 8192 bytes")
	}
	created, err := q.CreateRevisionCopy(ctx, dbgen.CreateRevisionCopyParams{DomainID: target.Domain.ID, OwnerID: actor, ID: release.ID})
	if err != nil {
		return nil, err
	}
	if err := tenancypg.LockContentStorage(ctx, tx, created.ID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, entry := range tree.Entries {
		if seen[entry.Blob.SHA256] {
			continue
		}
		seen[entry.Blob.SHA256] = true
		reader, err := repo.blobs.Open(ctx, source.ID, entry.Blob)
		if err != nil {
			return nil, err
		}
		ref, err := repo.blobs.Put(ctx, created.ID, reader)
		reader.Close()
		if err != nil {
			return nil, err
		}
		if ref != entry.Blob {
			return nil, domain.InvalidInput("copied source blob does not match release")
		}
		if err := registerBlob(ctx, q, created.ID, actor, ref); err != nil {
			return nil, err
		}
	}
	treeHash, err := repo.persistTree(ctx, q, created.ID, actor, tree)
	if err != nil {
		return nil, err
	}
	if err := q.InitializeAuthoringHead(ctx, dbgen.InitializeAuthoringHeadParams{ProblemID: created.ID, TreeHash: treeHash}); err != nil {
		return nil, err
	}
	if err := q.StartWorkingCopy(ctx, dbgen.StartWorkingCopyParams{ProblemID: created.ID, ActorID: actor}); err != nil {
		return nil, err
	}
	upload, err := copier.CloneChecked(ctx, source.ID, created.ID, domain.PackageUpload{Artifact: &manifest, StoragePath: release.TestdataPath, SHA256: release.Sha256, CaseCount: release.CaseCount, Checker: "artifact"}, *snapshot)
	if err != nil {
		return nil, err
	}
	// Keep unreachable files on transaction failure, including ambiguous COMMIT.
	// Reference-aware collection is safer than deleting potentially committed data.
	pkg := domain.CheckInput{DomainID: target.Domain.ID, ProblemID: created.ID, Check: snapshot}
	inputJSON, err := json.Marshal(pkg)
	if err != nil {
		return nil, err
	}
	manifestJSON, err := json.Marshal(upload.Artifact)
	if err != nil {
		return nil, err
	}
	results := []domain.TestOutcome{}
	for index, test := range snapshot.Tests {
		data := upload.Artifact.Tests[index]
		results = append(results, domain.TestOutcome{Index: index + 1, IsSample: test.Definition.IsSample, Group: test.Definition.Group, Points: test.Definition.Points, Source: "copied", Status: "ok", InputBytes: data.Input.Bytes, AnswerBytes: data.Answer.Bytes})
	}
	testsJSON, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}
	validation := []domain.ValidationOutcome{}
	for _, item := range snapshot.Validation {
		actual := "rejected"
		if item.Definition.Mode == "valid_output" {
			actual = "accepted"
		}
		validation = append(validation, domain.ValidationOutcome{ID: item.ID, Name: item.Definition.Name, Mode: item.Definition.Mode, Actual: actual, Status: "ok", Message: "复用已发布结果，未重新执行"})
	}
	validationJSON, err := json.Marshal(validation)
	if err != nil {
		return nil, err
	}
	if _, err := q.CreateCopiedCheck(ctx, dbgen.CreateCopiedCheckParams{ProblemID: created.ID, InputJson: inputJSON, TreeHash: treeHash, DataHash: snapshot.DataHash, CheckPolicy: snapshot.PolicyVersion, ToolchainKey: manifest.ToolchainKey, CaseCount: upload.CaseCount, Log: "复用来源已发布版本的验证产物；未重新执行程序。", TestsJson: testsJSON, ValidationJson: validationJSON, PackagePath: upload.StoragePath, PackageSha256: upload.SHA256, PackageManifest: manifestJSON, ActorID: &actor}); err != nil {
		return nil, err
	}
	origin, err := q.SaveProblemOrigin(ctx, dbgen.SaveProblemOriginParams{ProblemID: created.ID, SourceDomainID: sourceDomainID, SourceDomainSlug: input.SourceDomain, SourceProblemID: source.ID, SourceProblemNumber: source.PublicID, SourceVersion: input.SourceVersion, SourceTitle: release.Title, SourceSha256: release.Sha256, Attribution: attribution, CopiedBy: &actor})
	if err != nil {
		return nil, err
	}
	for _, event := range []struct{ domain, action string }{{sourceDomainID, "problem.copy.export"}, {target.Domain.ID, "problem.copy.import"}} {
		if err := q.RecordProblemCopyAudit(ctx, dbgen.RecordProblemCopyAuditParams{DomainID: event.domain, ActorID: &actor, Action: event.action, Target: source.ID + ":" + created.ID}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &domain.CopyResult{ProblemID: created.ID, PublicID: created.PublicID, DomainID: target.Domain.ID, DomainSlug: target.Domain.Slug, Origin: originFromRecord(dbgen.GetProblemOriginRow(origin))}, nil
}

func (repo *RevisionRepository) Origin(ctx context.Context, id string) (*domain.CopyOrigin, error) {
	var result *domain.CopyOrigin
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, _ string) error {
		row, err := q.GetProblemOrigin(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		origin := originFromRecord(row)
		result = &origin
		return nil
	})
	return result, err
}

func originFromRecord(row dbgen.GetProblemOriginRow) domain.CopyOrigin {
	return domain.CopyOrigin{SourceDomainID: row.SourceDomainID, SourceDomainSlug: row.SourceDomainSlug,
		SourceProblemID: row.SourceProblemID, SourceProblemNumber: row.SourceProblemNumber, SourceVersion: row.SourceVersion,
		SourceTitle: row.SourceTitle, SourceSHA256: row.SourceSha256, Attribution: row.Attribution, CopiedBy: row.CopiedBy, CopiedAt: row.CopiedAt}
}
