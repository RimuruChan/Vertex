package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	judgeclient "github.com/RimuruChan/Vertex/judge/internal/client"
	"github.com/RimuruChan/Vertex/judge/internal/compile"
	workerconfig "github.com/RimuruChan/Vertex/judge/internal/config"
	"github.com/RimuruChan/Vertex/judge/internal/executor"
	"github.com/RimuruChan/Vertex/judge/internal/run"
	"github.com/RimuruChan/Vertex/judge/internal/scheduler"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := workerconfig.Load()
	if err != nil {
		slog.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}

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
	runtimes := make([]scheduler.WorkerRuntime, 0, cfg.Workers)
	sandboxes := make([]*run.Sandbox, 0, cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
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
		runtimes = append(runtimes, scheduler.WorkerRuntime{
			Compiler: compile.NewCompiler(sandbox, cfg.CacheRoot, cfg.ScratchRoot),
			Executor: executor.NewExecutor(sandbox, cfg.ScratchRoot),
		})
	}
	defer cleanupSandboxes(sandboxes)

	sched := scheduler.New(apiClient, cfg.JudgeWorkerID, runtimes)
	sched.Run(ctx)
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
