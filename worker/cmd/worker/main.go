package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/builder"
	"github.com/RimuruChan/Vertex/worker/internal/checker"
	judgeclient "github.com/RimuruChan/Vertex/worker/internal/client"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	workerconfig "github.com/RimuruChan/Vertex/worker/internal/config"
	"github.com/RimuruChan/Vertex/worker/internal/executor"
	workerinstance "github.com/RimuruChan/Vertex/worker/internal/instance"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/scheduler"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := workerconfig.Load()
	if err != nil {
		slog.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}
	instanceLock, err := workerinstance.Acquire(cfg.SandboxInstanceLock)
	if err != nil {
		slog.Error("acquire sandbox instance", "instance_id", cfg.SandboxInstanceID, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := instanceLock.Close(); err != nil {
			slog.Warn("release sandbox instance", "instance_id", cfg.SandboxInstanceID, "error", err)
		}
	}()

	// Fail closed before accepting work when the native isolation boundary is unavailable.
	if err := scheduler.SandboxAvailable(); err != nil {
		slog.Error("vertex sandbox unavailable", "error", err)
		os.Exit(1)
	}
	if err := ensureDirectories(cfg.TestdataRoot, cfg.ScratchRoot, cfg.CacheRoot); err != nil {
		slog.Error("prepare worker directories", "error", err)
		os.Exit(1)
	}

	// 每个并发循环必须拥有独立的 sandbox workspace。共享 workspace 会导致并发提交
	// 互相覆盖文件或在对方运行时执行 cleanup。
	apiClient, err := judgeclient.New(
		cfg.JudgeAPIURL, cfg.JudgeAPIToken, cfg.JudgeWorkerID, cfg.TestdataRoot,
		&http.Client{Timeout: cfg.HTTPTimeout}, int(cfg.LongPollTimeout/time.Second),
	)
	if err != nil {
		slog.Error("configure judge API client", "error", err)
		os.Exit(1)
	}
	// The build loop owns its own sandbox slot, so it never contends with a
	// judging loop for a workspace.
	slots := cfg.Workers
	if cfg.BuildsEnabled {
		slots++
	}
	runtimes := make([]scheduler.WorkerRuntime, 0, cfg.Workers)
	sandboxes := make([]*run.Sandbox, 0, slots)
	var packageBuilder *builder.Builder
	for i := 0; i < slots; i++ {
		sandbox := run.NewSandbox(cfg.SandboxBoxID+i, cfg.SandboxBase)
		sandbox.Policy = cfg.SandboxPolicy
		initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := sandbox.Init(initCtx)
		cancel()
		if err != nil {
			slog.Error("sandbox init failed", "box_id", cfg.SandboxBoxID+i, "error", err)
			cleanupSandboxes(sandboxes)
			os.Exit(1)
		}
		sandboxes = append(sandboxes, sandbox)
		compiler := compile.NewCompiler(sandbox, cfg.CacheRoot, cfg.ScratchRoot)
		// The last slot belongs to the package builder when builds are enabled.
		if i == cfg.Workers {
			packageBuilder, err = builder.NewBuilder(sandbox, compiler, cfg.ScratchRoot, cfg.TestlibPath)
			if err != nil {
				slog.Error("configure package builder", "error", err, "testlib_path", cfg.TestlibPath)
				cleanupSandboxes(sandboxes)
				os.Exit(1)
			}
			continue
		}
		// Each judging slot compiles packaged checkers in its own workspace;
		// the compile cache is shared, so the work happens once per revision.
		checkerSource, err := checker.NewSourceCompiler(compiler, cfg.TestlibPath)
		if err != nil {
			slog.Error("configure testlib checker support", "error", err, "testlib_path", cfg.TestlibPath)
			cleanupSandboxes(sandboxes)
			os.Exit(1)
		}
		runtimes = append(runtimes, scheduler.WorkerRuntime{
			Compiler: compiler,
			Executor: executor.NewExecutor(
				sandbox, cfg.ScratchRoot,
				checker.NewRunner(sandbox, checker.DefaultLimits()), checkerSource,
			),
		})
	}
	defer cleanupSandboxes(sandboxes)

	var group sync.WaitGroup
	if packageBuilder != nil {
		group.Add(1)
		go func() {
			defer group.Done()
			scheduler.NewBuildScheduler(
				apiClient, packageBuilder, cfg.JudgeWorkerID, cfg.BuildProgressTick,
			).Run(ctx)
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		scheduler.New(apiClient, cfg.JudgeWorkerID, runtimes).Run(ctx)
	}()
	group.Wait()
}

func ensureDirectories(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func cleanupSandboxes(sandboxes []*run.Sandbox) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, sandbox := range sandboxes {
		_ = sandbox.Cleanup(ctx)
	}
}
