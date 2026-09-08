package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/database"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

type PackageQueries struct {
	db      *database.DB
	queries *dbgen.Queries
}

func NewPackageQueries(db *database.DB) *PackageQueries {
	return &PackageQueries{db: db, queries: dbgen.New(db.Pool.DB)}
}

func (r *PackageQueries) checkRead(ctx context.Context, problemID string) error {
	access, err := problempg.LoadAccess(ctx, r.db.Pool, problemID, tenancy.ActorID(ctx))
	if err != nil {
		return packageReadError(err)
	}
	if !access.Permissions.ReadPackage {
		return tenancy.ErrForbidden
	}
	return nil
}

func packageReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, problemdomain.ErrNotFound) || errors.Is(err, tenancy.ErrNotFound) {
		return domain.ErrNotFound
	}
	return err
}

func (r *PackageQueries) Files(ctx context.Context, problemID string, includeSource bool) ([]domain.File, error) {
	if err := r.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	return packageFiles(ctx, r.queries, problemID, includeSource)
}

func packageFiles(ctx context.Context, q *dbgen.Queries, problemID string, includeSource bool) ([]domain.File, error) {
	rows, err := q.ListPackageFiles(ctx, dbgen.ListPackageFilesParams{ProblemID: problemID, IncludeSource: includeSource})
	if err != nil {
		return nil, err
	}
	result := make([]domain.File, 0, len(rows))
	for _, row := range rows {
		result = append(result, fileFromRecord(dbgen.ProblemFile(row)))
	}
	return result, nil
}

func (r *PackageQueries) File(ctx context.Context, problemID string, id int64) (*domain.File, error) {
	if err := r.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	row, err := r.queries.GetProblemFile(ctx, dbgen.GetProblemFileParams{ProblemID: problemID, ID: id, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, packageReadError(err)
	}
	item := fileFromRecord(row)
	return &item, nil
}

func (r *PackageQueries) Tests(ctx context.Context, problemID string, includeInput bool) ([]domain.Test, error) {
	if err := r.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	return packageTests(ctx, r.queries, problemID, includeInput)
}

func packageTests(ctx context.Context, q *dbgen.Queries, problemID string, includeInput bool) ([]domain.Test, error) {
	rows, err := q.ListPackageTests(ctx, dbgen.ListPackageTestsParams{ProblemID: problemID, IncludeInput: includeInput})
	if err != nil {
		return nil, err
	}
	result := make([]domain.Test, 0, len(rows))
	for _, row := range rows {
		result = append(result, testFromRecord(dbgen.ProblemTest(row)))
	}
	return result, nil
}

func (r *PackageQueries) Test(ctx context.Context, problemID string, id int64) (*domain.Test, error) {
	if err := r.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	row, err := r.queries.GetProblemTest(ctx, dbgen.GetProblemTestParams{ProblemID: problemID, ID: id})
	if err != nil {
		return nil, packageReadError(err)
	}
	item := testFromRecord(row)
	return &item, nil
}

func (r *PackageQueries) Snapshot(ctx context.Context, problemID string) (*domain.Package, error) {
	if err := r.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	return loadPackageSnapshot(ctx, r.db.Pool, problemID)
}

// loadPackageSnapshot runs on the caller's connection so enqueue can seal the
// complete package while holding its parent lock. Worker paths derive the
// parent from their fenced job instead of the request's domain.
func loadPackageSnapshot(ctx context.Context, conn dbgen.DBTX, problemID string) (*domain.Package, error) {
	q := dbgen.New(conn)
	row, err := q.GetPackageSnapshotHeader(ctx, problemID)
	if err != nil {
		return nil, packageReadError(err)
	}
	pkg := domain.Package{ProblemID: row.ID, Title: row.Title, TimeLimitMs: row.TimeLimitMs,
		MemoryLimitKB: row.MemoryLimitKb, JudgeType: row.JudgeType, Revision: row.PackageRevision,
		DomainID: row.DomainID, DataRevision: row.DataRevision}
	files, err := packageFiles(ctx, q, problemID, true)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		switch file.Kind {
		case domain.KindChecker:
			if file.IsActive {
				pkg.Checker = &file
			}
		case domain.KindValidator:
			if file.IsActive {
				pkg.Validator = &file
			}
		case domain.KindInteractor:
			if file.IsActive {
				pkg.Interactor = &file
			}
		case domain.KindGenerator:
			pkg.Generators = append(pkg.Generators, file)
		case domain.KindSolution:
			pkg.Solutions = append(pkg.Solutions, file)
		}
	}
	pkg.Tests, err = packageTests(ctx, q, problemID, true)
	if err != nil {
		return nil, err
	}
	return &pkg, nil
}

func (r *PackageQueries) Meta(ctx context.Context, problemID string) (*domain.PackageMeta, error) {
	access, err := problempg.LoadAccess(ctx, r.db.Pool, problemID, tenancy.ActorID(ctx))
	if err != nil {
		return nil, packageReadError(err)
	}
	if !access.Permissions.ReadPackage {
		return nil, tenancy.ErrForbidden
	}
	row, err := r.queries.GetPackageMetadata(ctx, dbgen.GetPackageMetadataParams{ID: problemID, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, packageReadError(err)
	}
	return &domain.PackageMeta{CanEdit: access.Permissions.Edit, CanPublish: access.Permissions.Publish,
		ProblemID: row.ID, ProblemPublicID: row.PublicID, Title: row.Title, Visibility: row.Visibility,
		JudgeType: row.JudgeType, StatementLanguage: row.StatementLanguage, TimeLimitMs: row.TimeLimitMs, MemoryLimitKB: row.MemoryLimitKb,
		PackageRevision: row.PackageRevision, BuiltRevision: row.BuiltRevision, LastBuiltAt: row.LastBuiltAt,
		TestdataCases: row.CaseCount, TestdataChecker: row.Checker, TestdataVersion: row.DataVersion, TestdataSHA256: row.Sha256,
		DataRevision: row.DataRevision, PublishedVersion: row.PublishedVersion,
		PublishedRevision: row.PublishedRevision, PublishedArtifactVersion: row.PublishedArtifactVersion}, nil
}

func fileFromRecord(row dbgen.ProblemFile) domain.File {
	return domain.File{ID: row.ID, ProblemID: row.ProblemID, Kind: row.Kind, Name: row.Name, Language: row.Language,
		SourceCode: row.SourceCode, ExpectedVerdict: row.ExpectedVerdict, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func testFromRecord(row dbgen.ProblemTest) domain.Test {
	return domain.Test{ID: row.ID, ProblemID: row.ProblemID, Index: row.TestIndex, Group: row.GroupName, Source: row.Source,
		InputData: row.InputData, GenerateCmd: row.GenerateCmd, IsSample: row.IsSample, Points: row.Points, Description: row.Description}
}
