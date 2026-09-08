package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	publiciddomain "github.com/RimuruChan/Vertex/server/internal/modules/publicid/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

func (s *PackageRepository) Origin(ctx context.Context, id string) (*authoringdomain.CopyOrigin, error) {
	if err := s.checkRead(ctx, id); err != nil {
		return nil, err
	}
	row, err := s.queries.GetProblemOrigin(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	origin := originFromRecord(row)
	return &origin, nil
}

// Copy fixes source authorization and both domains before allocating an
// independent draft. No source ACLs, evaluations or live file references cross.
func (s *PackageRepository) Copy(ctx context.Context, input authoringdomain.CopyInput, artifacts authoringdomain.ArtifactCopier) (_ *authoringdomain.CopyResult, err error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	actor := tenancydomain.ActorID(ctx)
	q := s.queries.WithTx(tx.Tx)
	sourceDomainID, err := q.GetCopySourceDomain(ctx, input.SourceDomain)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, authoringdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	scopes, err := tenancypg.LockScopes(ctx, tx, actor, sourceDomainID, tenancydomain.ID(ctx))
	if err != nil {
		return nil, packageReadError(err)
	}
	target := scopes[tenancydomain.ID(ctx)]
	if !target.Allows(tenancydomain.CreateProblem) {
		return nil, tenancydomain.ErrForbidden
	}
	sourceContext := tenancydomain.WithScope(ctx, scopes[sourceDomainID])
	var source dbgen.ResolveCopySourceIDRow
	if publiciddomain.IsNumber(input.SourceProblem) {
		number, parseErr := strconv.ParseInt(input.SourceProblem, 10, 64)
		if parseErr != nil {
			return nil, parseErr
		}
		row, queryErr := q.ResolveCopySourceNumber(ctx, dbgen.ResolveCopySourceNumberParams{DomainID: sourceDomainID, ProblemNumber: number})
		source, err = dbgen.ResolveCopySourceIDRow(row), queryErr
	} else {
		source, err = q.ResolveCopySourceID(ctx, dbgen.ResolveCopySourceIDParams{DomainID: sourceDomainID, ProblemID: input.SourceProblem})
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, authoringdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	access, err := problempg.LockAuthorization(sourceContext, tx, source.ID, actor)
	if err != nil {
		return nil, packageReadError(err)
	}
	if !access.Permissions.Copy {
		return nil, tenancydomain.ErrForbidden
	}
	release, err := q.GetCopySourceVersion(ctx, dbgen.GetCopySourceVersionParams{ProblemID: source.ID, VersionNo: input.SourceVersion})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, authoringdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	artifact := authoringdomain.PackageUpload{StoragePath: release.TestdataPath, SHA256: release.Sha256, CaseCount: release.CaseCount, Checker: release.Checker}
	inherited, err := q.GetInheritedAttribution(ctx, source.ID)
	if err != nil {
		return nil, err
	}

	attribution := strings.TrimSpace(inherited + "\n\n" + input.Attribution)
	if len(attribution) > 8192 {
		return nil, authoringdomain.InvalidInput("combined copy attribution exceeds 8192 bytes")
	}
	result := &authoringdomain.CopyResult{DomainID: target.Domain.ID, DomainSlug: target.Domain.Slug}
	created, err := q.CreateCopiedProblem(ctx, dbgen.CreateCopiedProblemParams{DomainID: target.Domain.ID, OwnerID: actor, ID: release.ID})
	if err != nil {
		return nil, err
	}
	result.ProblemID, result.PublicID = created.ID, created.PublicID
	if err := q.CopyWorkspaceTags(ctx, dbgen.CopyWorkspaceTagsParams{ProblemID: result.ProblemID, VersionID: release.ID}); err != nil {
		return nil, err
	}
	if err := q.CopyVersionStatements(ctx, dbgen.CopyVersionStatementsParams{ProblemID: result.ProblemID, VersionID: release.ID}); err != nil {
		return nil, err
	}
	if err := q.CopyVersionFiles(ctx, dbgen.CopyVersionFilesParams{ProblemID: result.ProblemID, VersionID: release.ID}); err != nil {
		return nil, err
	}
	if err := q.CopyVersionTests(ctx, dbgen.CopyVersionTestsParams{ProblemID: result.ProblemID, VersionID: release.ID}); err != nil {
		return nil, err
	}

	copied, err := artifacts.Clone(ctx, source.ID, result.ProblemID, artifact)
	if err != nil {
		return nil, err
	}
	committing := false
	defer func() {
		// A failed COMMIT can have an unknown outcome. Keep files in that case;
		// deleting them could break a copy which PostgreSQL already committed.
		if err != nil && !committing && copied.Created {
			err = errors.Join(err, artifacts.Remove(result.ProblemID))
		}
	}()
	if err := q.CopyVersionTestdata(ctx, dbgen.CopyVersionTestdataParams{ProblemID: result.ProblemID, ID: release.ID, StoragePath: copied.StoragePath, Sha256: copied.SHA256, CaseCount: copied.CaseCount}); err != nil {
		return nil, err
	}
	origin,
		err := q.SaveProblemOrigin(ctx,
		dbgen.SaveProblemOriginParams{ProblemID: result.ProblemID,
			SourceDomainID:      sourceDomainID,
			SourceDomainSlug:    input.SourceDomain,
			SourceProblemID:     source.ID,
			SourceProblemNumber: source.PublicID,
			SourceVersion:       input.SourceVersion,
			SourceTitle:         release.Title,
			SourceSha256:        artifact.SHA256,
			Attribution:         attribution,
			CopiedBy:            database.Ptr(actor)})
	if err != nil {
		return nil, err
	}
	result.Origin = originFromRecord(dbgen.GetProblemOriginRow(origin))
	for _, event := range []struct{ domainID, action, target string }{
		{sourceDomainID, "problem.copy.export", source.ID + ":" + result.ProblemID},
		{target.Domain.ID, "problem.copy.import", result.ProblemID},
	} {
		if err := q.RecordProblemCopyAudit(ctx, dbgen.RecordProblemCopyAuditParams{DomainID: event.domainID, ActorID: database.Ptr(actor), Action: event.action, Target: event.target}); err != nil {
			return nil, err
		}
	}
	committing = true
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func originFromRecord(row dbgen.GetProblemOriginRow) authoringdomain.CopyOrigin {
	return authoringdomain.CopyOrigin{SourceDomainID: row.SourceDomainID, SourceDomainSlug: row.SourceDomainSlug,
		SourceProblemID: row.SourceProblemID, SourceProblemNumber: row.SourceProblemNumber, SourceVersion: row.SourceVersion,
		SourceTitle: row.SourceTitle, SourceSHA256: row.SourceSha256, Attribution: row.Attribution, CopiedBy: row.CopiedBy, CopiedAt: row.CopiedAt}
}
