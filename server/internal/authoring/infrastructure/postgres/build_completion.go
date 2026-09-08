package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/database"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
)

const (
	maxBuildAttempts    = 3
	maxBuildLogBytes    = 256 << 10
	maxSampleTextBytes  = 8 << 10
	maxOutcomeTextBytes = 4 << 10
)

// Cancel stops a queued or running build. A running worker discovers the
// cancellation when its next fenced write is rejected.
func (s *BuildRepository) Cancel(ctx context.Context, problemID, buildID string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tenancypg.LockScope(ctx, tx, tenancydomain.ActorID(ctx)); err != nil {
		return packageReadError(err)
	}
	// Match completion's build -> problem order before changing either row.
	_, err = dbgen.New(tx).LockBuildForCancellation(ctx, dbgen.LockBuildForCancellationParams{BuildID: buildID, ProblemID: problemID, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return authoringdomain.ErrNotFound
	}
	if err != nil {
		return err
	}
	access, err := problempg.LockAccess(ctx, tx, problemID, tenancydomain.ActorID(ctx))
	if err != nil {
		return packageReadError(err)
	}
	if !access.Permissions.Edit {
		return tenancydomain.ErrForbidden
	}
	affected, err := dbgen.New(tx).CancelBuild(ctx, dbgen.CancelBuildParams{BuildID: buildID, ProblemID: problemID, DomainID: tenancydomain.ID(ctx)})
	if err != nil {
		return err
	}
	if affected == 0 {
		return authoringdomain.ErrNotFound
	}
	return tx.Commit()
}

// Progress renews the lease and publishes advisory stage/progress data. Like
// the judge heartbeat, a malformed progress number costs display accuracy, not
// the lease.
func (s *BuildRepository) Progress(ctx context.Context, progress authoringdomain.Progress, leaseTTL time.Duration) error {
	renewed, err := dbgen.New(s.db.Pool).RenewBuildLease(ctx, dbgen.RenewBuildLeaseParams{BuildID: progress.BuildID, WorkerID: progress.WorkerID, LeaseToken: progress.LeaseToken, LeaseMs: leaseTTL.Milliseconds(),
		Stage: progress.Stage, ProgressDone: progress.Done, ProgressTotal: progress.Total, Log: progress.Log, LogLimit: int32(maxBuildLogBytes)})
	if err != nil {
		return err
	}
	if renewed != 1 {
		return authoringdomain.ErrStaleLease
	}
	return nil
}

// ResolvePackageTarget returns the server-owned problem ID only while the
// caller still owns a live build lease. Filesystem paths must be derived from
// this value, never from the worker's query string.
func (s *BuildRepository) ResolvePackageTarget(ctx context.Context, buildID, workerID, leaseToken string) (string, error) {
	problemID, err := dbgen.New(s.db.Pool).GetBuildUploadTarget(ctx, dbgen.GetBuildUploadTargetParams{BuildID: buildID, WorkerID: workerID, LeaseToken: leaseToken})
	if errors.Is(err, sql.ErrNoRows) {
		return "", authoringdomain.ErrStaleLease
	}
	if err != nil {
		return "", err
	}
	return problemID, nil
}

// RecordPackage stores the artifact a worker just materialized. It repeats
// both lease and problem checks to close the race between target resolution
// and filesystem work. Only a successful Complete flips problem_testdata.
func (s *BuildRepository) RecordPackage(
	ctx context.Context, buildID, problemID, workerID, leaseToken string, upload authoringdomain.PackageUpload,
) error {
	if path.Dir(upload.StoragePath) != problemID || path.Base(upload.StoragePath) != upload.SHA256 {
		return authoringdomain.ErrPackageTarget
	}
	affected, err := dbgen.New(s.db.Pool).RecordBuildArtifact(ctx, dbgen.RecordBuildArtifactParams{BuildID: buildID, ProblemID: problemID, WorkerID: workerID, LeaseToken: leaseToken, PackagePath: upload.StoragePath, PackageSha256: upload.SHA256, PackageCases: upload.CaseCount})
	if err != nil {
		return err
	}
	if affected != 1 {
		return authoringdomain.ErrStaleLease
	}
	return nil
}

// Complete records a successful candidate only. A stale data revision remains
// a historical build and cannot replace newer candidate data or a publication.
func (s *BuildRepository) Complete(ctx context.Context, result authoringdomain.BuildResult, checker string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	build, err := dbgen.New(tx).LockBuildForCompletion(ctx, result.BuildID)
	if errors.Is(err, sql.ErrNoRows) {
		return authoringdomain.ErrStaleLease
	}
	if err != nil {
		return err
	}
	if build.State != authoringdomain.BuildRunning {
		return authoringdomain.ErrStaleLease
	}

	candidateReady := result.Success
	if candidateReady && (build.PackagePath == "" || build.PackageCases <= 0) {
		candidateReady = false
		result.ErrorMessage = "构建声明成功但没有上传测试数据"
	}
	finalState := authoringdomain.BuildFailed
	if candidateReady {
		finalState = authoringdomain.BuildSucceeded
	}
	tests, err := json.Marshal(boundedTestOutcomes(result.Tests))
	if err != nil {
		return err
	}
	solutions, err := json.Marshal(boundedSolutionOutcomes(result.Solutions))
	if err != nil {
		return err
	}

	affected, err := dbgen.New(tx).CompleteBuild(ctx, dbgen.CompleteBuildParams{BuildID: result.BuildID, WorkerID: result.WorkerID, LeaseToken: result.LeaseToken, ProblemID: build.ProblemID,
		State: finalState, Log: result.Log, ErrorMessage: result.ErrorMessage,
		TestsJson: json.RawMessage(tests), LogLimit: int32(maxBuildLogBytes), SolutionsJson: json.RawMessage(solutions)})
	if err != nil {
		return err
	}
	if affected != 1 {
		return authoringdomain.ErrStaleLease
	}

	if !candidateReady {
		return tx.Commit()
	}
	currentDataRevision, err := dbgen.New(tx).LockProblemDataRevision(ctx, build.ProblemID)
	if err != nil {
		return err
	}

	if currentDataRevision != build.DataRevision {
		return tx.Commit()
	}

	if err := dbgen.New(tx).SaveBuiltTestdata(ctx, dbgen.SaveBuiltTestdataParams{ProblemID: build.ProblemID, StoragePath: build.PackagePath, Sha256: build.PackageSha256,
		CaseCount: build.PackageCases, Checker: checker, ConfigJson: json.RawMessage(testManifest(result.Tests)), DataRevision: build.DataRevision, BuildID: database.Ptr(result.BuildID), SamplesJson: json.RawMessage(tests)}); err != nil {
		return err
	}
	if err := dbgen.New(tx).MarkProblemBuilt(ctx, dbgen.MarkProblemBuiltParams{ProblemID: build.ProblemID, BuiltRevision: build.DataRevision}); err != nil {
		return err
	}
	return tx.Commit()
}

// failExhausted retires builds whose worker died more times than the retry
// budget allows, so the queue cannot spin forever on a poisoned package.
func (s *BuildRepository) failExhausted(ctx context.Context) error {
	return dbgen.New(s.db.Pool).FailExhaustedBuilds(ctx, maxBuildAttempts)
}

// testManifest is the per-test metadata the judge side keeps alongside the
// data snapshot: groups, points and which tests are samples.
func testManifest(tests []authoringdomain.TestOutcome) []byte {
	type manifestTest struct {
		Index    int    `json:"index"`
		Group    string `json:"group,omitempty"`
		Points   int    `json:"points"`
		IsSample bool   `json:"sample"`
	}
	entries := make([]manifestTest, 0, len(tests))
	for _, item := range tests {
		entries = append(entries, manifestTest{
			Index: item.Index, Group: item.Group, Points: item.Points, IsSample: item.IsSample,
		})
	}
	payload, err := json.Marshal(map[string]any{"tests": entries})
	if err != nil {
		return []byte(`{}`)
	}
	return payload
}

func boundedTestOutcomes(tests []authoringdomain.TestOutcome) []authoringdomain.TestOutcome {
	result := make([]authoringdomain.TestOutcome, 0, len(tests))
	for _, item := range tests {
		item.Message = truncate(item.Message, maxOutcomeTextBytes)
		item.InputHead = truncate(item.InputHead, maxSampleTextBytes)
		item.AnswerHead = truncate(item.AnswerHead, maxSampleTextBytes)
		result = append(result, item)
	}
	return result
}

func boundedSolutionOutcomes(solutions []authoringdomain.SolutionOutcome) []authoringdomain.SolutionOutcome {
	result := make([]authoringdomain.SolutionOutcome, 0, len(solutions))
	for _, item := range solutions {
		item.Message = truncate(item.Message, maxOutcomeTextBytes)
		result = append(result, item)
	}
	return result
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n…(truncated)"
}
