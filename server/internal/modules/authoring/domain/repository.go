package domain

import (
	"context"
	"time"
)

// PackageRepository is the persistence boundary for editable package content.
type PackageRepository interface {
	Statements(ctx context.Context, problemID string) ([]Statement, error)
	SaveStatement(ctx context.Context, statement Statement) (*Statement, error)
	DeleteStatement(ctx context.Context, problemID, language string) error
	Files(ctx context.Context, problemID string, includeSource bool) ([]File, error)
	File(ctx context.Context, problemID string, id int64) (*File, error)
	SaveFile(ctx context.Context, file File) (*File, error)
	DeleteFile(ctx context.Context, problemID string, id int64) error
	Tests(ctx context.Context, problemID string, includeInput bool) ([]Test, error)
	Test(ctx context.Context, problemID string, testID int64) (*Test, error)
	CreateTest(ctx context.Context, test Test) (*Test, error)
	UpdateTest(ctx context.Context, test Test) (*Test, error)
	DeleteTest(ctx context.Context, problemID string, id int64) error
	ReorderTest(ctx context.Context, problemID string, id int64, target int) error
	Snapshot(ctx context.Context, problemID string) (*Package, error)
	Meta(ctx context.Context, problemID string) (*PackageMeta, error)
	Publish(ctx context.Context, problemID string, input PublishInput) (*Release, error)
	Releases(ctx context.Context, problemID string) ([]Release, error)
	Samples(ctx context.Context, problemID string) ([]TestOutcome, error)
	Copy(ctx context.Context, input CopyInput, artifacts ArtifactCopier) (*CopyResult, error)
	Origin(ctx context.Context, problemID string) (*CopyOrigin, error)
}

// BuildRepository is the persistence boundary for the build queue.
type BuildRepository interface {
	Enqueue(ctx context.Context, problemID, createdBy string) (*Build, error)
	Get(ctx context.Context, problemID, buildID string) (*Build, error)
	Latest(ctx context.Context, problemID string) (*Build, error)
	LatestSuccessful(ctx context.Context, problemID string) (*Build, error)
	List(ctx context.Context, problemID string, limit int) ([]Build, error)
	Cancel(ctx context.Context, problemID, buildID string) error
	Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*Build, *Package, error)
	Progress(ctx context.Context, progress Progress, leaseTTL time.Duration) error
	ResolvePackageTarget(ctx context.Context, buildID, workerID, leaseToken string) (string, error)
	RecordPackage(ctx context.Context, buildID, problemID, workerID, leaseToken string, upload PackageUpload) error
	Complete(ctx context.Context, result BuildResult, checker string) error
}

// Publisher materializes a build artifact into the shared testdata volume.
type Publisher interface {
	ArtifactCopier

	Publish(problemID string, archive []byte) (*PackageUpload, error)
}

type ArtifactCopier interface {
	Clone(ctx context.Context, sourceProblemID, targetProblemID string, source PackageUpload) (*PackageUpload, error)
	Remove(storagePath string) error
}
