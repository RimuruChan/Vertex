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
	sandbox := run.NewClient(cfg.SandboxBase, cfg.SandboxPolicy)
	if err := ensureDirectories(cfg.TestdataRoot, cfg.ScratchRoot, cfg.CacheRoot); err != nil {
		slog.Error("prepare worker directories", "error", err)
		os.Exit(1)
	}

	workDir, err := os.MkdirTemp(cfg.ScratchRoot, "worker-")
	if err != nil {
		slog.Error("create worker staging directory", "error", err)
		os.Exit(1)
	}
	defer os.RemoveAll(workDir)
	cfg.ScratchRoot = workDir

	// The client is a stateless factory. Each execution creates its own environment.
	apiClient, err := judgeclient.New(
		cfg.JudgeAPIURL, cfg.JudgeAPIToken, cfg.JudgeWorkerID, cfg.TestdataRoot,
		&http.Client{Timeout: cfg.HTTPTimeout}, int(cfg.LongPollTimeout/time.Second),
	)
	if err != nil {
		slog.Error("configure judge API client", "error", err)
		os.Exit(1)
	}
	// Judge and build loops create environments on demand.
	slots := cfg.Workers
	if cfg.BuildsEnabled {
		slots++
	}
	runtimes := make([]scheduler.WorkerRuntime, 0, cfg.Workers)
	var packageBuilder *builder.Builder
	for i := 0; i < slots; i++ {
		compiler := compile.NewCompiler(sandbox, cfg.CacheRoot, cfg.ScratchRoot)
		// The last slot belongs to the package builder when builds are enabled.
		if i == cfg.Workers {
			packageBuilder, err = builder.NewBuilder(sandbox, compiler, cfg.ScratchRoot, cfg.TestlibPath)
			if err != nil {
				slog.Error("configure package builder", "error", err, "testlib_path", cfg.TestlibPath)
				os.Exit(1)
			}
			continue
		}
		// Each judging slot compiles packaged checkers in its own workspace;
		// the compile cache is shared, so the work happens once per revision.
		checkerSource, err := checker.NewSourceCompiler(compiler, cfg.TestlibPath)
		if err != nil {
			slog.Error("configure testlib checker support", "error", err, "testlib_path", cfg.TestlibPath)
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
