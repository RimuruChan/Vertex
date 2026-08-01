package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vertex-oj/web/internal/api"
	"github.com/vertex-oj/web/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 连接数据库(带重试,等 postgres 就绪)
	var db *store.DB
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		db, err = store.NewDB(ctx)
		if err == nil {
			break
		}
		slog.Warn("postgres not ready, retrying", "attempt", attempt+1, "error", err)
		select {
		case <-ctx.Done():
			os.Exit(1)
		case <-time.After(2 * time.Second):
		}
	}
	if db == nil {
		slog.Error("cannot connect to postgres after retries")
		os.Exit(1)
	}
	defer db.Close()

	// 应用迁移
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://vertex:vertex@localhost:5432/vertex"
	}
	if err := store.RunMigrations(databaseURL); err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}

	// 可选:启动时创建默认管理员(ADMIN_USERNAME / ADMIN_PASSWORD 环境变量)
	store.BootstrapAdmin(ctx, db)

	router := api.Router(db)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("vertex web listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
