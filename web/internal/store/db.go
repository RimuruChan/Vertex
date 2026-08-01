package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB 封装 pgx 连接池,供各 store 使用。
type DB struct {
	Pool *pgxpool.Pool
}

// NewDB 从环境变量 DATABASE_URL 建立连接池(如 postgres://vertex:vertex@localhost:5432/vertex)。
func NewDB(ctx context.Context) (*DB, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://vertex:vertex@localhost:5432/vertex"
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// 启动时确认可达(带重试,因为 compose 里 postgres 可能还在启动)
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	slog.Info("connected to postgres")
	return &DB{Pool: pool}, nil
}

// Close 关闭连接池。
func (db *DB) Close() {
	db.Pool.Close()
}
