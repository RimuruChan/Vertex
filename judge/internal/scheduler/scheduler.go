package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vertex-oj/judge/internal/compile"
	"github.com/vertex-oj/judge/internal/executor"
	"github.com/vertex-oj/judge/internal/verdict"
)

// Submission 判题 worker 视角的提交。
type Submission struct {
	ID         string
	UserID     string
	ProblemID  string
	Language   string
	SourceCode string
	ContestID  *string
}

// ProblemLimits 判题 worker 视角的题目限值。
type ProblemLimits struct {
	TimeLimitMs int
	MemLimitKB  int
}

// Testdata 测试数据目录。
type Testdata struct {
	Dir       string // 宿主侧目录,含 1.in / 1.out / 2.in / 2.out ...
	CaseCount int
}

// Store 抽象 worker 需要的数据访问(便于测试替换)。
type Store interface {
	ClaimNext(ctx context.Context) (*Submission, error)
	GetProblemLimits(ctx context.Context, problemID string) (*ProblemLimits, error)
	GetTestdataDir(ctx context.Context, problemID string) (*Testdata, error)
	MarkResult(ctx context.Context, sub *Submission, status string, score int,
		totalTime int64, peakMem int, compileResult string, cases []executor.CaseResult) error
	Requeue(ctx context.Context, subID string) error
}

// WorkerRuntime owns one sandbox workspace and the components that use it.
// A runtime must never be shared by concurrent judge loops.
type WorkerRuntime struct {
	Compiler *compile.Compiler
	Executor *executor.Executor
}

// Scheduler 判题 worker 主循环。
type Scheduler struct {
	store     Store
	workers   []WorkerRuntime
	pollEvery time.Duration
	stopped   chan struct{}
}

// New 创建 scheduler。
func New(store Store, workers []WorkerRuntime) *Scheduler {
	if len(workers) == 0 {
		panic("scheduler requires at least one worker runtime")
	}
	return &Scheduler{
		store:     store,
		workers:   workers,
		pollEvery: 500 * time.Millisecond,
		stopped:   make(chan struct{}),
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
	slog.Info("judge scheduler started", "workers", len(s.workers))
	<-ctx.Done()
	wg.Wait()
	slog.Info("judge scheduler stopped")
}

// loop 单个判题循环:领取 → 判题 → 写结果。
func (s *Scheduler) loop(ctx context.Context, id int, worker *WorkerRuntime) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 领取带超时,避免长阻塞
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		sub, err := s.store.ClaimNext(claimCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("claim failed", "worker", id, "error", err)
			time.Sleep(s.pollEvery)
			continue
		}
		if sub == nil {
			// 无待判提交,休眠后重试
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.pollEvery):
			}
			continue
		}

		slog.Info("judging submission", "worker", id, "submission", sub.ID, "problem", sub.ProblemID, "lang", sub.Language)
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

	// 语言配置
	langCfg, ok := compile.Supported[sub.Language]
	if !ok {
		s.finish(sub, verdict.SE, 0, 0, 0, "unsupported language: "+sub.Language, nil)
		return
	}

	// 题目限值与测试数据
	limits, err := s.store.GetProblemLimits(judgeCtx, sub.ProblemID)
	if err != nil {
		s.finish(sub, verdict.SE, 0, 0, 0, "load limits: "+err.Error(), nil)
		return
	}
	td, err := s.store.GetTestdataDir(judgeCtx, sub.ProblemID)
	if err != nil {
		s.finish(sub, verdict.SE, 0, 0, 0, "load testdata: "+err.Error(), nil)
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
	casesSpec := buildCases(td, limits)
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

	slog.Info("judge done", "worker", workerID, "submission", sub.ID, "verdict", status)
	s.finish(sub, status, score, totalTime, peakMem, compileErr, cases)
}

func (s *Scheduler) finish(sub *Submission, status string, score int, totalTime int64, peakMem int, compileErr string, cases []executor.CaseResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.store.MarkResult(ctx, sub, status, score, totalTime, peakMem, compileErr, cases); err != nil {
		slog.Error("mark result failed", "submission", sub.ID, "error", err)
		// 写失败:退回队列重试,避免死结果
		_ = s.store.Requeue(ctx, sub.ID)
	}
}

// buildCases 由测试数据目录组装测试点(1.in/1.out, 2.in/2.out ...)。
func buildCases(td *Testdata, limits *ProblemLimits) []executor.Case {
	cases := make([]executor.Case, 0, td.CaseCount)
	for i := 1; i <= td.CaseCount; i++ {
		inPath := filepath.Join(td.Dir, fmt.Sprintf("%d.in", i))
		outPath := filepath.Join(td.Dir, fmt.Sprintf("%d.out", i))
		if _, err := os.Stat(inPath); err != nil {
			slog.Warn("missing input file", "path", inPath)
			continue
		}
		if _, err := os.Stat(outPath); err != nil {
			slog.Warn("missing expected output", "path", outPath)
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
