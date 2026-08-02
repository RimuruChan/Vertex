package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vertex-oj/judge/internal/compile"
	"github.com/vertex-oj/judge/internal/executor"
	"github.com/vertex-oj/judge/internal/run"
	"github.com/vertex-oj/judge/internal/scheduler"
	"github.com/vertex-oj/judge/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 自检:原生沙箱、Landlock 与 cgroup v2 必须同时可用。
	if err := scheduler.SandboxAvailable(); err != nil {
		slog.Error("vertex sandbox unavailable", "error", err)
		os.Exit(1)
	}

	// Postgres
	databaseURL := env("DATABASE_URL", "postgres://vertex:vertex@localhost:5432/vertex")
	pool, err := connectDB(ctx, databaseURL)
	if err != nil {
		slog.Error("connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 配置
	workerNum, err := strconv.Atoi(env("JUDGE_WORKERS", "2"))
	if err != nil || workerNum <= 0 {
		slog.Error("JUDGE_WORKERS must be a positive integer")
		os.Exit(1)
	}
	testdataRoot := env("TESTDATA_ROOT", "/testdata")
	scratchRoot := env("SCRATCH_ROOT", "/scratch")
	cacheRoot := env("CACHE_ROOT", "/cache")
	boxBase := env("SANDBOX_BASE", "/var/local/lib/vertex-sandbox")
	boxID, err := strconv.Atoi(env("SANDBOX_BOX_ID", "0"))
	if err != nil || boxID < 0 || workerNum > 4096 || boxID > 4096-workerNum {
		slog.Error("sandbox box range must fit within 0..4095",
			"box_id", boxID, "workers", workerNum)
		os.Exit(1)
	}

	_ = os.MkdirAll(testdataRoot, 0o755)
	_ = os.MkdirAll(scratchRoot, 0o755)
	_ = os.MkdirAll(cacheRoot, 0o755)

	// 每个并发循环必须拥有独立的 sandbox workspace。共享 workspace 会导致并发提交
	// 互相覆盖文件或在对方运行时执行 cleanup。
	st := store.New(pool, testdataRoot)
	runtimes := make([]scheduler.WorkerRuntime, 0, workerNum)
	sandboxes := make([]*run.Sandbox, 0, workerNum)
	for i := 0; i < workerNum; i++ {
		sandbox := run.NewSandbox(boxID+i, boxBase)
		initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := sandbox.Init(initCtx)
		cancel()
		if err != nil {
			slog.Error("sandbox init failed", "box_id", boxID+i, "error", err)
			cleanupSandboxes(sandboxes)
			os.Exit(1)
		}
		sandboxes = append(sandboxes, sandbox)
		runtimes = append(runtimes, scheduler.WorkerRuntime{
			Compiler: compile.NewCompiler(sandbox, cacheRoot, scratchRoot),
			Executor: executor.NewExecutor(sandbox, scratchRoot),
		})
	}
	defer cleanupSandboxes(sandboxes)

	sched := scheduler.New(st, runtimes)
	sched.Run(ctx)
}

func cleanupSandboxes(sandboxes []*run.Sandbox) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, sandbox := range sandboxes {
		_ = sandbox.Cleanup(ctx)
	}
}

func connectDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var err error
	for attempt := 0; attempt < 15; attempt++ {
		pool, err = pgxpool.New(ctx, url)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		slog.Warn("postgres not ready, retrying", "attempt", attempt+1, "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, err
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
