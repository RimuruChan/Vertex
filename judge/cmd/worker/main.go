package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
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

	// 自检:isolate 必须存在(Linux only)
	if err := scheduler.IsolateAvailable(); err != nil {
		slog.Error("isolate not found in PATH — judge worker must run on Linux with ioi/isolate installed")
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
	workerNum := atoi(env("JUDGE_WORKERS", "2"))
	if workerNum <= 0 {
		workerNum = 1
	}
	testdataRoot := env("TESTDATA_ROOT", "/testdata")
	scratchRoot := env("SCRATCH_ROOT", "/scratch")
	cacheRoot := env("CACHE_ROOT", "/cache")
	boxBase := env("ISOLATE_BASE", "/var/local/lib/isolate")
	boxID := atoi(env("ISOLATE_BOX_ID", "0"))

	_ = os.MkdirAll(testdataRoot, 0o755)
	_ = os.MkdirAll(scratchRoot, 0o755)
	_ = os.MkdirAll(cacheRoot, 0o755)

	// 每个并发循环必须拥有独立的 isolate box。共享 box 会导致并发提交
	// 互相覆盖文件或在对方运行时执行 cleanup。
	st := store.New(pool, testdataRoot)
	runtimes := make([]scheduler.WorkerRuntime, 0, workerNum)
	isolates := make([]*run.Isolate, 0, workerNum)
	for i := 0; i < workerNum; i++ {
		isolate := run.NewIsolate(boxID+i, boxBase)
		initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := isolate.Init(initCtx)
		cancel()
		if err != nil {
			slog.Error("isolate --init failed (need root or cgroups v2)", "box_id", boxID+i, "error", err)
			cleanupIsolates(isolates)
			os.Exit(1)
		}
		isolates = append(isolates, isolate)
		runtimes = append(runtimes, scheduler.WorkerRuntime{
			Compiler: compile.NewCompiler(isolate, cacheRoot, scratchRoot),
			Executor: executor.NewExecutor(isolate, scratchRoot),
		})
	}
	defer cleanupIsolates(isolates)

	sched := scheduler.New(st, runtimes)
	sched.Run(ctx)
}

func cleanupIsolates(isolates []*run.Isolate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, isolate := range isolates {
		_ = isolate.Cleanup(ctx)
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

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
