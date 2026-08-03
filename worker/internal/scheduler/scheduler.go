package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/executor"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// Submission 判题 worker 视角的提交。
type Submission struct {
	JobID      string
	ID         string
	Generation int
	Attempt    int
	LeaseToken string
	LeaseUntil time.Time
	UserID     string
	ProblemID  string
	Language   string
	SourceCode string
	ContestID  *string
	Limits     ProblemLimits
	Testdata   Testdata
}

// ProblemLimits 判题 worker 视角的题目限值。
type ProblemLimits struct {
	TimeLimitMs int
	MemLimitKB  int
}

// Testdata 测试数据目录。
type Testdata struct {
	Dir         string // 宿主侧目录,含 1.in / 1.out / 2.in / 2.out ...
	DataVersion int
	SHA256      string
	CaseCount   int
	Checker     string
}

// JobClient is the scheduler's authenticated Web API boundary.
type JobClient interface {
	ClaimNext(ctx context.Context) (*Submission, error)
	Heartbeat(ctx context.Context, sub *Submission) error
	MarkResult(ctx context.Context, sub *Submission, status string, score int,
		totalTime int64, peakMem int, compileResult string, cases []executor.CaseResult) error
}

var ErrLeaseLost = errors.New("judge lease lost")

// WorkerRuntime owns one sandbox workspace and the components that use it.
// A runtime must never be shared by concurrent judge loops.
type WorkerRuntime struct {
	Compiler *compile.Compiler
	Executor *executor.Executor
}

// Scheduler 判题 worker 主循环。
type Scheduler struct {
	client   JobClient
	workerID string
	workers  []WorkerRuntime
	stopped  chan struct{}
}

// New 创建 scheduler。
func New(client JobClient, workerID string, workers []WorkerRuntime) *Scheduler {
	if len(workers) == 0 {
		panic("scheduler requires at least one worker runtime")
	}
	return &Scheduler{
		client:   client,
		workerID: workerID,
		workers:  workers,
		stopped:  make(chan struct{}),
	}
}

// Run 阻塞运行 N 个判题循环直到 ctx 取消。
func (s *Scheduler) Run(ctx context.Context) {
	defer close(s.stopped)
	var wg sync.WaitGroup
	for i := range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.loop(ctx, i, &s.workers[i])
		}()
	}
	slog.Info("judge scheduler started", "worker_id", s.workerID, "workers", len(s.workers))
	<-ctx.Done()
	wg.Wait()
	slog.Info("judge scheduler stopped", "worker_id", s.workerID)
}

// loop 单个判题循环:领取 → 判题 → 写结果。
func (s *Scheduler) loop(ctx context.Context, id int, worker *WorkerRuntime) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sub, err := s.client.ClaimNext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("claim failed", "worker_id", s.workerID, "worker_slot", id, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if sub == nil {
			// ClaimNext performs the server-side long poll; immediately opening the
			// next request avoids adding a second idle delay.
			continue
		}

		slog.Info("judging submission",
			"worker_id", s.workerID, "worker_slot", id, "job_id", sub.JobID,
			"submission_id", sub.ID, "generation", sub.Generation,
			"problem_id", sub.ProblemID, "language", sub.Language)
		s.judgeOne(ctx, id, worker, sub)
	}
}

