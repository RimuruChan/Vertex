package application

import (
	"context"
	"errors"
	"strings"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// BuildService implements only the fenced Worker protocol. Authoring edits,
// checks and releases enter through Workbench and immutable content trees.
type BuildService struct {
	builds            authoringdomain.WorkerBuildRepository
	publisher         authoringdomain.ArtifactPublisher
	dispatcher        *Dispatcher
	leaseTTL, maxWait time.Duration
}

func NewBuildService(builds authoringdomain.WorkerBuildRepository, publisher authoringdomain.ArtifactPublisher, dispatcher *Dispatcher, leaseTTL, maxWait time.Duration) (*BuildService, error) {
	if builds == nil || publisher == nil || dispatcher == nil {
		return nil, errors.New("build dependencies must not be nil")
	}
	if leaseTTL <= 0 || maxWait <= 0 || leaseTTL <= maxWait {
		return nil, errors.New("build lease TTL must be greater than the positive long-poll timeout")
	}
	return &BuildService{builds: builds, publisher: publisher, dispatcher: dispatcher, leaseTTL: leaseTTL, maxWait: maxWait}, nil
}

// Claim long-polls for one leased build job, mirroring the judge protocol so
// workers can run both loops with the same failure handling.
func (s *BuildService) Claim(ctx context.Context, workerID string, wait time.Duration) (*authoringdomain.Build, *authoringdomain.CheckInput, error) {
	if !authoringdomain.AcceptsCheckProtocol(ctx) {
		return nil, nil, authoringdomain.InvalidInput("worker must support the current frozen-check protocol")
	}
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
			if pkg == nil || pkg.Check == nil {
				return nil, nil, authoringdomain.InvalidInput("legacy build input is no longer supported")
			}
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

func (s *BuildService) Progress(ctx context.Context, progress authoringdomain.Progress) error {
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
func (s *BuildService) UploadPackage(
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
	var upload *authoringdomain.PackageUpload
	err = s.builds.WithArtifactStorage(ctx, problemID, func(guarded context.Context) error {
		var err error
		upload, err = s.publisher.Publish(problemID, archive)
		if err != nil {
			return err
		}
		return s.builds.RecordPackage(guarded, buildID, problemID, workerID, leaseToken, *upload)
	})
	if err != nil {
		// A newer lease or another frozen check may already reference this
		// content-addressed directory, even when this request installed it.
		// Reclaim unreferenced artifacts by reference-aware GC, never by a
		// stale request's filesystem compensation.
		return nil, err
	}
	return upload, nil
}

// Complete stores a fenced build result and candidate. Publication is separate.
func (s *BuildService) Complete(ctx context.Context, result authoringdomain.BuildResult, checker string) error {
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
