package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

var toolchainKeyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

const (
	maxBuildAttempts    = 3
	maxBuildLogBytes    = 256 << 10
	maxSampleTextBytes  = 8 << 10
	maxOutcomeTextBytes = 4 << 10
)

// Progress renews the lease and publishes advisory stage/progress data. Like
// the judge heartbeat, a malformed progress number costs display accuracy, not
// the lease.
func (s *BuildRepository) Progress(ctx context.Context, progress authoringdomain.Progress, leaseTTL time.Duration) error {
	renewed, err := dbgen.New(s.db.Pool).RenewBuildLease(ctx, dbgen.RenewBuildLeaseParams{BuildID: progress.BuildID, WorkerID: progress.WorkerID, LeaseToken: progress.LeaseToken, LeaseMs: leaseTTL.Milliseconds(),
		Stage: truncate(progress.Stage, 128), ProgressDone: progress.Done, ProgressTotal: progress.Total, Log: truncate(progress.Log, maxBuildLogBytes), LogLimit: int32(maxBuildLogBytes)})
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
// and filesystem work. Completion only records the frozen check result.
func (s *BuildRepository) RecordPackage(
	ctx context.Context, buildID, problemID, workerID, leaseToken string, upload authoringdomain.PackageUpload,
) error {
	if path.Dir(upload.StoragePath) != problemID || path.Base(upload.StoragePath) != upload.SHA256 {
		return authoringdomain.ErrPackageTarget
	}
	q, guarded := artifactStorageQueries(ctx, problemID)
	if !guarded {
		return s.WithArtifactStorage(ctx, problemID, func(guarded context.Context) error {
			return s.RecordPackage(guarded, buildID, problemID, workerID, leaseToken, upload)
		})
	}
	input, err := q.GetBuildInput(ctx, buildID)
	if err != nil {
		return packageReadError(err)
	}
	var pkg authoringdomain.CheckInput
	if err := json.Unmarshal(input.InputJson, &pkg); err != nil {
		return err
	}
	manifest := json.RawMessage(`{}`)
	if pkg.Check != nil {
		if upload.Artifact == nil {
			return authoringdomain.ErrPackageTarget
		}
		expected, err := json.Marshal(pkg.Check)
		if err != nil {
			return err
		}
		actual, err := json.Marshal(upload.Artifact.Snapshot)
		if err != nil {
			return err
		}
		if !bytes.Equal(expected, actual) || upload.CaseCount != len(pkg.Check.Tests) {
			return authoringdomain.ErrPackageTarget
		}
		manifest, err = json.Marshal(upload.Artifact)
		if err != nil {
			return err
		}
	} else if upload.Artifact != nil {
		return authoringdomain.ErrPackageTarget
	}
	affected, err := q.RecordBuildArtifact(ctx, dbgen.RecordBuildArtifactParams{BuildID: buildID, ProblemID: problemID, WorkerID: workerID, LeaseToken: leaseToken, PackagePath: upload.StoragePath, PackageSha256: upload.SHA256, PackageCases: upload.CaseCount, PackageManifest: manifest})
	if err != nil {
		return err
	}
	if affected != 1 {
		return authoringdomain.ErrStaleLease
	}
	return nil
}

// Complete records a frozen check result without modifying any working copy or release.
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

	checkReady := result.Success
	binding, err := dbgen.New(tx).ReadCheckBinding(ctx, result.BuildID)
	if err != nil {
		return err
	}
	if binding.SourceTreeHash == "" {
		return authoringdomain.ErrPackageTarget
	}
	{
		var artifact authoringdomain.CheckArtifact
		if err := json.Unmarshal(build.PackageManifest, &artifact); err != nil {
			return err
		}
		if checkReady && (artifact.ToolchainKey != result.ToolchainKey || artifact.Snapshot.TreeHash != binding.SourceTreeHash) {
			checkReady = false
			result.ErrorMessage = "检查结果与已上传产物不匹配"
		}
		if checkReady && !validationResultsMatch(artifact.Snapshot.Validation, result.Validation) {
			checkReady = false
			result.ErrorMessage = "校验器自测结果不完整或未通过"
		}
		if checkReady && !toolchainKeyPattern.MatchString(result.ToolchainKey) {
			checkReady = false
			result.ErrorMessage = "检查结果缺少有效的工具链指纹"
		}
		if result.Success && !toolchainKeyPattern.MatchString(result.ToolchainKey) {
			checkReady = false
			result.ErrorMessage = "检查结果缺少有效的工具链指纹"
		}
		if result.ToolchainKey != "" {
			if !toolchainKeyPattern.MatchString(result.ToolchainKey) {
				return authoringdomain.InvalidInput("invalid toolchain fingerprint")
			}
			affected, err := dbgen.New(tx).RecordCheckToolchain(ctx, dbgen.RecordCheckToolchainParams{ID: result.BuildID, WorkerID: sql.NullString{String: result.WorkerID, Valid: true}, LeaseToken: &result.LeaseToken, ToolchainKey: result.ToolchainKey})
			if err != nil {
				return err
			}
			if affected != 1 {
				return authoringdomain.ErrStaleLease
			}
		}
	}
	if checkReady && (build.PackagePath == "" || build.PackageCases <= 0) {
		checkReady = false
		result.ErrorMessage = "构建声明成功但没有上传测试数据"
	}
	finalState := authoringdomain.BuildFailed
	if checkReady {
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
	validation, err := json.Marshal(boundedValidationOutcomes(result.Validation))
	if err != nil {
		return err
	}

	affected, err := dbgen.New(tx).CompleteBuild(ctx, dbgen.CompleteBuildParams{BuildID: result.BuildID, WorkerID: result.WorkerID, LeaseToken: result.LeaseToken, ProblemID: build.ProblemID,
		State: finalState, Log: truncate(result.Log, maxBuildLogBytes), ErrorMessage: truncate(result.ErrorMessage, maxOutcomeTextBytes),
		TestsJson: json.RawMessage(tests), LogLimit: int32(maxBuildLogBytes), SolutionsJson: json.RawMessage(solutions), ValidationJson: json.RawMessage(validation)})
	if err != nil {
		return err
	}
	if affected != 1 {
		return authoringdomain.ErrStaleLease
	}

	return tx.Commit()
}

// failExhausted retires builds whose worker died more times than the retry
// budget allows, so the queue cannot spin forever on a poisoned package.
func (s *BuildRepository) failExhausted(ctx context.Context) error {
	return dbgen.New(s.db.Pool).FailExhaustedBuilds(ctx, maxBuildAttempts)
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
	// Reports are display text, not an alternate transport for original bytes.
	// PostgreSQL text/JSONB cannot represent NUL. Keep raw samples in blobs.
	value = strings.ReplaceAll(strings.ToValidUTF8(value, "�"), "\x00", "�")
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + "\n…(truncated)"
}

func validationResultsMatch(cases []authoringdomain.SnapshotValidation, outcomes []authoringdomain.ValidationOutcome) bool {
	if len(cases) != len(outcomes) {
		return false
	}
	for index, item := range cases {
		outcome := outcomes[index]
		expected := "rejected"
		if item.Definition.Mode == "valid_output" {
			expected = "accepted"
		}
		if outcome.ID != item.ID || outcome.Mode != item.Definition.Mode || outcome.Status != "ok" || outcome.Actual != expected {
			return false
		}
	}
	return true
}

func boundedValidationOutcomes(items []authoringdomain.ValidationOutcome) []authoringdomain.ValidationOutcome {
	result := make([]authoringdomain.ValidationOutcome, 0, min(len(items), authoringdomain.MaxValidationCases))
	for _, item := range items[:min(len(items), authoringdomain.MaxValidationCases)] {
		item.ID = truncate(item.ID, 128)
		item.Name = truncate(item.Name, 512)
		item.Mode = truncate(item.Mode, 32)
		item.Actual = truncate(item.Actual, 32)
		item.Status = truncate(item.Status, 32)
		item.Message = truncate(item.Message, maxOutcomeTextBytes)
		result = append(result, item)
	}
	return result
}
