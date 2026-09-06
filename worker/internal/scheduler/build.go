package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/builder"
)

// BuildClient is the build scheduler's authenticated Web API boundary.
type BuildClient interface {
	ClaimBuild(ctx context.Context) (*builder.Job, error)
	ReportBuildProgress(ctx context.Context, job *builder.Job, stage string, done, total int, log string) error
	UploadBuildPackage(ctx context.Context, job *builder.Job, archive []byte) error
	CompleteBuild(ctx context.Context, job *builder.Job, report *builder.Report, log string) error
}

// PackageBuilder runs one build to completion.
type PackageBuilder interface {
	Build(ctx context.Context, job *builder.Job, reporter builder.Reporter) (*builder.Report, error)
}

// maxBuildDuration bounds a single build so a pathological package cannot pin
// a worker slot forever.
const maxBuildDuration = 30 * time.Minute

// BuildScheduler claims package builds and reports their results. It runs one
// build at a time because a build owns its sandbox workspace exclusively.
type BuildScheduler struct {
	client   BuildClient
	builder  PackageBuilder
	workerID string
	// progressInterval is how often a running build renews its lease.
	progressInterval time.Duration
}

func NewBuildScheduler(client BuildClient, packageBuilder PackageBuilder, workerID string, progressInterval time.Duration) *BuildScheduler {
	if progressInterval <= 0 {
		progressInterval = 15 * time.Second
	}
	return &BuildScheduler{
		client: client, builder: packageBuilder, workerID: workerID,
		progressInterval: progressInterval,
	}
}

// Run claims and executes builds until the context ends.
func (s *BuildScheduler) Run(ctx context.Context) {
	slog.Info("package build scheduler started", "worker_id", s.workerID)
	defer slog.Info("package build scheduler stopped", "worker_id", s.workerID)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := s.client.ClaimBuild(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("build claim failed", "worker_id", s.workerID, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if job == nil {
			continue
		}
		slog.Info("building problem package",
			"worker_id", s.workerID, "build_id", job.BuildID,
			"problem_id", job.ProblemID, "revision", job.Revision, "tests", len(job.Tests))
		s.buildOne(ctx, job)
	}
}

// buildOne runs one leased build and always reports a terminal result unless
// the lease was already lost.
func (s *BuildScheduler) buildOne(ctx context.Context, job *builder.Job) {
	buildCtx, cancel := context.WithTimeout(ctx, maxBuildDuration)
	defer cancel()

	tracker := newProgressTracker()
	leaseLost := make(chan struct{})
	var once sync.Once
	reportLost := func() { once.Do(func() { close(leaseLost) }) }

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.reportLoop(buildCtx, job, tracker, func() {
			reportLost()
			cancel()
		})
	}()

	report, err := s.builder.Build(buildCtx, job, tracker)
	cancel()
	<-done

	select {
	case <-leaseLost:
		slog.Warn("discarding build after lease loss",
			"worker_id", s.workerID, "build_id", job.BuildID, "problem_id", job.ProblemID)
		return
	default:
	}

	if err != nil {
		// A worker-side failure is still reported so the author sees why the
		// build stopped instead of watching it hang until the lease expires.
		report = &builder.Report{Stage: "done", ErrorMessage: "构建执行失败: " + err.Error()}
		slog.Error("package build failed",
			"worker_id", s.workerID, "build_id", job.BuildID, "error", err)
	}

	if report.Success {
		if uploadErr := s.client.UploadBuildPackage(context.WithoutCancel(ctx), job, report.Archive); uploadErr != nil {
			if errors.Is(uploadErr, ErrLeaseLost) {
				slog.Warn("build package rejected as stale",
					"worker_id", s.workerID, "build_id", job.BuildID)
				return
			}
			report.Success = false
			report.ErrorMessage = "测试数据上传失败: " + uploadErr.Error()
		}
	}

	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer finishCancel()
	if err := s.client.CompleteBuild(finishCtx, job, report, tracker.drainLog()); err != nil {
		if errors.Is(err, ErrLeaseLost) {
			slog.Warn("build result rejected as stale",
				"worker_id", s.workerID, "build_id", job.BuildID)
			return
		}
		slog.Error("report build result failed",
			"worker_id", s.workerID, "build_id", job.BuildID, "error", err)
		return
	}
	slog.Info("package build finished",
		"worker_id", s.workerID, "build_id", job.BuildID,
		"problem_id", job.ProblemID, "success", report.Success)
}

// reportLoop renews the lease on a fixed interval, carrying whatever progress
// and log lines the build has produced since the previous tick.
func (s *BuildScheduler) reportLoop(ctx context.Context, job *builder.Job, tracker *progressTracker, onLeaseLost func()) {
	ticker := time.NewTicker(s.progressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stage, doneCount, total := tracker.snapshot()
			reportCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := s.client.ReportBuildProgress(reportCtx, job, stage, doneCount, total, tracker.drainLog())
			cancel()
			if errors.Is(err, ErrLeaseLost) {
				onLeaseLost()
				return
			}
			if err != nil && ctx.Err() == nil {
				slog.Warn("build progress report failed",
					"worker_id", s.workerID, "build_id", job.BuildID, "error", err)
			}
		}
	}
}

// maxTrackedLogBytes bounds the buffered log between two progress reports.
const maxTrackedLogBytes = 32 << 10

// progressTracker is the builder.Reporter implementation. The build goroutine
// writes and the reporting goroutine drains, so every field is mutex-guarded.
type progressTracker struct {
	mutex sync.Mutex
	stage string
	done  int
	total int
	log   strings.Builder
}

func newProgressTracker() *progressTracker {
	return &progressTracker{stage: "compile"}
}

func (t *progressTracker) Stage(stage string, done, total int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.stage, t.done, t.total = stage, done, total
}

func (t *progressTracker) Logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.log.Len() >= maxTrackedLogBytes {
		return
	}
	t.log.WriteString(line)
	t.log.WriteString("\n")
}

func (t *progressTracker) snapshot() (string, int, int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.stage, t.done, t.total
}

// drainLog returns and clears the buffered lines so each is reported once.
func (t *progressTracker) drainLog() string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	text := t.log.String()
	t.log.Reset()
	return text
}
