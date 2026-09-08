package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Service holds the authoring rules: what a valid package looks like, when it
// may be built, and how a worker's fenced reports are accepted.
type Service struct {
	packages   authoringdomain.PackageRepository
	builds     authoringdomain.BuildRepository
	publisher  authoringdomain.Publisher
	dispatcher *Dispatcher
	leaseTTL   time.Duration
	maxWait    time.Duration
}

func NewService(
	packages authoringdomain.PackageRepository, builds authoringdomain.BuildRepository, publisher authoringdomain.Publisher,
	dispatcher *Dispatcher, leaseTTL, maxWait time.Duration,
) (*Service, error) {
	if packages == nil || builds == nil || publisher == nil || dispatcher == nil {
		return nil, errors.New("authoring dependencies must not be nil")
	}
	if leaseTTL <= 0 || maxWait <= 0 || leaseTTL <= maxWait {
		return nil, errors.New("build lease TTL must be greater than the positive long-poll timeout")
	}
	return &Service{
		packages: packages, builds: builds, publisher: publisher,
		dispatcher: dispatcher, leaseTTL: leaseTTL, maxWait: maxWait,
	}, nil
}

// ---------- statements ----------

func (s *Service) Statements(ctx context.Context, problemID string) ([]authoringdomain.Statement, error) {
	return s.packages.Statements(ctx, problemID)
}

func (s *Service) SaveStatement(ctx context.Context, statement authoringdomain.Statement) (*authoringdomain.Statement, error) {
	normalized, err := authoringdomain.NormalizeStatement(statement)
	if err != nil {
		return nil, err
	}
	return s.packages.SaveStatement(ctx, normalized)
}

func (s *Service) DeleteStatement(ctx context.Context, problemID, language string) error {
	return s.packages.DeleteStatement(ctx, problemID, strings.ToLower(strings.TrimSpace(language)))
}

// PreviewStatement renders the public Markdown for a statement without saving
// it, using the samples produced by the last successful build.
func (s *Service) PreviewStatement(_ context.Context, statement authoringdomain.Statement, samples []authoringdomain.Sample) string {
	return authoringdomain.RenderStatement(statement, samples)
}

// CandidateSamples supplies preview examples without changing the public statement.
func (s *Service) CandidateSamples(ctx context.Context, problemID string) ([]authoringdomain.Sample, error) {
	tests, err := s.packages.Samples(ctx, problemID)
	if err != nil {
		return nil, err
	}
	return authoringdomain.SamplesFromOutcomes(tests), nil
}

// ---------- files ----------

func (s *Service) Files(ctx context.Context, problemID string) ([]authoringdomain.File, error) {
	return s.packages.Files(ctx, problemID, false)
}

func (s *Service) File(ctx context.Context, problemID string, id int64) (*authoringdomain.File, error) {
	return s.packages.File(ctx, problemID, id)
}

func (s *Service) SaveFile(ctx context.Context, file authoringdomain.File) (*authoringdomain.File, error) {
	normalized, err := authoringdomain.NormalizeFile(file)
	if err != nil {
		return nil, err
	}
	return s.packages.SaveFile(ctx, normalized)
}

func (s *Service) DeleteFile(ctx context.Context, problemID string, id int64) error {
	return s.packages.DeleteFile(ctx, problemID, id)
}

// ---------- tests ----------

func (s *Service) Tests(ctx context.Context, problemID string) ([]authoringdomain.Test, error) {
	return s.packages.Tests(ctx, problemID, false)
}

func (s *Service) Test(ctx context.Context, problemID string, id int64) (*authoringdomain.Test, error) {
	if id <= 0 {
		return nil, authoringdomain.InvalidInput("test ID must be positive")
	}
	return s.packages.Test(ctx, problemID, id)
}

func (s *Service) CreateTest(ctx context.Context, test authoringdomain.Test) (*authoringdomain.Test, error) {
	prepared, err := authoringdomain.NormalizeTest(test)
	if err != nil {
		return nil, err
	}
	return s.packages.CreateTest(ctx, prepared)
}

func (s *Service) UpdateTest(ctx context.Context, test authoringdomain.Test) (*authoringdomain.Test, error) {
	prepared, err := authoringdomain.NormalizeTest(test)
	if err != nil {
		return nil, err
	}
	return s.packages.UpdateTest(ctx, prepared)
}

func (s *Service) DeleteTest(ctx context.Context, problemID string, id int64) error {
	return s.packages.DeleteTest(ctx, problemID, id)
}

func (s *Service) ReorderTest(ctx context.Context, problemID string, id int64, target int) error {
	return s.packages.ReorderTest(ctx, problemID, id, target)
}

// ---------- builds ----------

