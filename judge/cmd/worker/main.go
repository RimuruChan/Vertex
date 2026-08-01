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
	testdataRoot := env("TESTDATA_ROOT", "/testdata")
	scratchRoot := env("SCRATCH_ROOT", "/scratch")
	cacheRoot := env("CACHE_ROOT", "/cache")
	boxBase := env("ISOLATE_BASE", "/var/local/lib/isolate")
	boxID := atoi(env("ISOLATE_BOX_ID", "0"))

	_ = os.MkdirAll(testdataRoot, 0o755)
	_ = os.MkdirAll(scratchRoot, 0o755)
	_ = os.MkdirAll(cacheRoot, 0o755)

	// 组件
	st := store.New(pool, testdataRoot)
	isolate := run.NewIsolate(boxID, boxBase)

	// 初始化 box(worker 启动即准备;失败则退出,避免判题中才发现不可用)
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := isolate.Init(initCtx); err != nil {
		slog.Error("isolate --init failed (need root or cgroups v2)", "error", err)
		cancel()
		os.Exit(1)
	}
	cancel()

	compiler := compile.NewCompiler(isolate, cacheRoot, scratchRoot)
	ex := executor.NewExecutor(isolate, scratchRoot)

	sched := scheduler.New(st, isolate, compiler, ex, workerNum)
	sched.Run(ctx)
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