// judgeOne 判一份提交,无论成败都写回结果。
func (s *Scheduler) judgeOne(ctx context.Context, workerID int, worker *WorkerRuntime, sub *Submission) {
	status := verdict.AC
	score := 100
	var compileErr string
	var cases []executor.CaseResult
	var totalTime int64
	peakMem := 0

	judgeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	heartbeatDone := make(chan struct{})
	leaseLost := make(chan struct{}, 1)
	go func() {
		defer close(heartbeatDone)
		s.heartbeatLoop(judgeCtx, sub, leaseLost, cancel)
	}()
	defer func() {
		cancel()
		<-heartbeatDone
	}()

	// 语言配置
	langCfg, ok := compile.Supported[sub.Language]
	if !ok {
		s.finish(sub, verdict.SE, 0, 0, 0, "unsupported language: "+sub.Language, nil)
		return
	}

	if sub.Testdata.CaseCount <= 0 || sub.Testdata.Dir == "" {
		s.finish(sub, verdict.SE, 0, 0, 0, "testdata is missing", nil)
		return
	}

	// 编译(带缓存)
	hash := sha256.Sum256([]byte(sub.SourceCode))
	srcHash := hex.EncodeToString(hash[:])
	exePath, cres := worker.Compiler.Compile(judgeCtx, sub.Language, []byte(sub.SourceCode), srcHash)
	if !cres.OK {
		s.finish(sub, verdict.CE, 0, 0, 0, cres.Error, nil)
		return
	}
	compileErr = ""

	// 组装测试点
	casesSpec := s.buildCases(sub)
	if len(casesSpec) == 0 {
		s.finish(sub, verdict.SE, 0, 0, 0, "no test cases", nil)
		return
	}

	// 执行
	results, tt, pm, err := worker.Executor.Judge(judgeCtx, langCfg, exePath, casesSpec)
	if err != nil {
		s.finish(sub, verdict.SE, 0, 0, 0, "judge failed: "+err.Error(), nil)
		return
	}
	totalTime, peakMem = tt, pm
	cases = results

	// 聚合判定:第一个非 AC 即最终判定
	for _, c := range results {
		if c.Verdict != verdict.AC {
			status = c.Verdict
			if c.Verdict == verdict.Skip {
				continue
			}
			score = 0
			break
		}
	}

	slog.Info("judge done",
		"worker_id", s.workerID, "worker_slot", workerID, "job_id", sub.JobID,
		"submission_id", sub.ID, "generation", sub.Generation, "verdict", status)
	select {
	case <-leaseLost:
		slog.Warn("discarding result after lease loss",
			"worker_id", s.workerID, "worker_slot", workerID, "job_id", sub.JobID,
			"submission_id", sub.ID, "generation", sub.Generation)
		return
	default:
	}
	s.finish(sub, status, score, totalTime, peakMem, compileErr, cases)
}

func (s *Scheduler) heartbeatLoop(ctx context.Context, sub *Submission, leaseLost chan<- struct{}, cancel context.CancelFunc) {
	interval := time.Until(sub.LeaseUntil) / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatCtx, heartbeatCancel := context.WithTimeout(ctx, 10*time.Second)
			err := s.client.Heartbeat(heartbeatCtx, sub)
			heartbeatCancel()
			if errors.Is(err, ErrLeaseLost) {
				select {
				case leaseLost <- struct{}{}:
				default:
				}
				cancel()
				return
			}
			if err != nil {
				slog.Warn("judge heartbeat failed",
					"worker_id", s.workerID, "job_id", sub.JobID,
					"submission_id", sub.ID, "generation", sub.Generation, "error", err)
			}
		}
	}
}

func (s *Scheduler) finish(sub *Submission, status string, score int, totalTime int64, peakMem int, compileErr string, cases []executor.CaseResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.client.MarkResult(ctx, sub, status, score, totalTime, peakMem, compileErr, cases); err != nil {
		if errors.Is(err, ErrLeaseLost) {
			slog.Warn("judge result rejected as stale",
				"worker_id", s.workerID, "job_id", sub.JobID,
				"submission_id", sub.ID, "generation", sub.Generation)
			return
		}
		slog.Error("mark result failed",
			"worker_id", s.workerID, "job_id", sub.JobID,
			"submission_id", sub.ID, "generation", sub.Generation, "error", err)
	}
}

// buildCases 由测试数据目录组装测试点(1.in/1.out, 2.in/2.out ...)。
func (s *Scheduler) buildCases(sub *Submission) []executor.Case {
	td, limits := &sub.Testdata, &sub.Limits
	cases := make([]executor.Case, 0, td.CaseCount)
	for i := 1; i <= td.CaseCount; i++ {
		inPath := filepath.Join(td.Dir, fmt.Sprintf("%d.in", i))
		outPath := filepath.Join(td.Dir, fmt.Sprintf("%d.out", i))
		if _, err := os.Stat(inPath); err != nil {
			slog.Warn("missing input file",
				"worker_id", s.workerID, "job_id", sub.JobID,
				"submission_id", sub.ID, "generation", sub.Generation, "path", inPath)
			continue
		}
		if _, err := os.Stat(outPath); err != nil {
			slog.Warn("missing expected output",
				"worker_id", s.workerID, "job_id", sub.JobID,
				"submission_id", sub.ID, "generation", sub.Generation, "path", outPath)
			continue
		}
		cases = append(cases, executor.Case{
			Index:        i,
			InputPath:    inPath,
			ExpectedPath: outPath,
			TimeLimitMs:  limits.TimeLimitMs,
			MemLimitKB:   limits.MemLimitKB,
		})
	}
	return cases
}

// SandboxAvailable validates the native runner and its required kernel features.
func SandboxAvailable() error {
	if _, err := exec.LookPath("vertex-sandbox"); err != nil {
		return err
	}
	cmd := exec.Command("vertex-sandbox", "probe")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("vertex-sandbox probe: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