// Workspace assembles the authoring view. Sources and test inputs are omitted;
// the editor fetches those one at a time.
func (s *Service) Workspace(ctx context.Context, problemID string) (*authoringdomain.Workspace, error) {
	meta, err := s.packages.Meta(ctx, problemID)
	if err != nil {
		return nil, err
	}
	statements, err := s.packages.Statements(ctx, problemID)
	if err != nil {
		return nil, err
	}
	files, err := s.packages.Files(ctx, problemID, false)
	if err != nil {
		return nil, err
	}
	tests, err := s.packages.Tests(ctx, problemID, false)
	if err != nil {
		return nil, err
	}
	latest, err := s.builds.Latest(ctx, problemID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.packages.Snapshot(ctx, problemID)
	if err != nil {
		return nil, err
	}
	return &authoringdomain.Workspace{
		Meta: *meta, Statements: statements, Files: files, Tests: tests,
		LatestBuild: latest, Issues: authoringdomain.Validate(snapshot),
	}, nil
}

// Build validates the package and queues one build. A package that is already
// building returns the running build with ErrBuildRunning so the caller can
// show it instead of creating a duplicate.
func (s *Service) Build(ctx context.Context, problemID, requestedBy string) (*authoringdomain.Build, error) {
	pkg, err := s.packages.Snapshot(ctx, problemID)
	if err != nil {
		return nil, err
	}
	if issues := authoringdomain.Validate(pkg); len(issues) > 0 {
		return nil, fmt.Errorf("%w: %s", authoringdomain.ErrNotBuildable, strings.Join(issues, "; "))
	}
	build, err := s.builds.Enqueue(ctx, problemID, requestedBy)
	if err != nil {
		return build, err
	}
	s.dispatcher.Notify()
	return build, nil
}

func (s *Service) BuildStatus(ctx context.Context, problemID, buildID string) (*authoringdomain.Build, error) {
	return s.builds.Get(ctx, problemID, buildID)
}

func (s *Service) LatestBuild(ctx context.Context, problemID string) (*authoringdomain.Build, error) {
	return s.builds.Latest(ctx, problemID)
}

func (s *Service) Builds(ctx context.Context, problemID string, limit int) ([]authoringdomain.Build, error) {
	return s.builds.List(ctx, problemID, limit)
}

func (s *Service) CancelBuild(ctx context.Context, problemID, buildID string) error {
	return s.builds.Cancel(ctx, problemID, buildID)
}

// Claim long-polls for one leased build job, mirroring the judge protocol so
// workers can run both loops with the same failure handling.
func (s *Service) Claim(ctx context.Context, workerID string, wait time.Duration) (*authoringdomain.Build, *authoringdomain.Package, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || len(workerID) > 128 {
		return nil, nil, authoringdomain.InvalidInput("worker ID must contain 1-128 characters")
	}
	if wait <= 0 || wait > s.maxWait {
		wait = s.maxWait
	}
	deadline := time.Now().Add(wait)
	for {
		build, pkg, err := s.builds.Claim(ctx, workerID, s.leaseTTL)
		if err != nil {
			return nil, nil, err
		}
		if build != nil {
			s.dispatcher.Notify()
			return build, pkg, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, nil, ctx.Err()
		case <-s.dispatcher.wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (s *Service) Progress(ctx context.Context, progress authoringdomain.Progress) error {
	if progress.BuildID == "" || progress.WorkerID == "" || progress.LeaseToken == "" {
		return authoringdomain.ErrStaleLease
	}
	if progress.Done < 0 {
		progress.Done = 0
	}
	if progress.Total < 0 {
		progress.Total = 0
	}
	if len(progress.Log) > 64<<10 {
		progress.Log = progress.Log[:64<<10]
	}
	return s.builds.Progress(ctx, progress, s.leaseTTL)
}

// UploadPackage materializes an artifact for a leased build. Publication is
// deferred to Complete so a worker that dies mid-report can never leave the
// problem pointing at data whose build never finished.
func (s *Service) UploadPackage(
	ctx context.Context, buildID, requestedProblemID, workerID, leaseToken string, archive []byte,
) (*authoringdomain.PackageUpload, error) {
	if buildID == "" || workerID == "" || leaseToken == "" {
		return nil, authoringdomain.ErrStaleLease
	}
	problemID, err := s.builds.ResolvePackageTarget(ctx, buildID, workerID, leaseToken)
	if err != nil {
		return nil, err
	}
	if requestedProblemID != problemID {
		return nil, authoringdomain.ErrPackageTarget
	}
	upload, err := s.publisher.Publish(problemID, archive)
	if err != nil {
		return nil, err
	}
	if err := s.builds.RecordPackage(ctx, buildID, problemID, workerID, leaseToken, *upload); err != nil {
		if upload.Created {
			if cleanupErr := s.publisher.Remove(upload.StoragePath); cleanupErr != nil {
				return nil, errors.Join(err, fmt.Errorf("remove rejected build package: %w", cleanupErr))
			}
		}
		return nil, err
	}
	return upload, nil
}

// Complete stores a fenced build result and candidate. Publication is separate.
func (s *Service) Complete(ctx context.Context, result authoringdomain.BuildResult, checker string) error {
	if result.BuildID == "" || result.WorkerID == "" || result.LeaseToken == "" {
		return authoringdomain.ErrStaleLease
	}
	if checker != "diff" && checker != "testlib" && checker != "interactive" {
		checker = "diff"
	}
	if len(result.Tests) > 10000 {
		return authoringdomain.InvalidInput("build reported too many tests")
	}
	for i := range result.Tests {
		if result.Tests[i].Index <= 0 {
			return authoringdomain.InvalidInput("build reported an invalid test index")
		}
	}
	return s.builds.Complete(ctx, result, checker)
}

func (s *Service) Publish(ctx context.Context, id string, input authoringdomain.PublishInput) (*authoringdomain.Release, error) {
	normalized, err := authoringdomain.NormalizePublication(input)
	if err != nil {
		return nil, err
	}
	return s.packages.Publish(ctx, id, normalized)
}

func (s *Service) Releases(ctx context.Context, id string) ([]authoringdomain.Release, error) {
	return s.packages.Releases(ctx, id)
}
